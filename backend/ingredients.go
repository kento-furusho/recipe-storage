package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Ingredient は食材マスタの1件（全ユーザー共通）。正規名 Name がキー。
type Ingredient struct {
	Name      string    `json:"name" dynamodbav:"name"`
	Aliases   []string  `json:"aliases,omitempty" dynamodbav:"aliases,omitempty"`
	Category  string    `json:"category" dynamodbav:"category"`
	IsStaple  bool      `json:"isStaple" dynamodbav:"isStaple"` // 基本調味料（家にある前提で検索時に無視する）
	CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
}

// ingredientCategories は食材マスタの分類。AIがこれ以外を返したら「その他」にする
var ingredientCategories = []string{
	"肉", "魚介", "野菜", "きのこ", "豆・大豆製品", "卵・乳製品", "穀物・麺", "加工品", "調味料", "その他",
}

// invokeLLM はテストで差し替えられるようにしたAI呼び出し
var invokeLLM = callBedrockAPI

// --- 食材マスタのキャッシュ（Lambdaのメモリに保持） ---

const ingredientMasterTTL = 5 * time.Minute

var (
	ingredientMasterMu       sync.Mutex
	ingredientMasterCache    []*Ingredient
	ingredientMasterLoadedAt time.Time
)

// loadIngredientMaster は食材マスタを返す。5分間はメモリのキャッシュを使う
func loadIngredientMaster(ctx context.Context) ([]*Ingredient, error) {
	ingredientMasterMu.Lock()
	defer ingredientMasterMu.Unlock()

	if ingredientMasterCache != nil && time.Since(ingredientMasterLoadedAt) < ingredientMasterTTL {
		return append([]*Ingredient(nil), ingredientMasterCache...), nil
	}
	list, err := dbStore.ListIngredients(ctx)
	if err != nil {
		return nil, err
	}
	ingredientMasterCache = list
	ingredientMasterLoadedAt = time.Now()
	return append([]*Ingredient(nil), list...), nil
}

// addToIngredientMasterCache は新しく登録した食材をキャッシュにも反映する
func addToIngredientMasterCache(ingredient *Ingredient) {
	ingredientMasterMu.Lock()
	defer ingredientMasterMu.Unlock()
	if ingredientMasterCache == nil {
		return // 次回の読み込みでDBから取得される
	}
	for _, existing := range ingredientMasterCache {
		if existing.Name == ingredient.Name {
			return
		}
	}
	ingredientMasterCache = append(ingredientMasterCache, ingredient)
}

// resetIngredientMasterCache はキャッシュを捨てる（テスト用）
func resetIngredientMasterCache() {
	ingredientMasterMu.Lock()
	defer ingredientMasterMu.Unlock()
	ingredientMasterCache = nil
	ingredientMasterLoadedAt = time.Time{}
}

// --- 名前の正規化と照合 ---

// canonicalizeIngredientName は全角/半角をそろえ（NFKC）、空白を取り除く
func canonicalizeIngredientName(s string) string {
	return strings.Join(strings.Fields(norm.NFKC.String(s)), "")
}

// buildIngredientLookup は正規化した名前・別名 → マスタの正規名 の対応表を作る。
// 別名が他の食材の名前とぶつかった場合は名前を優先する。
func buildIngredientLookup(master []*Ingredient) map[string]string {
	lookup := make(map[string]string, len(master)*2)
	for _, ingredient := range master {
		for _, alias := range ingredient.Aliases {
			if key := canonicalizeIngredientName(alias); key != "" {
				lookup[key] = ingredient.Name
			}
		}
	}
	for _, ingredient := range master {
		if key := canonicalizeIngredientName(ingredient.Name); key != "" {
			lookup[key] = ingredient.Name
		}
	}
	return lookup
}

func isValidIngredientCategory(category string) bool {
	for _, c := range ingredientCategories {
		if c == category {
			return true
		}
	}
	return false
}

// --- AIによる材料の対応づけ ---

type resolvedIngredient struct {
	Name     string   `json:"name"`
	IsNew    bool     `json:"isNew"`
	Category string   `json:"category"`
	IsStaple bool     `json:"isStaple"`
	Aliases  []string `json:"aliases"`
}

func buildIngredientResolutionPrompt(rawLines []string, master []*Ingredient) string {
	var p strings.Builder

	p.WriteString("あなたは料理レシピの材料を、食材マスタの食材に対応づける担当です。\n")
	p.WriteString("以下の【材料】の各行が、どの食材を指すかを判定してください。\n\n")

	p.WriteString("【食材マスタ】（名前 / 別名 / 分類）\n")
	if len(master) == 0 {
		p.WriteString("（まだ登録なし）\n")
	}
	for _, ingredient := range master {
		p.WriteString("- " + ingredient.Name)
		if len(ingredient.Aliases) > 0 {
			p.WriteString(" / 別名: " + strings.Join(ingredient.Aliases, "、"))
		}
		p.WriteString(" / " + ingredient.Category)
		if ingredient.IsStaple {
			p.WriteString("（基本調味料）")
		}
		p.WriteString("\n")
	}

	p.WriteString("\n【材料】\n")
	for _, line := range rawLines {
		p.WriteString("- " + line + "\n")
	}

	p.WriteString("\n【ルール】\n")
	p.WriteString("- 同じ食材が食材マスタにあれば、必ずその名前をそのまま使い isNew を false にする（別名も参照する）\n")
	p.WriteString("- 部位や切り方は区別しない（例: 鶏もも肉・鶏むね肉 → 鶏肉、豚バラ肉 → 豚肉、玉ねぎのみじん切り → 玉ねぎ）\n")
	p.WriteString("- ひき肉は肉とは別の食材として扱う（例: 合いびき肉、鶏ひき肉）\n")
	p.WriteString("- 「あれば」「好みで」「お好みで」と書かれた材料、飾り・トッピング用の材料、水は含めない\n")
	p.WriteString("- 1行に複数の食材がある場合（例: 塩、こしょう）は別々の食材にする\n")
	p.WriteString("- 「タレ」「A」など見出しだけの行は無視する\n")
	p.WriteString("- 食材マスタにない食材は isNew を true にし、一般的な食材名（例: たまねぎ・玉葱 → 玉ねぎ）で name を付け、")
	p.WriteString("category を次から1つ選ぶ: " + strings.Join(ingredientCategories, "、") + "\n")
	p.WriteString("- 新しい食材が塩・こしょう・砂糖・しょうゆ・酢・みりん・酒・味噌・サラダ油・ごま油・オリーブオイル・小麦粉・片栗粉のような")
	p.WriteString("どの家庭にもある基本調味料なら isStaple を true にする\n")
	p.WriteString("- 新しい食材には、よくある別の表記を aliases に入れてよい（ひらがな表記など。なければ空配列）\n")
	p.WriteString("- 同じ食材を2回以上返さない\n\n")

	p.WriteString("【出力形式】JSONのみを返してください。説明文やコードブロックは不要です。\n")
	p.WriteString(`{"ingredients":[{"name":"玉ねぎ","isNew":false},{"name":"ズッキーニ","isNew":true,"category":"野菜","isStaple":false,"aliases":["ずっきーに"]}]}`)
	p.WriteString("\n")

	return p.String()
}

func parseIngredientResolution(responseText string) ([]resolvedIngredient, error) {
	extracted, err := extractJSON(responseText)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Ingredients []resolvedIngredient `json:"ingredients"`
	}
	if err := json.Unmarshal([]byte(extracted), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse ingredient resolution: %w", err)
	}
	return parsed.Ingredients, nil
}

// resolveIngredients はレシピの材料の行を食材マスタの正規名の一覧に変換する。
// マスタにない食材は新しく登録する。調味料も含めて返す（検索時に isStaple で除外する）。
func resolveIngredients(ctx context.Context, rawLines []string) ([]string, error) {
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return []string{}, nil
	}

	master, err := loadIngredientMaster(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load ingredient master: %w", err)
	}

	responseText, err := invokeLLM(ctx, buildIngredientResolutionPrompt(lines, master), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}
	resolved, err := parseIngredientResolution(responseText)
	if err != nil {
		return nil, err
	}

	lookup := buildIngredientLookup(master)
	names := make([]string, 0, len(resolved))
	seen := make(map[string]bool, len(resolved))
	for _, item := range resolved {
		key := canonicalizeIngredientName(item.Name)
		if key == "" {
			continue
		}

		name, known := lookup[key]
		if !known {
			// マスタにない → 新しい食材として登録する（AIが isNew=false と言っていても同じ扱い）
			ingredient := newIngredientFromResolution(key, item)
			if _, err := dbStore.CreateIngredientIfNotExists(ctx, ingredient); err != nil {
				return nil, fmt.Errorf("failed to create ingredient %q: %w", key, err)
			}
			addToIngredientMasterCache(ingredient)
			lookup[key] = key
			for _, alias := range ingredient.Aliases {
				if _, exists := lookup[alias]; !exists {
					lookup[alias] = key
				}
			}
			name = key
			log.Printf("Registered new ingredient: %s (%s, staple=%v)", key, ingredient.Category, ingredient.IsStaple)
		}

		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

func newIngredientFromResolution(name string, item resolvedIngredient) *Ingredient {
	category := item.Category
	if !isValidIngredientCategory(category) {
		category = "その他"
	}

	aliases := make([]string, 0, len(item.Aliases))
	seen := map[string]bool{name: true}
	for _, alias := range item.Aliases {
		key := canonicalizeIngredientName(alias)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		aliases = append(aliases, key)
	}

	return &Ingredient{
		Name:      name,
		Aliases:   aliases,
		Category:  category,
		IsStaple:  item.IsStaple,
		CreatedAt: time.Now(),
	}
}

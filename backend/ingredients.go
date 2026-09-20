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
	p.WriteString("- 豚肉・牛肉の部位や、切り方は区別しない（例: 豚バラ肉・豚こま切れ肉 → 豚肉、玉ねぎのみじん切り → 玉ねぎ）\n")
	p.WriteString("- ただし鶏肉は部位ごとに分ける。鶏むね肉（鶏胸肉も同じ）・鶏もも肉・鶏ささみ・手羽先・手羽元・手羽中のいずれかにし、")
	p.WriteString("部位が書かれていない場合だけ「鶏肉」にする。食材マスタに「鶏肉」があっても、部位が分かるならそれに対応づけてはいけない\n")
	p.WriteString("- 「鶏の煮汁」「ゆで汁」のように調理の途中でできるものは食材に含めない\n")
	p.WriteString("- ただし、ひき肉（挽き肉・ミンチ）は塊や薄切りの肉とは別の食材。豚ひき肉・鶏ひき肉・牛ひき肉・合いびき肉のどれかにする。")
	p.WriteString("食材マスタに「豚肉」などがあっても、ひき肉をそれに対応づけてはいけない。マスタに該当するひき肉が無ければ isNew を true にする\n")
	p.WriteString("- 「あれば」「好みで」「お好みで」と書かれた材料、飾り・トッピング用の材料、水は含めない\n")
	p.WriteString("- 1行に複数の食材がある場合（例: 塩、こしょう）は別々の食材にする\n")
	p.WriteString("- 「タレ」「A」など見出しだけの行は無視する\n")
	p.WriteString("- 食材マスタにない食材は isNew を true にし、一般的な食材名（例: たまねぎ・玉葱 → 玉ねぎ）で name を付け、")
	p.WriteString("category を次から1つ選ぶ: " + strings.Join(ingredientCategories, "、") + "\n")
	p.WriteString("- 新しい食材が塩・こしょう・砂糖・しょうゆ・酢・みりん・酒・味噌・サラダ油・ごま油・オリーブオイル・小麦粉・片栗粉・")
	p.WriteString("マヨネーズ・ケチャップ・だし（だし汁・顆粒だし・白だし）・ごま（いりごま・すりごま）・ソース（ウスター・中濃・オイスター）・米のような")
	p.WriteString("どの家庭にもある基本調味料なら isStaple を true にする（category は米なら穀物・麺、それ以外は調味料）\n")
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
	// 鶏肉の部位は材料の文字から決めるので、AIが返した鶏肉系の名前は使わない
	chickenParts := detectChickenParts(lines)
	chickenInserted := false
	for _, item := range resolved {
		key := canonicalizeIngredientName(item.Name)
		if key == "" {
			continue
		}

		if len(chickenParts) > 0 && isChickenName(key) {
			if !chickenInserted {
				chickenInserted = true
				for _, part := range chickenParts {
					if !seen[part] {
						seen[part] = true
						names = append(names, part)
					}
				}
			}
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

	// 鶏肉の部位はAIの判断に任せず、材料の文字から決める
	names = applyChickenParts(names, chickenParts)
	// 部位の食材がマスタに無ければ登録する。別名ではなく名前そのもので確認する
	// （「鶏肉」の別名に「鶏もも肉」が入っている、といった状態でも取りこぼさないため）
	masterNames := make(map[string]bool, len(master))
	for _, ingredient := range master {
		masterNames[ingredient.Name] = true
	}
	for _, name := range names {
		if masterNames[name] {
			continue
		}
		ingredient := &Ingredient{Name: name, Aliases: []string{}, Category: "肉", CreatedAt: time.Now()}
		if _, err := dbStore.CreateIngredientIfNotExists(ctx, ingredient); err != nil {
			return nil, fmt.Errorf("failed to create ingredient %q: %w", name, err)
		}
		addToIngredientMasterCache(ingredient)
		masterNames[name] = true
		log.Printf("Registered new chicken part: %s", name)
	}

	return names, nil
}

// --- 鶏肉の部位の判定（AI任せにせず、材料の文字から確実に決める） ---

// chickenPartRules は材料の行から鶏肉の部位を判定する規則。上から順に最初に当たったものを採用する。
// 「ささみ」「手羽」は鶏にしか使わないが、「むね」「もも」「スペアリブ」は
// 豚もも肉などと紛れるため、同じ行に鶏を表す語があるときだけ採用する。
var chickenPartRules = []struct {
	keywords     []string
	needsChicken bool
	name         string
}{
	{keywords: []string{"ささみ", "ササミ", "笹身"}, name: "鶏ささみ"},
	{keywords: []string{"手羽先"}, name: "手羽先"},
	{keywords: []string{"手羽元"}, name: "手羽元"},
	{keywords: []string{"手羽中"}, name: "手羽中"},
	{keywords: []string{"スペアリブ"}, needsChicken: true, name: "手羽中"},
	{keywords: []string{"むね", "ムネ", "胸"}, needsChicken: true, name: "鶏むね肉"},
	{keywords: []string{"もも", "モモ"}, needsChicken: true, name: "鶏もも肉"},
}

var chickenWords = []string{"鶏", "鳥", "とり", "チキン"}

func containsAny(s string, candidates []string) bool {
	for _, c := range candidates {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}

// detectChickenParts は材料の行から鶏肉の部位名を拾う（重複なし・登場順）
func detectChickenParts(rawLines []string) []string {
	parts := make([]string, 0)
	seen := make(map[string]bool)
	for _, line := range rawLines {
		l := canonicalizeIngredientName(line)
		// ひき肉は部位ではなく別の食材として扱う
		if strings.Contains(l, "ひき肉") || strings.Contains(l, "挽肉") || strings.Contains(l, "ミンチ") {
			continue
		}
		isChicken := containsAny(l, chickenWords)
		for _, rule := range chickenPartRules {
			if rule.needsChicken && !isChicken {
				continue
			}
			if containsAny(l, rule.keywords) {
				if !seen[rule.name] {
					seen[rule.name] = true
					parts = append(parts, rule.name)
				}
				break
			}
		}
	}
	return parts
}

// isChickenName は「鶏肉」やその部位を指す名前かどうかを判定する。
// AIが返した部位名（例: 鶏手羽元）も、こちらの判定結果で置き換えるために使う
func isChickenName(name string) bool {
	n := canonicalizeIngredientName(name)
	if n == "鶏肉" || strings.Contains(n, "手羽") || strings.Contains(n, "ささみ") || strings.Contains(n, "ササミ") {
		return true
	}
	if strings.Contains(n, "ひき肉") || strings.Contains(n, "挽肉") {
		return false // 鶏ひき肉は部位ではない
	}
	return containsAny(n, chickenWords) && containsAny(n, []string{"むね", "ムネ", "胸", "もも", "モモ"})
}

// applyChickenParts は「鶏肉」やAIが返した部位名を、材料から判定した部位に置き換える
func applyChickenParts(names []string, parts []string) []string {
	if len(parts) == 0 {
		return names
	}
	out := make([]string, 0, len(names)+len(parts))
	replaced := false
	for _, name := range names {
		if isChickenName(name) {
			if !replaced {
				out = append(out, parts...)
				replaced = true
			}
			continue
		}
		out = append(out, name)
	}
	if !replaced {
		// AIが「鶏肉」を返さなかった場合でも、判定した部位は必ず含める
		out = append(out, parts...)
	}

	deduped := make([]string, 0, len(out))
	seen := make(map[string]bool, len(out))
	for _, name := range out {
		if !seen[name] {
			seen[name] = true
			deduped = append(deduped, name)
		}
	}
	return deduped
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
		// 鶏肉の部位を別名にしない（「鶏肉」の別名に「鶏もも肉」が入ると、
		// 検索でもも肉を入力しても総称の鶏肉に寄ってしまうため）
		if parts := detectChickenParts([]string{key}); len(parts) > 0 && parts[0] != name {
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

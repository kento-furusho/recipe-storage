package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

// stubLLM は invokeLLM を固定の応答に差し替え、受け取ったプロンプトを記録する
func stubLLM(response string, err error) *[]string {
	prompts := &[]string{}
	invokeLLM = func(_ context.Context, prompt string, _ []*ImageData) (string, error) {
		*prompts = append(*prompts, prompt)
		return response, err
	}
	return prompts
}

func TestCanonicalizeIngredientName(t *testing.T) {
	tests := map[string]string{
		"  玉ねぎ ": "玉ねぎ",
		"鶏 もも　肉": "鶏もも肉",
		"ﾄﾏﾄ":    "トマト",  // 半角カナ → 全角
		"ＡＢＣ缶":   "ABC缶", // 全角英字 → 半角
		"":       "",
	}
	for in, want := range tests {
		if got := canonicalizeIngredientName(in); got != want {
			t.Errorf("canonicalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildIngredientResolutionPrompt(t *testing.T) {
	master := []*Ingredient{
		{Name: "玉ねぎ", Aliases: []string{"たまねぎ"}, Category: "野菜"},
		{Name: "塩", Category: "調味料", IsStaple: true},
	}
	prompt := buildIngredientResolutionPrompt([]string{"玉ねぎのみじん切り: 1/2個", "塩、こしょう: 適宜"}, master)

	for _, want := range []string{
		"玉ねぎ / 別名: たまねぎ / 野菜",
		"塩 / 調味料（基本調味料）",
		"- 玉ねぎのみじん切り: 1/2個",
		"- 塩、こしょう: 適宜",
		"豚バラ肉・豚こま切れ肉 → 豚肉",
		"鶏むね肉（鶏胸肉も同じ）・鶏もも肉・鶏ささみ・手羽先・手羽元・手羽中",
		"部位が分かるならそれに対応づけてはいけない",
		"ひき肉をそれに対応づけてはいけない",
		"マヨネーズ・ケチャップ・だし",
		"isStaple",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not contain %q", want)
		}
	}
}

func TestParseIngredientResolution(t *testing.T) {
	text := "```json\n{\"ingredients\":[{\"name\":\"玉ねぎ\",\"isNew\":false},{\"name\":\"ズッキーニ\",\"isNew\":true,\"category\":\"野菜\"}]}\n```"
	got, err := parseIngredientResolution(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Name != "玉ねぎ" || !got[1].IsNew || got[1].Category != "野菜" {
		t.Errorf("parsed = %+v", got)
	}
	if _, err := parseIngredientResolution("not json"); err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestResolveIngredients(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	mock.ingredients["玉ねぎ"] = &Ingredient{Name: "玉ねぎ", Aliases: []string{"たまねぎ"}, Category: "野菜"}
	mock.ingredients["塩"] = &Ingredient{Name: "塩", Category: "調味料", IsStaple: true}

	prompts := stubLLM(`{"ingredients":[
		{"name":"玉ねぎ","isNew":false},
		{"name":"たまねぎ","isNew":true,"category":"野菜"},
		{"name":"ズッキーニ","isNew":true,"category":"野菜","aliases":["ずっきーに","ズッキーニ",""]},
		{"name":"鶏肉","isNew":false},
		{"name":"ﾍﾞｰｺﾝ","isNew":true,"category":"謎の分類"},
		{"name":"塩","isNew":false},
		{"name":"  ","isNew":true},
		{"name":"玉ねぎ","isNew":false}
	]}`, nil)

	names, err := resolveIngredients(context.Background(), []string{"玉ねぎ: 1個", "", "ズッキーニ: 1本", "鶏もも肉: 1枚", "ベーコン: 2枚", "塩: 少々"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 材料に「鶏もも肉」とあるので、AIが「鶏肉」と答えても部位に直す
	want := []string{"玉ねぎ", "ズッキーニ", "鶏もも肉", "ベーコン", "塩"}
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if len(*prompts) != 1 || strings.Contains((*prompts)[0], "- \n") {
		t.Errorf("expected exactly one prompt without blank lines, got %d", len(*prompts))
	}

	// 別名「たまねぎ」は既存の玉ねぎに寄せ、新規登録しない
	if _, exists := mock.ingredients["たまねぎ"]; exists {
		t.Error("alias たまねぎ should not be registered as a new ingredient")
	}
	zucchini := mock.ingredients["ズッキーニ"]
	if zucchini == nil || zucchini.Category != "野菜" || !slices.Equal(zucchini.Aliases, []string{"ずっきーに"}) {
		t.Errorf("ズッキーニ = %+v", zucchini)
	}
	// 部位に直した食材もマスタに登録される
	if thigh := mock.ingredients["鶏もも肉"]; thigh == nil || thigh.Category != "肉" {
		t.Errorf("鶏もも肉 = %+v", thigh)
	}
	if bacon := mock.ingredients["ベーコン"]; bacon == nil || bacon.Category != "その他" {
		t.Errorf("ベーコン (from half-width kana) = %+v", bacon)
	}
	// 玉ねぎ・塩・ズッキーニ・ベーコン・鶏もも肉。AIが答えた総称の「鶏肉」は登録しない
	if _, exists := mock.ingredients["鶏肉"]; exists {
		t.Error("部位が分かる場合、総称の鶏肉は登録しない")
	}
	if len(mock.ingredients) != 5 {
		t.Errorf("master has %d ingredients, want 5", len(mock.ingredients))
	}
}

func TestResolveIngredients_NoLinesSkipsLLM(t *testing.T) {
	_, cleanup := setupTest()
	defer cleanup()
	prompts := stubLLM("", fmt.Errorf("should not be called"))

	names, err := resolveIngredients(context.Background(), []string{"", "  "})
	if err != nil || names == nil || len(names) != 0 || len(*prompts) != 0 {
		t.Errorf("names=%v err=%v prompts=%d; want empty, nil, 0", names, err, len(*prompts))
	}
}

func TestResolveIngredients_LLMError(t *testing.T) {
	_, cleanup := setupTest()
	defer cleanup()
	stubLLM("", fmt.Errorf("bedrock down"))

	if _, err := resolveIngredients(context.Background(), []string{"玉ねぎ"}); err == nil {
		t.Error("expected error when LLM fails")
	}
}

// nil（未処理）と空配列（食材なし）の区別がDynamoDBの保存・読み込みで保たれること
func TestRecipeIngredientNamesNilVsEmptyRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []string
		wantNil bool
	}{
		{"nil", nil, true},
		{"empty", []string{}, false},
		{"values", []string{"玉ねぎ"}, false},
	} {
		item, err := attributevalue.MarshalMap(&Recipe{RecipeID: "r1", IngredientNames: tc.in})
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		var out Recipe
		if err := attributevalue.UnmarshalMap(item, &out); err != nil {
			t.Fatalf("%s: unmarshal: %v", tc.name, err)
		}
		if (out.IngredientNames == nil) != tc.wantNil || len(out.IngredientNames) != len(tc.in) {
			t.Errorf("%s: round trip = %#v", tc.name, out.IngredientNames)
		}
	}
}

func TestHandleUpdateRecipe_ResolvesIngredientsOnlyWhenChanged(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	mock.recipes["r1"] = &Recipe{
		RecipeID: "r1", UserID: "user-1", Title: "t",
		Ingredients: []string{"玉ねぎ: 1個"}, Steps: []string{"s"},
		IngredientNames: []string{"玉ねぎ"},
	}

	update := func(body string) *Recipe {
		t.Helper()
		r := withChiURLParam(reqAsUser("PUT", "/recipes/r1", []byte(body), "user-1"), "recipeId", "r1")
		w := httptest.NewRecorder()
		handleUpdateRecipe(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		return mock.recipes["r1"]
	}

	// 材料が同じ → AIを呼ばず、既存の対応づけを残す
	prompts := stubLLM(`{"ingredients":[{"name":"鶏肉","isNew":true,"category":"肉"}]}`, nil)
	got := update(`{"title":"新しいタイトル","ingredients":["玉ねぎ: 1個"],"steps":["s"]}`)
	if len(*prompts) != 0 || !slices.Equal(got.IngredientNames, []string{"玉ねぎ"}) {
		t.Errorf("unchanged ingredients: prompts=%d names=%v", len(*prompts), got.IngredientNames)
	}

	// 材料が変わった → 対応づけをやり直す
	got = update(`{"title":"t","ingredients":["鶏もも肉: 1枚"],"steps":["s"]}`)
	if len(*prompts) != 1 || !slices.Equal(got.IngredientNames, []string{"鶏もも肉"}) {
		t.Errorf("changed ingredients: prompts=%d names=%v", len(*prompts), got.IngredientNames)
	}

	// AIが失敗 → 保存は成功し、古い対応づけは残さない
	stubLLM("", fmt.Errorf("bedrock down"))
	got = update(`{"title":"t","ingredients":["豚肉: 200g"],"steps":["s"]}`)
	if got.IngredientNames != nil || !slices.Equal(got.Ingredients, []string{"豚肉: 200g"}) {
		t.Errorf("LLM failure: names=%v ingredients=%v", got.IngredientNames, got.Ingredients)
	}
}

func TestDetectChickenParts(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{"もも肉", []string{"鶏もも肉: 大1枚"}, []string{"鶏もも肉"}},
		{"胸肉の表記ゆれ", []string{"鶏胸肉 1枚（250g）"}, []string{"鶏むね肉"}},
		{"むね肉", []string{"鶏むね肉: 1枚 (約300g)"}, []string{"鶏むね肉"}},
		{"ささみは鶏の語がなくてもよい", []string{"ささみ 2本"}, []string{"鶏ささみ"}},
		{"手羽元", []string{"鶏手羽元 12本"}, []string{"手羽元"}},
		{"鶏スペアリブは手羽中", []string{"鶏スペアリブ 8本（180g）"}, []string{"手羽中"}},
		{"豚もも肉は対象外", []string{"豚もも肉 200g"}, []string{}},
		{"豚スペアリブは対象外", []string{"豚スペアリブ: 8本"}, []string{}},
		{"鶏ひき肉は部位にしない", []string{"鶏ひき肉 150g"}, []string{}},
		{"部位なしの鶏肉", []string{"鶏肉 300g"}, []string{}},
		{"複数の部位", []string{"鶏もも肉 1枚", "鶏むね肉 1枚", "鶏もも肉 追加"}, []string{"鶏もも肉", "鶏むね肉"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectChickenParts(tt.lines); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyChickenParts(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		parts []string
		want  []string
	}{
		{"総称を部位に置き換える", []string{"玉ねぎ", "鶏肉", "塩"}, []string{"鶏もも肉"}, []string{"玉ねぎ", "鶏もも肉", "塩"}},
		{"AIが既に部位を返していれば変わらない", []string{"鶏もも肉", "塩"}, []string{"鶏もも肉"}, []string{"鶏もも肉", "塩"}},
		{"鶏肉が無ければ末尾に足す", []string{"玉ねぎ"}, []string{"手羽元"}, []string{"玉ねぎ", "手羽元"}},
		{"部位が2つなら両方に展開", []string{"鶏肉", "塩"}, []string{"鶏もも肉", "鶏むね肉"}, []string{"鶏もも肉", "鶏むね肉", "塩"}},
		{"部位が無ければそのまま", []string{"鶏肉"}, []string{}, []string{"鶏肉"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyChickenParts(tt.names, tt.parts); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// AIが「鶏肉」と答えても、材料の文字から部位に直してマスタにも登録する
func TestResolveIngredients_ChickenPartOverridesLLM(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	mock.ingredients["鶏肉"] = &Ingredient{Name: "鶏肉", Category: "肉"}
	mock.ingredients["塩"] = &Ingredient{Name: "塩", Category: "調味料", IsStaple: true}
	stubLLM(`{"ingredients":[{"name":"鶏肉","isNew":false},{"name":"塩","isNew":false}]}`, nil)

	names, err := resolveIngredients(context.Background(), []string{"鶏もも肉: 大1枚", "塩: 少々"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(names, []string{"鶏もも肉", "塩"}) {
		t.Fatalf("names = %v, want [鶏もも肉 塩]", names)
	}
	if got := mock.ingredients["鶏もも肉"]; got == nil || got.Category != "肉" {
		t.Errorf("鶏もも肉 in master = %+v", got)
	}
}

// AIが部位名（鶏手羽元など）を返しても、こちらの判定した部位に置き換えて重複させない
func TestResolveIngredients_ChickenPartNoDuplicate(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	stubLLM(`{"ingredients":[{"name":"鶏手羽元","isNew":true,"category":"肉"},{"name":"玉ねぎ","isNew":true,"category":"野菜"}]}`, nil)

	names, err := resolveIngredients(context.Background(), []string{"鶏手羽元 12本", "玉ねぎ 1個"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(names, []string{"手羽元", "玉ねぎ"}) {
		t.Errorf("names = %v, want [手羽元 玉ねぎ]", names)
	}
	if _, exists := mock.ingredients["鶏手羽元"]; exists {
		t.Error("AIが返した鶏手羽元をマスタに登録してはいけない")
	}
	if mock.ingredients["手羽元"] == nil {
		t.Error("手羽元 should be registered")
	}
}

// 「鶏肉」の別名に部位名を入れない（検索で部位が総称に寄るのを防ぐ）
func TestResolveIngredients_ChickenPartNotUsedAsAlias(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	stubLLM(`{"ingredients":[{"name":"鶏肉","isNew":true,"category":"肉","aliases":["とり肉","鶏もも肉"]}]}`, nil)

	if _, err := resolveIngredients(context.Background(), []string{"鶏肉 300g"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	chicken := mock.ingredients["鶏肉"]
	if chicken == nil || !slices.Equal(chicken.Aliases, []string{"とり肉"}) {
		t.Errorf("鶏肉 aliases = %+v, want [とり肉]", chicken)
	}
}

// マスタの別名に部位名が残っていても、部位の食材を取りこぼさず登録する
func TestResolveIngredients_RegistersPartEvenIfAliasedElsewhere(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	mock.ingredients["鶏肉"] = &Ingredient{Name: "鶏肉", Aliases: []string{"鶏もも肉"}, Category: "肉"}
	stubLLM(`{"ingredients":[{"name":"鶏肉","isNew":false}]}`, nil)

	names, err := resolveIngredients(context.Background(), []string{"鶏もも肉 1枚"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(names, []string{"鶏もも肉"}) {
		t.Errorf("names = %v, want [鶏もも肉]", names)
	}
	if mock.ingredients["鶏もも肉"] == nil {
		t.Error("鶏もも肉 should be registered in the master")
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func searchTestMaster() []*Ingredient {
	return []*Ingredient{
		{Name: "鶏肉", Category: "肉"},
		{Name: "玉ねぎ", Aliases: []string{"たまねぎ"}, Category: "野菜"},
		{Name: "トマト缶", Category: "加工品"},
		{Name: "ベーコン", Category: "加工品"},
		{Name: "にんにく", Category: "野菜"},
		{Name: "塩", Category: "調味料", IsStaple: true},
		{Name: "オリーブオイル", Category: "調味料", IsStaple: true},
	}
}

func searchTestRecipes() []*Recipe {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []*Recipe{
		{RecipeID: "tomato", Title: "チキントマト", CreatedAt: base,
			IngredientNames: []string{"鶏肉", "玉ねぎ", "トマト缶", "塩", "オリーブオイル"}},
		{RecipeID: "bacon", Title: "ベーコン炒め", CreatedAt: base.Add(time.Hour),
			IngredientNames: []string{"ベーコン", "にんにく", "玉ねぎ"}},
		{RecipeID: "both", Title: "鶏とベーコン", CreatedAt: base.Add(2 * time.Hour),
			IngredientNames: []string{"鶏肉", "ベーコン", "にんにく", "トマト缶"}},
		{RecipeID: "unprocessed", Title: "未処理", CreatedAt: base, IngredientNames: nil},
		{RecipeID: "salt", Title: "塩だけ", CreatedAt: base, IngredientNames: []string{"塩"}},
		// 手持ち（鶏肉・玉ねぎ・トマト缶）に対して3個足りない → 結果に出ない
		{RecipeID: "far", Title: "足りない多すぎ", CreatedAt: base,
			IngredientNames: []string{"鶏肉", "ベーコン", "にんにく", "じゃがいも"}},
	}
}

func resultIDs(results []IngredientSearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Recipe.RecipeID)
	}
	return ids
}

func TestSearchRecipesByIngredients(t *testing.T) {
	resp := searchRecipesByIngredients(searchTestRecipes(), searchTestMaster(),
		[]string{"鶏肉", "たまねぎ", "トマト缶", "オニオン"})

	// 今すぐ作れる → 足りない2個で一致2個 → 足りない2個で一致1個
	if got, want := resultIDs(resp.Results), []string{"tomato", "both", "bacon"}; !slices.Equal(got, want) {
		t.Fatalf("results = %v, want %v", got, want)
	}

	tomato := resp.Results[0]
	if len(tomato.Missing) != 0 || !slices.Equal(tomato.Matched, []string{"鶏肉", "玉ねぎ", "トマト缶"}) {
		t.Errorf("tomato: matched=%v missing=%v (staples must be ignored)", tomato.Matched, tomato.Missing)
	}
	if bacon := resp.Results[2]; !slices.Equal(bacon.Missing, []string{"ベーコン", "にんにく"}) {
		t.Errorf("bacon missing = %v", bacon.Missing)
	}
	if !slices.Equal(resp.UnknownInputs, []string{"オニオン"}) {
		t.Errorf("unknownInputs = %v", resp.UnknownInputs)
	}
	if resp.UnprocessedCount != 1 {
		t.Errorf("unprocessedCount = %d, want 1", resp.UnprocessedCount)
	}
}

func TestSearchRecipesByIngredients_TieBreakByNewest(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recipes := []*Recipe{
		{RecipeID: "old", CreatedAt: base, IngredientNames: []string{"鶏肉"}},
		{RecipeID: "new", CreatedAt: base.Add(time.Hour), IngredientNames: []string{"鶏肉"}},
	}
	resp := searchRecipesByIngredients(recipes, searchTestMaster(), []string{"鶏肉"})
	if got := resultIDs(resp.Results); !slices.Equal(got, []string{"new", "old"}) {
		t.Errorf("results = %v, want [new old]", got)
	}
}

func TestSearchRecipesByIngredients_EmptyResultsAreNotNil(t *testing.T) {
	resp := searchRecipesByIngredients(nil, searchTestMaster(), []string{"鶏肉"})
	b, _ := json.Marshal(resp)
	if !strings.Contains(string(b), `"results":[]`) || !strings.Contains(string(b), `"unknownInputs":[]`) {
		t.Errorf("json = %s", b)
	}
}

func TestHandleSearchRecipesByIngredients(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	for _, ingredient := range searchTestMaster() {
		mock.ingredients[ingredient.Name] = ingredient
	}
	for _, recipe := range searchTestRecipes() {
		recipe.UserID = "user-1"
		mock.recipes[recipe.RecipeID] = recipe
	}
	// 他のユーザーのレシピは結果に出ない
	mock.recipes["other"] = &Recipe{RecipeID: "other", UserID: "user-2", IngredientNames: []string{"鶏肉"}}

	r := reqAsUser("POST", "/recipes/search-by-ingredients", []byte(`{"ingredients":["鶏肉"," 鶏肉 ",""]}`), "user-1")
	w := httptest.NewRecorder()
	handleSearchRecipesByIngredients(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp IngredientSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, result := range resp.Results {
		if result.Recipe.UserID != "user-1" {
			t.Errorf("got another user's recipe %s", result.Recipe.RecipeID)
		}
	}
	if len(resp.Results) == 0 {
		t.Error("expected results for 鶏肉")
	}
}

func TestHandleSearchRecipesByIngredients_Validation(t *testing.T) {
	_, cleanup := setupTest()
	defer cleanup()

	tooMany := make([]string, maxSearchIngredients+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf(`"食材%d"`, i)
	}
	for name, body := range map[string]string{
		"invalid json": `not json`,
		"empty":        `{"ingredients":[]}`,
		"only blanks":  `{"ingredients":["", "  "]}`,
		"too long":     `{"ingredients":["` + strings.Repeat("あ", maxIngredientInputLen+1) + `"]}`,
		"too many":     `{"ingredients":[` + strings.Join(tooMany, ",") + `]}`,
	} {
		r := reqAsUser("POST", "/recipes/search-by-ingredients", []byte(body), "user-1")
		w := httptest.NewRecorder()
		handleSearchRecipesByIngredients(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, w.Code)
		}
	}
}

func TestHandleGetIngredients(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	for _, ingredient := range searchTestMaster() {
		mock.ingredients[ingredient.Name] = ingredient
	}
	for _, recipe := range searchTestRecipes() {
		recipe.UserID = "user-1"
		mock.recipes[recipe.RecipeID] = recipe
	}

	r := reqAsUser("GET", "/ingredients", nil, "user-1")
	w := httptest.NewRecorder()
	handleGetIngredients(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Ingredients []IngredientListItem `json:"ingredients"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	names := make([]string, 0, len(resp.Ingredients))
	for _, item := range resp.Ingredients {
		names = append(names, item.Name)
		if item.Name == "塩" || item.Name == "オリーブオイル" {
			t.Errorf("staple %s should be excluded", item.Name)
		}
	}
	// 使用回数の多い順（鶏肉・玉ねぎ・トマト缶・ベーコン・にんにく はそれぞれの使用回数で並ぶ）
	if resp.Ingredients[0].UsageCount < resp.Ingredients[len(resp.Ingredients)-1].UsageCount {
		t.Errorf("not sorted by usage: %+v", resp.Ingredients)
	}
	if len(names) != 5 {
		t.Errorf("ingredients = %v, want 5 non-staple items", names)
	}
}

func keywordTestRecipes() []*Recipe {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []*Recipe{
		{RecipeID: "teriyaki", UserID: "user-1", Title: "鶏の照り焼き", CreatedAt: base.Add(2 * time.Hour),
			Ingredients: []string{"鶏もも肉 1枚（300g）", "しょうゆ 大さじ1"}, CategoryIDs: []string{"cat-a"}},
		{RecipeID: "salad", UserID: "user-1", Title: "刻み野菜のサラダ", CreatedAt: base.Add(time.Hour),
			Ingredients: []string{"きゅうり: 1本", "ＡＢＣトマト: 2個"}},
		{RecipeID: "mapo", UserID: "user-1", Title: "麻婆なす", CreatedAt: base,
			Ingredients: []string{"なす 3～4本", "豚ひき肉 150g"}, CategoryIDs: []string{"cat-a", "cat-b"}},
	}
}

func TestFilterRecipesByKeyword(t *testing.T) {
	recipes := keywordTestRecipes()
	tests := []struct {
		name     string
		keyword  string
		category string
		want     []string
	}{
		{"レシピ名で一致", "照り焼き", "", []string{"teriyaki"}},
		{"材料で一致", "鶏もも", "", []string{"teriyaki"}},
		{"複数一致は元の並び順", "肉", "", []string{"teriyaki", "mapo"}},
		{"空白の違いを吸収", "豚 ひき肉", "", []string{"mapo"}},
		{"全角と半角の違いを吸収", "abcトマト", "", []string{"salad"}},
		{"カテゴリと併用", "肉", "cat-b", []string{"mapo"}},
		{"一致なし", "カレー", "", []string{}},
		{"キーワードなしはカテゴリだけで絞る", "", "cat-a", []string{"teriyaki", "mapo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]string, 0)
			for _, r := range filterRecipesByKeyword(recipes, tt.keyword, tt.category) {
				got = append(got, r.RecipeID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleGetRecipes_KeywordSearch(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()
	for _, recipe := range keywordTestRecipes() {
		mock.recipes[recipe.RecipeID] = recipe
	}
	mock.recipes["other"] = &Recipe{RecipeID: "other", UserID: "user-2", Title: "他人の照り焼き",
		Ingredients: []string{"鶏もも肉"}}

	// q だけでもページ形式（{items, nextCursor}）で返る
	page := getRecipePage(t, "/recipes?q=照り焼き")
	if len(page.Items) != 1 || page.Items[0].RecipeID != "teriyaki" {
		t.Fatalf("items = %+v", page.Items)
	}
	if page.NextCursor != nil {
		t.Errorf("nextCursor = %v, want nil (検索はページ送りしない)", *page.NextCursor)
	}

	// 一致なしでも items は空配列
	empty := getRecipePage(t, "/recipes?limit=10&q=存在しない食材")
	if empty.Items == nil || len(empty.Items) != 0 {
		t.Errorf("items = %#v, want []", empty.Items)
	}

	// 空白だけの q は検索とみなさず、従来どおりページ送りになる
	all := getRecipePage(t, "/recipes?limit=2&q=%20%20")
	if len(all.Items) != 2 || all.NextCursor == nil {
		t.Errorf("blank q: items=%d nextCursor=%v; want 2 items and a cursor", len(all.Items), all.NextCursor)
	}
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxSearchIngredients  = 30
	maxIngredientInputLen = 30
	// 足りない食材がこの数以下のレシピまで結果に出す
	maxMissingIngredients = 2
)

// normalizeForKeyword は全角/半角・大文字小文字・空白の違いを吸収する（検索語と対象の両方に使う）
func normalizeForKeyword(s string) string {
	return strings.ToLower(canonicalizeIngredientName(s))
}

// filterRecipesByKeyword はレシピ名と材料（元の文字列）へのキーワード部分一致で絞り込む。
// categoryID を指定するとそのカテゴリのレシピだけにする。並び順は渡された順のまま。
func filterRecipesByKeyword(recipes []*Recipe, keyword, categoryID string) []*Recipe {
	key := normalizeForKeyword(keyword)
	matched := make([]*Recipe, 0)
	for _, recipe := range recipes {
		if categoryID != "" && !slices.Contains(recipe.CategoryIDs, categoryID) {
			continue
		}
		if key != "" && !recipeMatchesKeyword(recipe, key) {
			continue
		}
		matched = append(matched, recipe)
	}
	return matched
}

func recipeMatchesKeyword(recipe *Recipe, normalizedKeyword string) bool {
	if strings.Contains(normalizeForKeyword(recipe.Title), normalizedKeyword) {
		return true
	}
	for _, ingredient := range recipe.Ingredients {
		if strings.Contains(normalizeForKeyword(ingredient), normalizedKeyword) {
			return true
		}
	}
	return false
}

type IngredientSearchRequest struct {
	Ingredients []string `json:"ingredients"`
}

type IngredientSearchResult struct {
	Recipe  *Recipe  `json:"recipe"`
	Missing []string `json:"missing"`
	Matched []string `json:"matched"`
}

type IngredientSearchResponse struct {
	Results []IngredientSearchResult `json:"results"`
	// 食材マスタの名前・別名のどれにも当たらなかった入力
	UnknownInputs []string `json:"unknownInputs"`
	// 食材の対応づけがまだ済んでいないレシピの数
	UnprocessedCount int `json:"unprocessedCount"`
}

type IngredientListItem struct {
	Name       string   `json:"name"`
	Aliases    []string `json:"aliases"`
	Category   string   `json:"category"`
	UsageCount int      `json:"usageCount"`
}

// searchRecipesByIngredients は手持ちの食材で作れる（あと少しで作れる）レシピを探す。AIは使わない。
func searchRecipesByIngredients(recipes []*Recipe, master []*Ingredient, inputs []string) IngredientSearchResponse {
	lookup := buildIngredientLookup(master)
	staples := make(map[string]bool)
	for _, ingredient := range master {
		if ingredient.IsStaple {
			staples[ingredient.Name] = true
		}
	}

	have := make(map[string]bool)
	unknown := make([]string, 0)
	for _, input := range inputs {
		if name, ok := lookup[canonicalizeIngredientName(input)]; ok {
			have[name] = true
		} else {
			unknown = append(unknown, input)
		}
	}

	results := make([]IngredientSearchResult, 0)
	unprocessed := 0
	for _, recipe := range recipes {
		if recipe.IngredientNames == nil {
			unprocessed++
			continue
		}
		matched := make([]string, 0)
		missing := make([]string, 0)
		seen := make(map[string]bool)
		for _, name := range recipe.IngredientNames {
			if staples[name] || seen[name] {
				continue
			}
			seen[name] = true
			if have[name] {
				matched = append(matched, name)
			} else {
				missing = append(missing, name)
			}
		}
		if len(matched) == 0 || len(missing) > maxMissingIngredients {
			continue
		}
		results = append(results, IngredientSearchResult{Recipe: recipe, Missing: missing, Matched: matched})
	}

	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if len(a.Missing) != len(b.Missing) {
			return len(a.Missing) < len(b.Missing)
		}
		if len(a.Matched) != len(b.Matched) {
			return len(a.Matched) > len(b.Matched)
		}
		return a.Recipe.CreatedAt.After(b.Recipe.CreatedAt)
	})

	return IngredientSearchResponse{Results: results, UnknownInputs: unknown, UnprocessedCount: unprocessed}
}

// normalizeSearchInputs は入力を整え、検証する。問題があればエラーメッセージを返す
func normalizeSearchInputs(raw []string) ([]string, string) {
	inputs := make([]string, 0, len(raw))
	seen := make(map[string]bool)
	for _, s := range raw {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		if utf8.RuneCountInString(trimmed) > maxIngredientInputLen {
			return nil, "ingredient name too long"
		}
		key := canonicalizeIngredientName(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		inputs = append(inputs, trimmed)
	}
	if len(inputs) == 0 {
		return nil, "ingredients required"
	}
	if len(inputs) > maxSearchIngredients {
		return nil, "too many ingredients"
	}
	return inputs, ""
}

// handleSearchRecipesByIngredients は POST /recipes/search-by-ingredients
func handleSearchRecipesByIngredients(w http.ResponseWriter, r *http.Request) {
	var req IngredientSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	inputs, problem := normalizeSearchInputs(req.Ingredients)
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	recipes, err := dbStore.ListRecipes(ctx, getUserID(r))
	if err != nil {
		log.Printf("Failed to list recipes for ingredient search: %v", err)
		http.Error(w, "failed to search recipes", http.StatusInternalServerError)
		return
	}
	master, err := loadIngredientMaster(ctx)
	if err != nil {
		log.Printf("Failed to load ingredient master: %v", err)
		http.Error(w, "failed to search recipes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(searchRecipesByIngredients(recipes, master, inputs))
}

// handleGetIngredients は GET /ingredients。調味料以外の食材と、このユーザーのレシピでの使用回数を返す
func handleGetIngredients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	master, err := loadIngredientMaster(ctx)
	if err != nil {
		log.Printf("Failed to load ingredient master: %v", err)
		http.Error(w, "failed to list ingredients", http.StatusInternalServerError)
		return
	}
	recipes, err := dbStore.ListRecipes(ctx, getUserID(r))
	if err != nil {
		log.Printf("Failed to list recipes for ingredient usage: %v", err)
		http.Error(w, "failed to list ingredients", http.StatusInternalServerError)
		return
	}

	usage := make(map[string]int)
	for _, recipe := range recipes {
		for _, name := range recipe.IngredientNames {
			usage[name]++
		}
	}

	items := make([]IngredientListItem, 0, len(master))
	for _, ingredient := range master {
		if ingredient.IsStaple {
			continue
		}
		aliases := ingredient.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		items = append(items, IngredientListItem{
			Name:       ingredient.Name,
			Aliases:    aliases,
			Category:   ingredient.Category,
			UsageCount: usage[ingredient.Name],
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UsageCount != items[j].UsageCount {
			return items[i].UsageCount > items[j].UsageCount
		}
		return items[i].Name < items[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]IngredientListItem{"ingredients": items})
}

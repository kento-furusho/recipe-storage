package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestIsBlockedRecipeSite(t *testing.T) {
	tests := map[string]bool{
		"https://www.buzzfeed.com/jp/tasty/recipe": true,
		"https://buzzfeed.com/foo":                 true,
		"https://BuzzFeed.com/foo":                 true,
		" https://www.buzzfeed.com/foo ":           true,
		"https://note.com/travelingfoodlab/n/abc":  false,
		"https://www.kurashiru.com/recipes/abc":    false,
		"https://notbuzzfeed.com/foo":              false, // 別ドメイン
		"https://buzzfeed.com.example.com/foo":     false, // なりすまし
		"https://www.youtube.com/watch?v=abc":      false,
		"not a url":                                false,
	}
	for url, want := range tests {
		if got := isBlockedRecipeSite(url); got != want {
			t.Errorf("isBlockedRecipeSite(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestHandleCreateJob_BlockedRecipeSite(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	// URLだけ → ページを取りに行くので受け付けない（ジョブも作らない）
	r := reqAsUser("POST", "/jobs", []byte(`{"imageKeys":[],"sourceUrl":"https://www.buzzfeed.com/jp/recipe"}`), "user-1")
	w := httptest.NewRecorder()
	handleCreateJob(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "利用規約で自動での取得が禁止") {
		t.Errorf("body = %q", w.Body.String())
	}
	if len(mock.jobs) != 0 {
		t.Errorf("job should not be created, got %d", len(mock.jobs))
	}
}

// 受付前に積まれたジョブでも、ページを取得せず、再作成対象の既存レシピを上書きしない
func TestProcessJob_BlockedRecipeSiteKeepsExistingRecipe(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	url := "https://www.buzzfeed.com/jp/recipe"
	mock.recipes["r1"] = &Recipe{RecipeID: "r1", UserID: "user-1", Title: "元のレシピ",
		Ingredients: []string{"なす"}, Steps: []string{"焼く"}, SourceURL: &url}
	target := "r1"
	mock.jobs["j1"] = &Job{JobID: "j1", UserID: "user-1", Status: JobStatusPending,
		ImageKeys: []string{}, SourceURL: &url, TargetRecipeID: &target}

	processJob("j1")

	if got := mock.jobs["j1"].Status; got != JobStatusFailed {
		t.Errorf("job status = %s, want FAILED", got)
	}
	r := mock.recipes["r1"]
	if r.Title != "元のレシピ" || !slices.Equal(r.Steps, []string{"焼く"}) {
		t.Errorf("existing recipe was overwritten: %+v", r)
	}
	if len(mock.recipes) != 1 {
		t.Errorf("no new recipe should be created, got %d", len(mock.recipes))
	}
}

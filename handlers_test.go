package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
	"github.com/go-chi/chi/v5"
)

// mockStore implements Store for testing.
type mockStore struct {
	jobs       map[string]*Job
	recipes    map[string]*Recipe
	categories map[string]*Category
}

func newMockStore() *mockStore {
	return &mockStore{
		jobs:       make(map[string]*Job),
		recipes:    make(map[string]*Recipe),
		categories: make(map[string]*Category),
	}
}

func (m *mockStore) SaveJob(_ context.Context, job *Job) error {
	m.jobs[job.JobID] = job
	return nil
}

func (m *mockStore) GetJob(_ context.Context, jobID string) (*Job, error) {
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, nil
	}
	return job, nil
}

func (m *mockStore) SaveRecipe(_ context.Context, recipe *Recipe) error {
	m.recipes[recipe.RecipeID] = recipe
	return nil
}

func (m *mockStore) GetRecipe(_ context.Context, recipeID string) (*Recipe, error) {
	recipe, ok := m.recipes[recipeID]
	if !ok {
		return nil, nil
	}
	return recipe, nil
}

func (m *mockStore) DeleteRecipe(_ context.Context, recipeID string) error {
	delete(m.recipes, recipeID)
	return nil
}

func (m *mockStore) ListRecipes(_ context.Context, userID string) ([]*Recipe, error) {
	var result []*Recipe
	for _, r := range m.recipes {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockStore) SaveCategory(_ context.Context, category *Category) error {
	m.categories[category.CategoryID] = category
	return nil
}

func (m *mockStore) GetCategory(_ context.Context, categoryID string) (*Category, error) {
	cat, ok := m.categories[categoryID]
	if !ok {
		return nil, nil
	}
	return cat, nil
}

func (m *mockStore) DeleteCategory(_ context.Context, categoryID string) error {
	delete(m.categories, categoryID)
	return nil
}

func (m *mockStore) ListCategories(_ context.Context, userID string) ([]*Category, error) {
	var result []*Category
	for _, c := range m.categories {
		if c.UserID == userID {
			result = append(result, c)
		}
	}
	return result, nil
}

// Helper: create an HTTP request as if it comes from a specific Cognito user.
func reqAsUser(method, path string, body []byte, userID string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Content-Type", "application/json")

	if userID == "" {
		return r // local / unauthenticated
	}

	accessor := core.RequestAccessor{}
	apiGwReq := events.APIGatewayProxyRequest{
		HTTPMethod: method,
		Path:       path,
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"claims": map[string]interface{}{
					"sub": userID,
				},
			},
		},
	}
	ctxReq, _ := accessor.EventToRequestWithContext(context.Background(), apiGwReq)
	// Merge the API Gateway context into our test request.
	return r.WithContext(ctxReq.Context())
}

// withChiURLParam injects chi URL params into the request context.
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// setupTest replaces the global dbStore with a mock and returns it.
// The caller should defer the cleanup returned.
func setupTest() (*mockStore, func()) {
	mock := newMockStore()
	origStore := dbStore
	dbStore = mock
	return mock, func() { dbStore = origStore }
}

// --- Tests for IDOR ownership checks ---

func TestHandleGetJob_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.jobs["job-1"] = &Job{
		JobID:  "job-1",
		UserID: "user-owner",
		Status: JobStatusPending,
	}

	tests := []struct {
		name       string
		userID     string
		jobID      string
		wantStatus int
	}{
		{"owner can access", "user-owner", "job-1", http.StatusOK},
		{"other user blocked", "user-other", "job-1", http.StatusNotFound},
		{"nonexistent job", "user-owner", "job-999", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reqAsUser("GET", "/jobs/"+tt.jobID, nil, tt.userID)
			r = withChiURLParam(r, "jobId", tt.jobID)
			w := httptest.NewRecorder()
			handleGetJob(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleGetRecipe_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["recipe-1"] = &Recipe{
		RecipeID:    "recipe-1",
		UserID:      "user-owner",
		Title:       "Test",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
	}

	tests := []struct {
		name       string
		userID     string
		recipeID   string
		wantStatus int
	}{
		{"owner can access", "user-owner", "recipe-1", http.StatusOK},
		{"other user blocked", "user-other", "recipe-1", http.StatusNotFound},
		{"nonexistent recipe", "user-owner", "recipe-999", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reqAsUser("GET", "/recipes/"+tt.recipeID, nil, tt.userID)
			r = withChiURLParam(r, "recipeId", tt.recipeID)
			w := httptest.NewRecorder()
			handleGetRecipe(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleDeleteRecipe_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	// s3Client is needed for delete but we only test the ownership guard,
	// so we seed a recipe with no ImageKeys.
	mock.recipes["recipe-1"] = &Recipe{
		RecipeID:    "recipe-1",
		UserID:      "user-owner",
		Title:       "Test",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
	}

	tests := []struct {
		name       string
		userID     string
		recipeID   string
		wantStatus int
	}{
		{"owner can delete", "user-owner", "recipe-1", http.StatusNoContent},
		{"other user blocked", "user-other", "recipe-1", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-seed the recipe for each sub-test since delete removes it.
			mock.recipes["recipe-1"] = &Recipe{
				RecipeID:    "recipe-1",
				UserID:      "user-owner",
				Title:       "Test",
				Ingredients: []string{"a"},
				Steps:       []string{"b"},
				ImageKeys:   []string{},
			}

			r := reqAsUser("DELETE", "/recipes/"+tt.recipeID, nil, tt.userID)
			r = withChiURLParam(r, "recipeId", tt.recipeID)
			w := httptest.NewRecorder()
			handleDeleteRecipe(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleUpdateRecipe_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["recipe-1"] = &Recipe{
		RecipeID:    "recipe-1",
		UserID:      "user-owner",
		Title:       "Original",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
	}

	body, _ := json.Marshal(UpdateRecipeRequest{
		Title:       "Updated",
		Ingredients: []string{"c"},
		Steps:       []string{"d"},
	})

	tests := []struct {
		name       string
		userID     string
		recipeID   string
		wantStatus int
	}{
		{"owner can update", "user-owner", "recipe-1", http.StatusOK},
		{"other user blocked", "user-other", "recipe-1", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reqAsUser("PUT", "/recipes/"+tt.recipeID, body, tt.userID)
			r = withChiURLParam(r, "recipeId", tt.recipeID)
			w := httptest.NewRecorder()
			handleUpdateRecipe(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleUpdateCategory_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1",
		UserID:     "user-owner",
		Name:       "Dinner",
		Color:      "#FF0000",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	body, _ := json.Marshal(UpdateCategoryRequest{
		Name:  "Lunch",
		Color: "#00FF00",
	})

	tests := []struct {
		name       string
		userID     string
		categoryID string
		wantStatus int
	}{
		{"owner can update", "user-owner", "cat-1", http.StatusOK},
		{"other user blocked", "user-other", "cat-1", http.StatusNotFound},
		{"nonexistent category", "user-owner", "cat-999", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reqAsUser("PUT", "/categories/"+tt.categoryID, body, tt.userID)
			r = withChiURLParam(r, "categoryId", tt.categoryID)
			w := httptest.NewRecorder()
			handleUpdateCategory(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleDeleteCategory_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	tests := []struct {
		name       string
		userID     string
		categoryID string
		wantStatus int
	}{
		{"owner can delete", "user-owner", "cat-1", http.StatusNoContent},
		{"other user blocked", "user-other", "cat-1", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-seed for each sub-test.
			mock.categories["cat-1"] = &Category{
				CategoryID: "cat-1",
				UserID:     "user-owner",
				Name:       "Dinner",
				Color:      "#FF0000",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			r := reqAsUser("DELETE", "/categories/"+tt.categoryID, nil, tt.userID)
			r = withChiURLParam(r, "categoryId", tt.categoryID)
			w := httptest.NewRecorder()
			handleDeleteCategory(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleBulkAssignCategories_OwnershipCheck(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1",
		UserID:     "user-owner",
		Name:       "Dinner",
		Color:      "#FF0000",
	}
	mock.recipes["recipe-owner"] = &Recipe{
		RecipeID:    "recipe-owner",
		UserID:      "user-owner",
		Title:       "My Recipe",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
	}
	mock.recipes["recipe-other"] = &Recipe{
		RecipeID:    "recipe-other",
		UserID:      "user-other",
		Title:       "Other Recipe",
		Ingredients: []string{"x"},
		Steps:       []string{"y"},
		ImageKeys:   []string{},
	}

	body, _ := json.Marshal(BulkAssignCategoriesRequest{
		RecipeIDs:   []string{"recipe-owner", "recipe-other"},
		CategoryIDs: []string{"cat-1"},
	})

	r := reqAsUser("POST", "/recipes/bulk-categories", body, "user-owner")
	w := httptest.NewRecorder()
	handleBulkAssignCategories(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp BulkAssignCategoriesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.UpdatedIDs) != 1 || resp.UpdatedIDs[0] != "recipe-owner" {
		t.Errorf("UpdatedIDs = %v, want [recipe-owner]", resp.UpdatedIDs)
	}
	if len(resp.Failed) != 1 || resp.Failed[0].RecipeID != "recipe-other" {
		t.Errorf("Failed = %v, want [{recipe-other not found}]", resp.Failed)
	}
}

func TestHandleGetImageURL_PrefixCheck(t *testing.T) {
	// handleGetImageURL uses s3Presigner, but the prefix check happens before
	// the presign call, so we only test the forbidden cases.
	tests := []struct {
		name       string
		userID     string
		s3Key      string
		wantStatus int
	}{
		{"other user prefix denied", "user-123", "uploads/user-456/job/0.jpg", http.StatusForbidden},
		{"arbitrary key denied", "user-123", "secret/data.txt", http.StatusForbidden},
		{"empty key", "user-123", "", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := fmt.Sprintf("/image-url?key=%s", tt.s3Key)
			r := reqAsUser("GET", url, nil, tt.userID)
			w := httptest.NewRecorder()
			handleGetImageURL(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleGetRecipes_UserIsolation(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["r1"] = &Recipe{
		RecipeID:    "r1",
		UserID:      "user-a",
		Title:       "A's recipe",
		Ingredients: []string{"a"},
		Steps:       []string{"a"},
		CreatedAt:   time.Now(),
	}
	mock.recipes["r2"] = &Recipe{
		RecipeID:    "r2",
		UserID:      "user-b",
		Title:       "B's recipe",
		Ingredients: []string{"b"},
		Steps:       []string{"b"},
		CreatedAt:   time.Now(),
	}

	r := reqAsUser("GET", "/recipes", nil, "user-a")
	w := httptest.NewRecorder()
	handleGetRecipes(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var recipes []*Recipe
	if err := json.NewDecoder(w.Body).Decode(&recipes); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(recipes) != 1 {
		t.Fatalf("got %d recipes, want 1", len(recipes))
	}
	if recipes[0].RecipeID != "r1" {
		t.Errorf("got recipeID = %s, want r1", recipes[0].RecipeID)
	}
}

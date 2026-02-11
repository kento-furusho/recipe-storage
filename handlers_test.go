package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- Tests for utility functions ---

func TestNormalizeCategoryIDs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  int
	}{
		{"empty", []string{}, 0},
		{"nil", nil, 0},
		{"single", []string{"a"}, 1},
		{"duplicates removed", []string{"a", "b", "a"}, 2},
		{"whitespace trimmed", []string{" a ", "b"}, 2},
		{"blanks removed", []string{"", " ", "a"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCategoryIDs(tt.input)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d (got %v)", len(got), tt.want, got)
			}
		})
	}
}

func TestValidateCategoryInput(t *testing.T) {
	tests := []struct {
		name    string
		inName  string
		inColor string
		wantErr bool
	}{
		{"valid", "Dinner", "#FF0000", false},
		{"valid lowercase color", "Lunch", "#aabbcc", false},
		{"empty name", "", "#FF0000", true},
		{"whitespace name", "   ", "#FF0000", true},
		{"name too long", "あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほま", "#FF0000", true},
		{"invalid color no hash", "Test", "FF0000", true},
		{"invalid color short", "Test", "#FFF", true},
		{"invalid color chars", "Test", "#GGGGGG", true},
		{"empty color", "Test", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := validateCategoryInput(tt.inName, tt.inColor)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoveCategoryID(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		target string
		want   int
	}{
		{"remove existing", []string{"a", "b", "c"}, "b", 2},
		{"target not found", []string{"a", "b"}, "z", 2},
		{"empty slice", []string{}, "a", 0},
		{"remove all occurrences", []string{"a", "a", "b"}, "a", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeCategoryID(tt.values, tt.target)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestMergeCategoryIDs(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		add      []string
		want     int
	}{
		{"merge disjoint", []string{"a"}, []string{"b"}, 2},
		{"merge with overlap", []string{"a", "b"}, []string{"b", "c"}, 3},
		{"empty existing", nil, []string{"a"}, 1},
		{"empty add", []string{"a"}, nil, 1},
		{"both empty", nil, nil, 0},
		{"blanks removed", []string{"a", ""}, []string{" ", "b"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeCategoryIDs(tt.existing, tt.add)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d (got %v)", len(got), tt.want, got)
			}
		})
	}
}

// --- Tests for handler validation ---

func TestHandleUpdateRecipe_Validation(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["r1"] = &Recipe{
		RecipeID:    "r1",
		UserID:      "user-1",
		Title:       "Original",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
	}

	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
		wantBody   string
	}{
		{
			name:       "missing title",
			body:       UpdateRecipeRequest{Title: "", Ingredients: []string{"a"}, Steps: []string{"b"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "title required",
		},
		{
			name:       "missing ingredients",
			body:       UpdateRecipeRequest{Title: "T", Ingredients: []string{}, Steps: []string{"b"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "ingredients required",
		},
		{
			name:       "missing steps",
			body:       UpdateRecipeRequest{Title: "T", Ingredients: []string{"a"}, Steps: []string{}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "steps required",
		},
		{
			name:       "too many images",
			body:       UpdateRecipeRequest{Title: "T", Ingredients: []string{"a"}, Steps: []string{"b"}, ImageKeys: []string{"1", "2", "3", "4"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "maximum 3 images",
		},
		{
			name:       "invalid categoryIds",
			body:       UpdateRecipeRequest{Title: "T", Ingredients: []string{"a"}, Steps: []string{"b"}, CategoryIDs: []string{"nonexistent"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid categoryIds",
		},
		{
			name:       "valid update",
			body:       UpdateRecipeRequest{Title: "New", Ingredients: []string{"x"}, Steps: []string{"y"}},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			r := reqAsUser("PUT", "/recipes/r1", body, "user-1")
			r = withChiURLParam(r, "recipeId", "r1")
			w := httptest.NewRecorder()
			handleUpdateRecipe(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantBody != "" && !containsString(w.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandleUpdateRecipe_InvalidJSON(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["r1"] = &Recipe{
		RecipeID: "r1",
		UserID:   "user-1",
	}

	r := reqAsUser("PUT", "/recipes/r1", []byte("not json"), "user-1")
	r = withChiURLParam(r, "recipeId", "r1")
	w := httptest.NewRecorder()
	handleUpdateRecipe(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateRecipe_WithValidCategories(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.recipes["r1"] = &Recipe{
		RecipeID:    "r1",
		UserID:      "user-1",
		Title:       "Original",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
	}
	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1",
		UserID:     "user-1",
		Name:       "Dinner",
		Color:      "#FF0000",
	}

	body, _ := json.Marshal(UpdateRecipeRequest{
		Title:       "Updated",
		Ingredients: []string{"x"},
		Steps:       []string{"y"},
		CategoryIDs: []string{"cat-1"},
	})

	r := reqAsUser("PUT", "/recipes/r1", body, "user-1")
	r = withChiURLParam(r, "recipeId", "r1")
	w := httptest.NewRecorder()
	handleUpdateRecipe(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var recipe Recipe
	json.NewDecoder(w.Body).Decode(&recipe)
	if len(recipe.CategoryIDs) != 1 || recipe.CategoryIDs[0] != "cat-1" {
		t.Errorf("categoryIDs = %v, want [cat-1]", recipe.CategoryIDs)
	}
}

func TestHandleGetCategories(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	t.Run("empty list", func(t *testing.T) {
		r := reqAsUser("GET", "/categories", nil, "user-1")
		w := httptest.NewRecorder()
		handleGetCategories(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var cats []*Category
		json.NewDecoder(w.Body).Decode(&cats)
		if len(cats) != 0 {
			t.Errorf("got %d categories, want 0", len(cats))
		}
	})

	t.Run("sorted by name", func(t *testing.T) {
		mock.categories["c1"] = &Category{CategoryID: "c1", UserID: "user-1", Name: "Dinner", Color: "#FF0000"}
		mock.categories["c2"] = &Category{CategoryID: "c2", UserID: "user-1", Name: "Breakfast", Color: "#00FF00"}
		mock.categories["c3"] = &Category{CategoryID: "c3", UserID: "user-other", Name: "Other", Color: "#0000FF"}

		r := reqAsUser("GET", "/categories", nil, "user-1")
		w := httptest.NewRecorder()
		handleGetCategories(w, r)

		var cats []*Category
		json.NewDecoder(w.Body).Decode(&cats)
		if len(cats) != 2 {
			t.Fatalf("got %d, want 2", len(cats))
		}
		if cats[0].Name != "Breakfast" || cats[1].Name != "Dinner" {
			t.Errorf("order = [%s, %s], want [Breakfast, Dinner]", cats[0].Name, cats[1].Name)
		}
	})
}

func TestHandleCreateCategory(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
	}{
		{
			name:       "valid",
			body:       CreateCategoryRequest{Name: "Dinner", Color: "#FF0000"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid - empty name",
			body:       CreateCategoryRequest{Name: "", Color: "#FF0000"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid - bad color",
			body:       CreateCategoryRequest{Name: "Test", Color: "red"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupTest()
			defer cleanup()
			_ = mock

			body, _ := json.Marshal(tt.body)
			r := reqAsUser("POST", "/categories", body, "user-1")
			w := httptest.NewRecorder()
			handleCreateCategory(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleCreateCategory_DuplicateName(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1",
		UserID:     "user-1",
		Name:       "Dinner",
		Color:      "#FF0000",
	}

	body, _ := json.Marshal(CreateCategoryRequest{Name: "Dinner", Color: "#00FF00"})
	r := reqAsUser("POST", "/categories", body, "user-1")
	w := httptest.NewRecorder()
	handleCreateCategory(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !containsString(w.Body.String(), "already exists") {
		t.Errorf("body = %q, want to contain 'already exists'", w.Body.String())
	}
}

func TestHandleCreateCategory_InvalidJSON(t *testing.T) {
	_, cleanup := setupTest()
	defer cleanup()

	r := reqAsUser("POST", "/categories", []byte("not json"), "user-1")
	w := httptest.NewRecorder()
	handleCreateCategory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateCategory_Validation(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1",
		UserID:     "user-1",
		Name:       "Dinner",
		Color:      "#FF0000",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
	}{
		{"valid", UpdateCategoryRequest{Name: "Lunch", Color: "#00FF00"}, http.StatusOK},
		{"invalid color", UpdateCategoryRequest{Name: "Test", Color: "bad"}, http.StatusBadRequest},
		{"empty name", UpdateCategoryRequest{Name: "", Color: "#00FF00"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			r := reqAsUser("PUT", "/categories/cat-1", body, "user-1")
			r = withChiURLParam(r, "categoryId", "cat-1")
			w := httptest.NewRecorder()
			handleUpdateCategory(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestHandleUpdateCategory_DuplicateName(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1", UserID: "user-1", Name: "Dinner", Color: "#FF0000",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	mock.categories["cat-2"] = &Category{
		CategoryID: "cat-2", UserID: "user-1", Name: "Lunch", Color: "#00FF00",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	body, _ := json.Marshal(UpdateCategoryRequest{Name: "Lunch", Color: "#FF0000"})
	r := reqAsUser("PUT", "/categories/cat-1", body, "user-1")
	r = withChiURLParam(r, "categoryId", "cat-1")
	w := httptest.NewRecorder()
	handleUpdateCategory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateCategory_InvalidJSON(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1", UserID: "user-1", Name: "X", Color: "#FF0000",
	}

	r := reqAsUser("PUT", "/categories/cat-1", []byte("bad"), "user-1")
	r = withChiURLParam(r, "categoryId", "cat-1")
	w := httptest.NewRecorder()
	handleUpdateCategory(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteCategory_CascadeCleanup(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1", UserID: "user-1", Name: "Dinner", Color: "#FF0000",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	mock.recipes["r1"] = &Recipe{
		RecipeID:    "r1",
		UserID:      "user-1",
		Title:       "Test",
		Ingredients: []string{"a"},
		Steps:       []string{"b"},
		ImageKeys:   []string{},
		CategoryIDs: []string{"cat-1", "cat-2"},
	}

	r := reqAsUser("DELETE", "/categories/cat-1", nil, "user-1")
	r = withChiURLParam(r, "categoryId", "cat-1")
	w := httptest.NewRecorder()
	handleDeleteCategory(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify cascade: cat-1 removed from recipe
	recipe := mock.recipes["r1"]
	if len(recipe.CategoryIDs) != 1 || recipe.CategoryIDs[0] != "cat-2" {
		t.Errorf("recipe categoryIDs = %v, want [cat-2]", recipe.CategoryIDs)
	}
}

func TestHandleBulkAssignCategories_Validation(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	mock.categories["cat-1"] = &Category{
		CategoryID: "cat-1", UserID: "user-1", Name: "Dinner", Color: "#FF0000",
	}

	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
		wantBody   string
	}{
		{
			name:       "empty recipeIds",
			body:       BulkAssignCategoriesRequest{RecipeIDs: []string{}, CategoryIDs: []string{"cat-1"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "recipeIds required",
		},
		{
			name:       "empty categoryIds",
			body:       BulkAssignCategoriesRequest{RecipeIDs: []string{"r1"}, CategoryIDs: []string{}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "categoryIds required",
		},
		{
			name:       "invalid categoryIds",
			body:       BulkAssignCategoriesRequest{RecipeIDs: []string{"r1"}, CategoryIDs: []string{"nonexistent"}},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid categoryIds",
		},
		{
			name:       "invalid JSON",
			body:       nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.body == nil {
				body = []byte("bad json")
			} else {
				body, _ = json.Marshal(tt.body)
			}
			r := reqAsUser("POST", "/recipes/bulk-categories", body, "user-1")
			w := httptest.NewRecorder()
			handleBulkAssignCategories(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantBody != "" && !containsString(w.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandleGetRecipes_SortOrder(t *testing.T) {
	mock, cleanup := setupTest()
	defer cleanup()

	now := time.Now()
	mock.recipes["r1"] = &Recipe{
		RecipeID: "r1", UserID: "user-1", Title: "Old",
		Ingredients: []string{"a"}, Steps: []string{"a"},
		CreatedAt: now.Add(-1 * time.Hour),
	}
	mock.recipes["r2"] = &Recipe{
		RecipeID: "r2", UserID: "user-1", Title: "New",
		Ingredients: []string{"b"}, Steps: []string{"b"},
		CreatedAt: now,
	}

	r := reqAsUser("GET", "/recipes", nil, "user-1")
	w := httptest.NewRecorder()
	handleGetRecipes(w, r)

	var recipes []*Recipe
	json.NewDecoder(w.Body).Decode(&recipes)
	if len(recipes) != 2 {
		t.Fatalf("got %d, want 2", len(recipes))
	}
	if recipes[0].RecipeID != "r2" {
		t.Errorf("first recipe = %s, want r2 (newer)", recipes[0].RecipeID)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}

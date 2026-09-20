package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type UpdateRecipeRequest struct {
	Title       string   `json:"title"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	SourceURL   *string  `json:"sourceUrl,omitempty"`
	ImageKeys   []string `json:"imageKeys,omitempty"`
	CategoryIDs []string `json:"categoryIds,omitempty"`
}

type CreateCategoryRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type UpdateCategoryRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type BulkAssignCategoriesRequest struct {
	RecipeIDs   []string `json:"recipeIds"`
	CategoryIDs []string `json:"categoryIds"`
}

type bulkAssignFailed struct {
	RecipeID string `json:"recipeId"`
	Reason   string `json:"reason"`
}

type BulkAssignCategoriesResponse struct {
	UpdatedIDs []string           `json:"updatedIds"`
	Failed     []bulkAssignFailed `json:"failed"`
}

const (
	maxCategoryNameLen = 30
)

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// handleCreateJob はジョブ作成ハンドラー
func handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	// 画像またはURLのどちらかが必要
	if len(req.ImageKeys) == 0 && (req.SourceURL == nil || *req.SourceURL == "") {
		http.Error(w, "imageKeys or sourceUrl required", http.StatusBadRequest)
		return
	}

	jobID := uuid.New().String()
	userID := getUserID(r)
	now := time.Now()
	ctx := r.Context()

	// 再作成（上書き）の場合は対象レシピの存在と所有者を検証する
	if req.TargetRecipeID != nil && *req.TargetRecipeID != "" {
		existing, err := dbStore.GetRecipe(ctx, *req.TargetRecipeID)
		if err != nil {
			log.Printf("Failed to get target recipe: %v", err)
			http.Error(w, "failed to load target recipe", http.StatusInternalServerError)
			return
		}
		if existing == nil || existing.UserID != userID {
			http.Error(w, "target recipe not found", http.StatusNotFound)
			return
		}
	}

	job := &Job{
		JobID:          jobID,
		UserID:         userID,
		Status:         JobStatusPending,
		ImageKeys:      req.ImageKeys,
		SourceURL:      req.SourceURL,
		RecipeID:       nil,
		TargetRecipeID: req.TargetRecipeID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// DynamoDBに保存
	if err := dbStore.SaveJob(ctx, job); err != nil {
		log.Printf("Failed to save job to DynamoDB: %v", err)
		http.Error(w, "failed to save job", http.StatusInternalServerError)
		return
	}

	// ジョブをキューに投入（ローカルは直接処理）
	if err := dispatchJob(ctx, jobID); err != nil {
		log.Printf("Failed to dispatch job: %v", err)
		http.Error(w, "failed to dispatch job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(CreateJobResponse{
		JobID: jobID,
	})
}

// handleGetJob はジョブ取得ハンドラー
func handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	if jobID == "" {
		http.Error(w, "jobId required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	job, err := dbStore.GetJob(ctx, jobID)
	if err != nil {
		log.Printf("Failed to get job from DynamoDB: %v", err)
		http.Error(w, "failed to get job", http.StatusInternalServerError)
		return
	}
	if job == nil || job.UserID != getUserID(r) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

const (
	defaultRecipePageSize = 10
	maxRecipePageSize     = 50
	// キーワード検索で一度に返す最大件数
	maxKeywordSearchResults = 100
)

// RecipePageResponse はページ単位のレシピ一覧レスポンス
type RecipePageResponse struct {
	Items      []*Recipe `json:"items"`
	NextCursor *string   `json:"nextCursor"`
}

// handleGetRecipes はレシピ一覧取得ハンドラー。
// limit か cursor を指定するとページ単位（RecipePageResponse）で返す。
// 指定なしは従来どおり全件の配列を返す（旧フロントとの互換のため）。
func handleGetRecipes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(r)
	query := r.URL.Query()
	limitParam := query.Get("limit")
	cursor := query.Get("cursor")

	if limitParam == "" && cursor == "" && strings.TrimSpace(query.Get("q")) == "" {
		recipes, err := dbStore.ListRecipes(ctx, userID)
		if err != nil {
			log.Printf("Failed to list recipes from DynamoDB: %v", err)
			http.Error(w, "failed to list recipes", http.StatusInternalServerError)
			return
		}

		// 作成日時の降順でソート（新しいものから）
		sort.SliceStable(recipes, func(i, j int) bool {
			return recipes[i].CreatedAt.After(recipes[j].CreatedAt)
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(recipes)
		return
	}

	limit := defaultRecipePageSize
	if limitParam != "" {
		n, err := strconv.Atoi(limitParam)
		if err != nil || n < 1 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = min(n, maxRecipePageSize)
	}

	// キーワード検索はレシピ名と材料の文字列を対象にする。件数が少ないので
	// ページ送りはせず、最大 maxKeywordSearchResults 件まで一度に返す
	if keyword := strings.TrimSpace(query.Get("q")); keyword != "" {
		recipes, err := dbStore.ListRecipes(ctx, userID)
		if err != nil {
			log.Printf("Failed to list recipes for keyword search: %v", err)
			http.Error(w, "failed to list recipes", http.StatusInternalServerError)
			return
		}
		matched := filterRecipesByKeyword(recipes, keyword, query.Get("categoryId"))
		if len(matched) > maxKeywordSearchResults {
			matched = matched[:maxKeywordSearchResults]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RecipePageResponse{Items: matched})
		return
	}

	recipes, nextCursor, err := dbStore.ListRecipesPage(ctx, userID, query.Get("categoryId"), limit, cursor)
	if errors.Is(err, errInvalidCursor) {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("Failed to list recipe page from DynamoDB: %v", err)
		http.Error(w, "failed to list recipes", http.StatusInternalServerError)
		return
	}

	resp := RecipePageResponse{Items: recipes}
	if resp.Items == nil {
		resp.Items = []*Recipe{}
	}
	if nextCursor != "" {
		resp.NextCursor = &nextCursor
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleGetRecipe はレシピ詳細取得ハンドラー
func handleGetRecipe(w http.ResponseWriter, r *http.Request) {
	recipeID := chi.URLParam(r, "recipeId")
	if recipeID == "" {
		http.Error(w, "recipeId required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	recipe, err := dbStore.GetRecipe(ctx, recipeID)
	if err != nil {
		log.Printf("Failed to get recipe from DynamoDB: %v", err)
		http.Error(w, "failed to get recipe", http.StatusInternalServerError)
		return
	}
	if recipe == nil || recipe.UserID != getUserID(r) {
		http.Error(w, "recipe not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recipe)
}

// handleDeleteRecipe はレシピを削除する
func handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
	recipeID := chi.URLParam(r, "recipeId")
	if recipeID == "" {
		http.Error(w, "recipeId required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	recipe, err := dbStore.GetRecipe(ctx, recipeID)
	if err != nil {
		log.Printf("Failed to get recipe from DynamoDB: %v", err)
		http.Error(w, "failed to get recipe", http.StatusInternalServerError)
		return
	}
	if recipe == nil || recipe.UserID != getUserID(r) {
		http.Error(w, "recipe not found", http.StatusNotFound)
		return
	}

	if err := dbStore.DeleteRecipe(ctx, recipeID); err != nil {
		log.Printf("Failed to delete recipe from DynamoDB: %v", err)
		http.Error(w, "failed to delete recipe", http.StatusInternalServerError)
		return
	}

	// Image deletion is best-effort; the recipe should be removed regardless.
	for _, key := range recipe.ImageKeys {
		if key == "" {
			continue
		}
		if _, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: &bucketName,
			Key:    &key,
		}); err != nil {
			log.Printf("Failed to delete image from S3 (key=%s): %v", key, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateRecipe はレシピを更新する
func handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
	recipeID := chi.URLParam(r, "recipeId")
	if recipeID == "" {
		http.Error(w, "recipeId required", http.StatusBadRequest)
		return
	}

	var req UpdateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	if len(req.Ingredients) == 0 {
		http.Error(w, "ingredients required", http.StatusBadRequest)
		return
	}
	if len(req.Steps) == 0 {
		http.Error(w, "steps required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	recipe, err := dbStore.GetRecipe(ctx, recipeID)
	if err != nil {
		log.Printf("Failed to get recipe from DynamoDB: %v", err)
		http.Error(w, "failed to get recipe", http.StatusInternalServerError)
		return
	}
	if recipe == nil || recipe.UserID != getUserID(r) {
		http.Error(w, "recipe not found", http.StatusNotFound)
		return
	}

	ingredientsChanged := !slices.Equal(recipe.Ingredients, req.Ingredients)

	recipe.Title = req.Title
	recipe.Ingredients = req.Ingredients
	recipe.Steps = req.Steps
	recipe.SourceURL = req.SourceURL

	// 材料が変わった時だけ食材マスタへの対応づけをやり直す。
	// 失敗しても保存は続け、古い対応づけは残さない（nil＝未処理）
	if ingredientsChanged {
		resolveCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		names, err := resolveIngredients(resolveCtx, recipe.Ingredients)
		cancel()
		if err != nil {
			log.Printf("Failed to resolve ingredients for recipe %s: %v", recipe.RecipeID, err)
			recipe.IngredientNames = nil
		} else {
			recipe.IngredientNames = names
		}
	}

	// ImageKeysの更新（nilの場合は既存の値を保持）
	if req.ImageKeys != nil {
		if len(req.ImageKeys) > 3 {
			http.Error(w, "maximum 3 images allowed", http.StatusBadRequest)
			return
		}
		recipe.ImageKeys = req.ImageKeys
	}

	if req.CategoryIDs != nil {
		categoryIDs := normalizeCategoryIDs(req.CategoryIDs)
		if err := validateCategoryIDs(ctx, categoryIDs); err != nil {
			http.Error(w, "invalid categoryIds", http.StatusBadRequest)
			return
		}
		recipe.CategoryIDs = categoryIDs
	}

	if err := dbStore.SaveRecipe(ctx, recipe); err != nil {
		log.Printf("Failed to save recipe to DynamoDB: %v", err)
		http.Error(w, "failed to update recipe", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recipe)
}

// handleGetImageURL は画像のpresigned URL取得ハンドラー
func handleGetImageURL(w http.ResponseWriter, r *http.Request) {
	s3Key := r.URL.Query().Get("key")
	if s3Key == "" {
		http.Error(w, "key parameter required", http.StatusBadRequest)
		return
	}

	// S3キーが自分のユーザーIDプレフィックスに属するか検証
	userID := getUserID(r)
	expectedPrefix := "uploads/" + userID + "/"
	if !strings.HasPrefix(s3Key, expectedPrefix) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	getReq, err := s3Presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &s3Key,
	}, s3.WithPresignExpires(1*time.Hour))
	if err != nil {
		http.Error(w, "failed to presign", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"url": getReq.URL,
	})
}

// handleGetUploadURL はアップロード用presigned URL取得ハンドラー
func handleGetUploadURL(w http.ResponseWriter, r *http.Request) {
	var req UploadURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.JobID == "" || req.Index < 0 || req.Index > 2 || req.ContentType == "" {
		http.Error(w, "jobId, index(0..2), contentType required", http.StatusBadRequest)
		return
	}

	userID := getUserID(r)
	key := "uploads/" + userID + "/" + req.JobID + "/" + itoa(req.Index) + ".jpg"

	ctx := r.Context()
	putReq, err := s3Presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      &bucketName,
		Key:         &key,
		ContentType: &req.ContentType,
	}, s3.WithPresignExpires(10*time.Minute))
	if err != nil {
		http.Error(w, "failed to presign", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(UploadURLResponse{
		UploadURL: putReq.URL,
		S3Key:     key,
	})
}

func normalizeCategoryIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateCategoryIDs(ctx context.Context, categoryIDs []string) error {
	for _, id := range categoryIDs {
		category, err := dbStore.GetCategory(ctx, id)
		if err != nil {
			return err
		}
		if category == nil {
			return fmt.Errorf("category not found: %s", id)
		}
	}
	return nil
}

func validateCategoryInput(name string, color string) (string, string, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return "", "", fmt.Errorf("name required")
	}
	if len([]rune(trimmedName)) > maxCategoryNameLen {
		return "", "", fmt.Errorf("name too long")
	}
	trimmedColor := strings.TrimSpace(color)
	if !colorPattern.MatchString(trimmedColor) {
		return "", "", fmt.Errorf("invalid color")
	}
	return trimmedName, trimmedColor, nil
}

// handleGetCategories はカテゴリ一覧取得ハンドラー
func handleGetCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(r)

	categories, err := dbStore.ListCategories(ctx, userID)
	if err != nil {
		log.Printf("Failed to list categories: %v", err)
		http.Error(w, "failed to list categories", http.StatusInternalServerError)
		return
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Name < categories[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(categories)
}

// handleCreateCategory はカテゴリ作成ハンドラー
func handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name, color, err := validateCategoryInput(req.Name, req.Color)
	if err != nil {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := getUserID(r)
	existing, err := dbStore.ListCategories(ctx, userID)
	if err != nil {
		http.Error(w, "failed to list categories", http.StatusInternalServerError)
		return
	}
	for _, category := range existing {
		if category.Name == name {
			http.Error(w, "category name already exists", http.StatusBadRequest)
			return
		}
	}

	now := time.Now()
	category := &Category{
		CategoryID: uuid.New().String(),
		UserID:     userID,
		Name:       name,
		Color:      color,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := dbStore.SaveCategory(ctx, category); err != nil {
		http.Error(w, "failed to save category", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(category)
}

// handleUpdateCategory はカテゴリ更新ハンドラー
func handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	categoryID := chi.URLParam(r, "categoryId")
	if categoryID == "" {
		http.Error(w, "categoryId required", http.StatusBadRequest)
		return
	}

	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name, color, err := validateCategoryInput(req.Name, req.Color)
	if err != nil {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	existingCategory, err := dbStore.GetCategory(ctx, categoryID)
	if err != nil {
		http.Error(w, "failed to get category", http.StatusInternalServerError)
		return
	}
	if existingCategory == nil || existingCategory.UserID != getUserID(r) {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}

	allCategories, err := dbStore.ListCategories(ctx, existingCategory.UserID)
	if err != nil {
		http.Error(w, "failed to list categories", http.StatusInternalServerError)
		return
	}
	for _, category := range allCategories {
		if category.Name == name && category.CategoryID != categoryID {
			http.Error(w, "category name already exists", http.StatusBadRequest)
			return
		}
	}

	existingCategory.Name = name
	existingCategory.Color = color
	existingCategory.UpdatedAt = time.Now()
	if err := dbStore.SaveCategory(ctx, existingCategory); err != nil {
		http.Error(w, "failed to update category", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(existingCategory)
}

// handleDeleteCategory はカテゴリ削除ハンドラー
func handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	categoryID := chi.URLParam(r, "categoryId")
	if categoryID == "" {
		http.Error(w, "categoryId required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	category, err := dbStore.GetCategory(ctx, categoryID)
	if err != nil {
		http.Error(w, "failed to get category", http.StatusInternalServerError)
		return
	}
	if category == nil || category.UserID != getUserID(r) {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}

	if err := dbStore.DeleteCategory(ctx, categoryID); err != nil {
		http.Error(w, "failed to delete category", http.StatusInternalServerError)
		return
	}

	recipes, err := dbStore.ListRecipes(ctx, category.UserID)
	if err == nil {
		for _, recipe := range recipes {
			if len(recipe.CategoryIDs) == 0 {
				continue
			}
			updated := removeCategoryID(recipe.CategoryIDs, categoryID)
			if len(updated) != len(recipe.CategoryIDs) {
				recipe.CategoryIDs = updated
				_ = dbStore.SaveRecipe(ctx, recipe)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func removeCategoryID(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == target {
			continue
		}
		result = append(result, value)
	}
	return result
}

func mergeCategoryIDs(existing []string, add []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(add))
	result := make([]string, 0, len(existing)+len(add))
	for _, value := range existing {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, value := range add {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func handleBulkAssignCategories(w http.ResponseWriter, r *http.Request) {
	var req BulkAssignCategoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	recipeIDs := normalizeCategoryIDs(req.RecipeIDs)
	if len(recipeIDs) == 0 {
		http.Error(w, "recipeIds required", http.StatusBadRequest)
		return
	}
	if len(recipeIDs) > 50 {
		http.Error(w, "recipeIds limit exceeded", http.StatusBadRequest)
		return
	}

	categoryIDs := normalizeCategoryIDs(req.CategoryIDs)
	if len(categoryIDs) == 0 {
		http.Error(w, "categoryIds required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := validateCategoryIDs(ctx, categoryIDs); err != nil {
		http.Error(w, "invalid categoryIds", http.StatusBadRequest)
		return
	}

	response := BulkAssignCategoriesResponse{
		UpdatedIDs: make([]string, 0, len(recipeIDs)),
		Failed:     make([]bulkAssignFailed, 0),
	}

	userID := getUserID(r)
	for _, recipeID := range recipeIDs {
		recipe, err := dbStore.GetRecipe(ctx, recipeID)
		if err != nil {
			response.Failed = append(response.Failed, bulkAssignFailed{
				RecipeID: recipeID,
				Reason:   "failed to load",
			})
			continue
		}
		if recipe == nil || recipe.UserID != userID {
			response.Failed = append(response.Failed, bulkAssignFailed{
				RecipeID: recipeID,
				Reason:   "not found",
			})
			continue
		}

		recipe.CategoryIDs = mergeCategoryIDs(recipe.CategoryIDs, categoryIDs)
		if err := dbStore.SaveRecipe(ctx, recipe); err != nil {
			response.Failed = append(response.Failed, bulkAssignFailed{
				RecipeID: recipeID,
				Reason:   "save failed",
			})
			continue
		}
		response.UpdatedIDs = append(response.UpdatedIDs, recipeID)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

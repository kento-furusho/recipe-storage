package main

import (
	"encoding/json"
	"log"
	"net/http"
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
}

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
	userID := "local"
	now := time.Now()

	job := &Job{
		JobID:     jobID,
		UserID:    userID,
		Status:    JobStatusPending,
		ImageKeys: req.ImageKeys,
		SourceURL: req.SourceURL,
		RecipeID:  nil,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// DynamoDBに保存
	ctx := r.Context()
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
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

// handleGetRecipes はレシピ一覧取得ハンドラー
func handleGetRecipes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := "local"
	recipes, err := dbStore.ListRecipes(ctx, userID)
	if err != nil {
		log.Printf("Failed to list recipes from DynamoDB: %v", err)
		http.Error(w, "failed to list recipes", http.StatusInternalServerError)
		return
	}

	// 作成日時の降順でソート（新しいものから）
	for i := 0; i < len(recipes)-1; i++ {
		for j := i + 1; j < len(recipes); j++ {
			if recipes[i].CreatedAt.Before(recipes[j].CreatedAt) {
				recipes[i], recipes[j] = recipes[j], recipes[i]
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recipes)
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
	if recipe == nil {
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
	if recipe == nil {
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
	if recipe == nil {
		http.Error(w, "recipe not found", http.StatusNotFound)
		return
	}

	recipe.Title = req.Title
	recipe.Ingredients = req.Ingredients
	recipe.Steps = req.Steps
	recipe.SourceURL = req.SourceURL

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

	userID := "local"
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

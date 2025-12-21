package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type UploadURLRequest struct {
	JobID       string `json:"jobId"`
	Index       int    `json:"index"`       // 0..2
	ContentType string `json:"contentType"` // image/jpeg, image/png
}

type UploadURLResponse struct {
	UploadURL string `json:"uploadUrl"`
	S3Key     string `json:"s3Key"`
}

type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusSucceeded  JobStatus = "SUCCEEDED"
	JobStatusFailed     JobStatus = "FAILED"
)

type Job struct {
	JobID     string     `json:"jobId"`
	UserID    string     `json:"userId"`
	Status    JobStatus  `json:"status"`
	ImageKeys []string   `json:"imageKeys"`
	SourceURL *string    `json:"sourceUrl"`
	RecipeID  *string    `json:"recipeId"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type CreateJobRequest struct {
	ImageKeys []string `json:"imageKeys"`
	SourceURL *string  `json:"sourceUrl,omitempty"`
}

type CreateJobResponse struct {
	JobID string `json:"jobId"`
}

type JobStore struct {
	mu    sync.RWMutex
	jobs  map[string]*Job
	recipes map[string]*Recipe
}

type Recipe struct {
	RecipeID    string    `json:"recipeId"`
	UserID      string    `json:"userId"`
	Title       string    `json:"title"`
	Ingredients []string  `json:"ingredients"`
	Steps       []string  `json:"steps"`
	ImageKeys   []string  `json:"imageKeys"`
	SourceURL   *string   `json:"sourceUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

var jobStore = &JobStore{
	jobs:    make(map[string]*Job),
	recipes: make(map[string]*Recipe),
}

func main() {
	// .envファイルを読み込む（エラーは無視 - 本番環境では環境変数を直接使用）
	_ = godotenv.Load()

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET is required (環境変数または.envファイルで設定してください)")
	}

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("LoadDefaultConfig: %v", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)
	presigner := s3.NewPresignClient(s3Client)

	r := chi.NewRouter()

	// CORS設定
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Post("/jobs", func(w http.ResponseWriter, r *http.Request) {
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

		jobStore.mu.Lock()
		jobStore.jobs[jobID] = job
		jobStore.mu.Unlock()

		// 非同期でレシピ生成を開始（ダミー実装）
		go processJob(jobID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreateJobResponse{
			JobID: jobID,
		})
	})

	r.Get("/jobs/{jobId}", func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		if jobID == "" {
			http.Error(w, "jobId required", http.StatusBadRequest)
			return
		}

		jobStore.mu.RLock()
		job, exists := jobStore.jobs[jobID]
		jobStore.mu.RUnlock()

		if !exists {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(job)
	})

	r.Get("/recipes", func(w http.ResponseWriter, r *http.Request) {
		jobStore.mu.RLock()
		recipes := make([]*Recipe, 0, len(jobStore.recipes))
		for _, recipe := range jobStore.recipes {
			recipes = append(recipes, recipe)
		}
		jobStore.mu.RUnlock()

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
	})

	r.Get("/recipes/{recipeId}", func(w http.ResponseWriter, r *http.Request) {
		recipeID := chi.URLParam(r, "recipeId")
		if recipeID == "" {
			http.Error(w, "recipeId required", http.StatusBadRequest)
			return
		}

		jobStore.mu.RLock()
		recipe, exists := jobStore.recipes[recipeID]
		jobStore.mu.RUnlock()

		if !exists {
			http.Error(w, "recipe not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(recipe)
	})

	r.Get("/image-url", func(w http.ResponseWriter, r *http.Request) {
		s3Key := r.URL.Query().Get("key")
		if s3Key == "" {
			http.Error(w, "key parameter required", http.StatusBadRequest)
			return
		}

		getReq, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
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
	})

	r.Post("/upload-url", func(w http.ResponseWriter, r *http.Request) {
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

		putReq, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
			Bucket:      &bucket,
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
	})

	log.Println("API listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i == 1 {
		return "1"
	}
	return "2"
}

// extractTitleFromURL はURLからタイトルを推測する
func extractTitleFromURL(sourceURL string) string {
	if sourceURL == "" {
		return "レシピ"
	}

	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return "レシピ"
	}

	host := parsedURL.Host
	path := parsedURL.Path

	// YouTube URLの場合
	if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
		return "YouTubeレシピ"
	}

	// パスからタイトルを推測
	if path != "" && path != "/" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			// URLエンコードされた文字をデコード
			if decoded, err := url.QueryUnescape(lastPart); err == nil {
				lastPart = decoded
			}
			// ハイフンやアンダースコアをスペースに変換
			lastPart = strings.ReplaceAll(lastPart, "-", " ")
			lastPart = strings.ReplaceAll(lastPart, "_", " ")
			// 最初の文字を大文字に
			if len(lastPart) > 0 {
				return strings.ToUpper(string(lastPart[0])) + lastPart[1:]
			}
		}
	}

	// ホスト名から推測
	if host != "" {
		parts := strings.Split(host, ".")
		if len(parts) > 0 {
			return parts[0] + "のレシピ"
		}
	}

	return "レシピ"
}

// generateDummyRecipe はダミーレシピデータを生成する
func generateDummyRecipe(job *Job) *Recipe {
	recipeID := uuid.New().String()
	now := time.Now()

	// ImageKeysがnilの場合は空配列にする
	imageKeys := job.ImageKeys
	if imageKeys == nil {
		imageKeys = []string{}
	}

	// URLからタイトルを推測
	title := "サンプルレシピ"
	if job.SourceURL != nil && *job.SourceURL != "" {
		title = extractTitleFromURL(*job.SourceURL)
	}

	// ダミーの材料と手順（タイトルに応じて変更可能）
	ingredients := []string{"材料1", "材料2", "材料3"}
	steps := []string{"手順1: 下準備をする", "手順2: 調理する", "手順3: 仕上げる"}

	return &Recipe{
		RecipeID:    recipeID,
		UserID:      job.UserID,
		Title:       title,
		Ingredients: ingredients,
		Steps:       steps,
		ImageKeys:   imageKeys,
		SourceURL:   job.SourceURL,
		CreatedAt:   now,
	}
}

// processJob は非同期でレシピ生成を行う（ダミー実装）
func processJob(jobID string) {
	jobStore.mu.Lock()
	job, exists := jobStore.jobs[jobID]
	if !exists {
		jobStore.mu.Unlock()
		return
	}
	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	jobStore.mu.Unlock()

	// ダミー処理: 3秒待機してからレシピを生成
	time.Sleep(3 * time.Second)

	recipe := generateDummyRecipe(job)

	jobStore.mu.Lock()
	job.Status = JobStatusSucceeded
	job.RecipeID = &recipe.RecipeID
	job.UpdatedAt = time.Now()
	jobStore.jobs[jobID] = job
	jobStore.recipes[recipe.RecipeID] = recipe
	jobStore.mu.Unlock()

	log.Printf("Job %s completed, recipe %s created: %s", jobID, recipe.RecipeID, recipe.Title)
}

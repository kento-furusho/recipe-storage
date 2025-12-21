package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
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

var (
	s3Client      *s3.Client
	bedrockClient *bedrockruntime.Client
	bucketName    string
)

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

	s3Client = s3.NewFromConfig(awsCfg)
	presigner := s3.NewPresignClient(s3Client)
	bedrockClient = bedrockruntime.NewFromConfig(awsCfg)
	bucketName = bucket

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

// ImageData は画像データとメディアタイプを保持する
type ImageData struct {
	Base64    string
	MediaType string
}

// downloadImageFromS3 はS3から画像をダウンロードしてbase64エンコードする
func downloadImageFromS3(ctx context.Context, s3Key string) (*ImageData, error) {
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &s3Key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	imageData, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	// メディアタイプを判定（ContentTypeから、またはファイル拡張子から）
	mediaType := "image/jpeg" // デフォルト
	if result.ContentType != nil {
		mediaType = *result.ContentType
	} else {
		// ファイル拡張子から判定
		lowerKey := strings.ToLower(s3Key)
		if strings.HasSuffix(lowerKey, ".png") {
			mediaType = "image/png"
		} else if strings.HasSuffix(lowerKey, ".jpg") || strings.HasSuffix(lowerKey, ".jpeg") {
			mediaType = "image/jpeg"
		}
	}

	// base64エンコード
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	return &ImageData{
		Base64:    base64Image,
		MediaType: mediaType,
	}, nil
}

// buildPrompt はLLM用のプロンプトを構築する
func buildPrompt(imageCount int, sourceURL *string) string {
	var prompt strings.Builder

	prompt.WriteString("あなたは料理レシピを画像から抽出する専門家です。\n")
	prompt.WriteString("以下の画像からレシピ情報を抽出し、JSON形式で返してください。\n\n")

	prompt.WriteString("【画像】\n")
	if imageCount > 0 {
		if imageCount > 1 {
			roles := []string{"完成写真", "材料", "手順"}
			for i := 0; i < imageCount && i < len(roles); i++ {
				prompt.WriteString(fmt.Sprintf("%d枚目: %s\n", i+1, roles[i]))
			}
		}
	}

	if sourceURL != nil && *sourceURL != "" {
		prompt.WriteString("\n【補助情報】\n")
		prompt.WriteString(fmt.Sprintf("- ソースURL: %s\n", *sourceURL))
	}

	prompt.WriteString("\n【出力形式】\n")
	prompt.WriteString("以下のJSON形式で返してください：\n")
	prompt.WriteString("{\n")
	prompt.WriteString(`  "title": "料理名（日本語）",` + "\n")
	prompt.WriteString(`  "ingredients": [` + "\n")
	prompt.WriteString(`    "材料1（分量含む）",` + "\n")
	prompt.WriteString(`    "材料2（分量含む）",` + "\n")
	prompt.WriteString(`    ...` + "\n")
	prompt.WriteString(`  ],` + "\n")
	prompt.WriteString(`  "steps": [` + "\n")
	prompt.WriteString(`    "手順1（具体的な調理方法）",` + "\n")
	prompt.WriteString(`    "手順2（具体的な調理方法）",` + "\n")
	prompt.WriteString(`    ...` + "\n")
	prompt.WriteString(`  ]` + "\n")
	prompt.WriteString("}\n\n")

	prompt.WriteString("【注意事項】\n")
	prompt.WriteString("- 画像にレシピ情報が含まれていない場合は、可能な限り推測してください\n")
	prompt.WriteString("- 材料は分量を含めて記載してください（例: \"鶏もも肉 200g\"）\n")
	prompt.WriteString("- 手順は具体的で実行可能な内容にしてください\n")
	prompt.WriteString("- 不明な点は推測せず、空欄にするか「要確認」と記載してください\n")
	prompt.WriteString("- 必ず有効なJSON形式で返してください\n")

	return prompt.String()
}

// callBedrockAPI はBedrock APIを呼び出してレシピを生成する
func callBedrockAPI(ctx context.Context, prompt string, imageDataList []*ImageData) (string, error) {
	// Claude 3 HaikuのモデルID
	modelID := "anthropic.claude-3-haiku-20240307-v1:0"

	// メッセージコンテンツを構築
	content := []map[string]interface{}{
		{
			"type": "text",
			"text": prompt,
		},
	}

	// 画像を追加
	for _, imgData := range imageDataList {
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": imgData.MediaType,
				"data":       imgData.Base64,
			},
		})
	}

	// リクエストボディを構築
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Bedrock APIを呼び出し
	result, err := bedrockClient.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Body:        jsonBody,
	})
	if err != nil {
		return "", fmt.Errorf("failed to invoke model: %w", err)
	}

	// レスポンスをパース
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(result.Body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	return response.Content[0].Text, nil
}

// parseRecipeResponse はLLMのレスポンスをパースしてRecipeオブジェクトを生成する
func parseRecipeResponse(responseText string, job *Job) (*Recipe, error) {
	// JSONを抽出（コードブロックやマークダウンを除去）
	responseText = strings.TrimSpace(responseText)
	if strings.HasPrefix(responseText, "```json") {
		responseText = strings.TrimPrefix(responseText, "```json")
		responseText = strings.TrimSuffix(responseText, "```")
	} else if strings.HasPrefix(responseText, "```") {
		responseText = strings.TrimPrefix(responseText, "```")
		responseText = strings.TrimSuffix(responseText, "```")
	}
	responseText = strings.TrimSpace(responseText)

	var llmResponse struct {
		Title       string   `json:"title"`
		Ingredients []string `json:"ingredients"`
		Steps       []string `json:"steps"`
	}

	if err := json.Unmarshal([]byte(responseText), &llmResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// バリデーション
	if llmResponse.Title == "" {
		llmResponse.Title = "レシピ"
		if job.SourceURL != nil && *job.SourceURL != "" {
			llmResponse.Title = extractTitleFromURL(*job.SourceURL)
		}
	}
	if len(llmResponse.Title) > 100 {
		llmResponse.Title = llmResponse.Title[:100]
	}

	if len(llmResponse.Ingredients) == 0 {
		llmResponse.Ingredients = []string{"要確認"}
	}
	if len(llmResponse.Ingredients) > 50 {
		llmResponse.Ingredients = llmResponse.Ingredients[:50]
	}

	if len(llmResponse.Steps) == 0 {
		llmResponse.Steps = []string{"要確認"}
	}
	if len(llmResponse.Steps) > 30 {
		llmResponse.Steps = llmResponse.Steps[:30]
	}

	// ImageKeysがnilの場合は空配列にする
	imageKeys := job.ImageKeys
	if imageKeys == nil {
		imageKeys = []string{}
	}

	recipe := &Recipe{
		RecipeID:    uuid.New().String(),
		UserID:      job.UserID,
		Title:       llmResponse.Title,
		Ingredients: llmResponse.Ingredients,
		Steps:       llmResponse.Steps,
		ImageKeys:   imageKeys,
		SourceURL:   job.SourceURL,
		CreatedAt:   time.Now(),
	}

	return recipe, nil
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

// processJob は非同期でレシピ生成を行う
func processJob(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	jobStore.mu.Lock()
	job, exists := jobStore.jobs[jobID]
	if !exists {
		jobStore.mu.Unlock()
		return
	}
	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	jobStore.mu.Unlock()

	var recipe *Recipe
	var err error

	// 画像がある場合はLLMを使用、ない場合はダミー実装
	if len(job.ImageKeys) > 0 {
		recipe, err = generateRecipeWithLLM(ctx, job)
		if err != nil {
			log.Printf("Failed to generate recipe with LLM for job %s: %v, falling back to dummy", jobID, err)
			// LLM失敗時はダミー実装にフォールバック
			recipe = generateDummyRecipe(job)
		}
	} else {
		// 画像がない場合はダミー実装
		recipe = generateDummyRecipe(job)
	}

	jobStore.mu.Lock()
	if err == nil {
		job.Status = JobStatusSucceeded
	} else {
		job.Status = JobStatusFailed
	}
	job.RecipeID = &recipe.RecipeID
	job.UpdatedAt = time.Now()
	jobStore.jobs[jobID] = job
	jobStore.recipes[recipe.RecipeID] = recipe
	jobStore.mu.Unlock()

	if len(job.ImageKeys) > 0 {
		log.Printf("Job %s completed, recipe %s created: %s (LLM generated)", jobID, recipe.RecipeID, recipe.Title)
	} else {
		log.Printf("Job %s completed, recipe %s created: %s (dummy)", jobID, recipe.RecipeID, recipe.Title)
	}
}

// generateRecipeWithLLM はLLMを使用してレシピを生成する
func generateRecipeWithLLM(ctx context.Context, job *Job) (*Recipe, error) {
	// S3から画像をダウンロードしてbase64エンコード
	imageDataList := make([]*ImageData, 0, len(job.ImageKeys))
	for _, imageKey := range job.ImageKeys {
		imgData, err := downloadImageFromS3(ctx, imageKey)
		if err != nil {
			return nil, fmt.Errorf("failed to download image %s: %w", imageKey, err)
		}
		imageDataList = append(imageDataList, imgData)
	}

	// プロンプトを構築
	prompt := buildPrompt(len(imageDataList), job.SourceURL)

	// Bedrock APIを呼び出し（リトライ付き）
	log.Printf("Calling Bedrock API for job %s", job.JobID)
	var responseText string
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		responseText, err = callBedrockAPI(ctx, prompt, imageDataList)
		if err == nil {
			log.Printf("Bedrock API call succeeded for job %s", job.JobID)
			break
		}
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second // 指数バックオフ: 1秒、2秒、4秒
			log.Printf("Retrying Bedrock API call (attempt %d/%d) after %v", i+1, maxRetries, backoff)
			time.Sleep(backoff)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to call Bedrock API after %d retries: %w", maxRetries, err)
	}

	// レスポンスをパース
	log.Printf("Parsing Bedrock API response for job %s", job.JobID)
	recipe, err := parseRecipeResponse(responseText, job)
	if err != nil {
		// JSONパースエラーの場合、もう一度リトライ
		log.Printf("Failed to parse response for job %s, retrying once: %v", job.JobID, err)
		responseText, err = callBedrockAPI(ctx, prompt, imageDataList)
		if err != nil {
			return nil, fmt.Errorf("failed to retry Bedrock API: %w", err)
		}
		recipe, err = parseRecipeResponse(responseText, job)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response after retry: %w", err)
		}
		log.Printf("Successfully parsed recipe response for job %s: %s", job.JobID, recipe.Title)
	} else {
		log.Printf("Successfully parsed recipe response for job %s: %s", job.JobID, recipe.Title)
	}

	return recipe, nil
}

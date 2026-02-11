package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
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
	JobID     string    `json:"jobId" dynamodbav:"jobId"`
	UserID    string    `json:"userId" dynamodbav:"userId"`
	Status    JobStatus `json:"status" dynamodbav:"status"`
	ImageKeys []string  `json:"imageKeys" dynamodbav:"imageKeys"`
	SourceURL *string   `json:"sourceUrl" dynamodbav:"sourceUrl,omitempty"`
	RecipeID  *string   `json:"recipeId" dynamodbav:"recipeId,omitempty"`
	CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" dynamodbav:"updatedAt"`
}

type CreateJobRequest struct {
	ImageKeys []string `json:"imageKeys"`
	SourceURL *string  `json:"sourceUrl,omitempty"`
}

type CreateJobResponse struct {
	JobID string `json:"jobId"`
}

type JobStore struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	recipes map[string]*Recipe
}

type Recipe struct {
	RecipeID    string    `json:"recipeId" dynamodbav:"recipeId"`
	UserID      string    `json:"userId" dynamodbav:"userId"`
	Title       string    `json:"title" dynamodbav:"title"`
	Ingredients []string  `json:"ingredients" dynamodbav:"ingredients"`
	Steps       []string  `json:"steps" dynamodbav:"steps"`
	ImageKeys   []string  `json:"imageKeys" dynamodbav:"imageKeys"`
	CategoryIDs []string  `json:"categoryIds,omitempty" dynamodbav:"categoryIds,omitempty"`
	SourceURL   *string   `json:"sourceUrl" dynamodbav:"sourceUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt" dynamodbav:"createdAt"`
}

type Category struct {
	CategoryID string    `json:"categoryId" dynamodbav:"categoryId"`
	UserID     string    `json:"userId" dynamodbav:"userId"`
	Name       string    `json:"name" dynamodbav:"name"`
	Color      string    `json:"color" dynamodbav:"color"`
	CreatedAt  time.Time `json:"createdAt" dynamodbav:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt" dynamodbav:"updatedAt"`
}

var jobStore = &JobStore{
	jobs:    make(map[string]*Job),
	recipes: make(map[string]*Recipe),
}

var (
	s3Client      *s3.Client
	s3Presigner   *s3.PresignClient
	bedrockClient *bedrockruntime.Client
	dynamoClient  *dynamodb.Client
	sqsClient     *sqs.Client
	bucketName    string
	jobQueueURL   string
	youtubeAPIKey string
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
	s3Presigner = s3.NewPresignClient(s3Client)
	bedrockClient = bedrockruntime.NewFromConfig(awsCfg)
	dynamoClient = dynamodb.NewFromConfig(awsCfg)
	sqsClient = sqs.NewFromConfig(awsCfg)
	bucketName = bucket
	jobQueueURL = os.Getenv("JOB_QUEUE_URL")
	youtubeAPIKey = os.Getenv("YOUTUBE_API_KEY")

	// ルーターをセットアップ
	r := setupRouter()

	// ローカル開発環境でのみHTTPサーバーを起動
	// Lambda環境ではlambda_handler.goのinitが実行される
	if os.Getenv("_LAMBDA_TELEMETRY_LOG_FD") == "" {
		log.Println("API listening on :8080")
		log.Fatal(http.ListenAndServe(":8080", r))
	}
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
	log.Printf("Downloading image from S3: %s", s3Key)
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &s3Key,
	})
	if err != nil {
		log.Printf("Failed to get object from S3: %s, error: %v", s3Key, err)
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	log.Printf("Reading image data from S3: %s", s3Key)
	imageData, err := io.ReadAll(result.Body)
	if err != nil {
		log.Printf("Failed to read image data from S3: %s, error: %v", s3Key, err)
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}
	log.Printf("Successfully downloaded image from S3: %s, size: %d bytes", s3Key, len(imageData))

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
	prompt.WriteString("- 返答はJSONのみ。前後の説明文、Markdown、コードブロックは出力しないでください\n")
	prompt.WriteString("- JSONの先頭は「{」で始め、末尾は「}」で終えてください\n")

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
	log.Printf("Invoking Bedrock model: %s, region: %s", modelID, bedrockClient.Options().Region)
	result, err := bedrockClient.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		ContentType: aws.String("application/json"),
		Body:        jsonBody,
	})
	if err != nil {
		log.Printf("Bedrock API error details: %+v", err)
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
	// JSONを抽出（コードブロックや前後テキストを除去）
	extracted, err := extractJSON(responseText)
	if err != nil {
		return nil, err
	}

	var llmResponse struct {
		Title       string   `json:"title"`
		Ingredients []string `json:"ingredients"`
		Steps       []string `json:"steps"`
	}

	if err := json.Unmarshal([]byte(extracted), &llmResponse); err != nil {
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
		CategoryIDs: []string{},
		SourceURL:   job.SourceURL,
		CreatedAt:   time.Now(),
	}

	return recipe, nil
}

func extractJSON(responseText string) (string, error) {
	responseText = strings.TrimSpace(responseText)
	if responseText == "" {
		return "", fmt.Errorf("empty response from model")
	}

	if strings.HasPrefix(responseText, "```json") {
		responseText = strings.TrimPrefix(responseText, "```json")
		responseText = strings.TrimSuffix(responseText, "```")
	} else if strings.HasPrefix(responseText, "```") {
		responseText = strings.TrimPrefix(responseText, "```")
		responseText = strings.TrimSuffix(responseText, "```")
	}
	responseText = strings.TrimSpace(responseText)

	start := strings.Index(responseText, "{")
	end := strings.LastIndex(responseText, "}")
	if start == -1 || end == -1 || start >= end {
		return "", fmt.Errorf("failed to locate JSON object in response")
	}

	return strings.TrimSpace(responseText[start : end+1]), nil
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
		CategoryIDs: []string{},
		SourceURL:   job.SourceURL,
		CreatedAt:   now,
	}
}

// processJob は非同期でレシピ生成を行う
func processJob(jobID string) {
	// Lambda関数のタイムアウト（300秒）より短く設定
	ctx, cancel := context.WithTimeout(context.Background(), 280*time.Second)
	defer cancel()

	// DynamoDBからジョブを取得
	job, err := dbStore.GetJob(ctx, jobID)
	if err != nil {
		log.Printf("Failed to get job from DynamoDB: %v", err)
		return
	}
	if job == nil {
		log.Printf("Job not found: %s", jobID)
		return
	}

	// ステータスを更新
	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	if err := dbStore.SaveJob(ctx, job); err != nil {
		log.Printf("Failed to update job status: %v", err)
		return
	}

	var recipe *Recipe
	var recipeErr error

	// 画像がある場合はLLMを使用、YouTube URLの場合はYouTube処理、それ以外はダミー
	if len(job.ImageKeys) > 0 {
		recipe, recipeErr = generateRecipeWithLLM(ctx, job)
		if recipeErr != nil {
			log.Printf("Failed to generate recipe with LLM for job %s: %v, falling back to dummy", jobID, recipeErr)
			recipe = generateDummyRecipe(job)
			recipeErr = nil
		}
	} else if job.SourceURL != nil && *job.SourceURL != "" && isYouTubeURL(*job.SourceURL) {
		recipe, recipeErr = generateRecipeFromYouTube(ctx, job)
		if recipeErr != nil {
			log.Printf("YouTube recipe generation failed, falling back to dummy: %v", recipeErr)
			recipe = generateDummyRecipe(job)
			recipeErr = nil
		}
	} else {
		recipe = generateDummyRecipe(job)
	}

	// ジョブとレシピをDynamoDBに保存
	if recipeErr == nil {
		job.Status = JobStatusSucceeded
	} else {
		job.Status = JobStatusFailed
	}
	job.RecipeID = &recipe.RecipeID
	job.UpdatedAt = time.Now()

	if err := dbStore.SaveJob(ctx, job); err != nil {
		log.Printf("Failed to save job to DynamoDB: %v", err)
		return
	}

	if err := dbStore.SaveRecipe(ctx, recipe); err != nil {
		log.Printf("Failed to save recipe to DynamoDB: %v", err)
		return
	}

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
	log.Printf("Calling Bedrock API for job %s with %d images", job.JobID, len(imageDataList))
	var responseText string
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		responseText, err = callBedrockAPI(ctx, prompt, imageDataList)
		if err == nil {
			log.Printf("Bedrock API call succeeded for job %s", job.JobID)
			break
		}
		log.Printf("Bedrock API call failed (attempt %d/%d) for job %s: %v", i+1, maxRetries, job.JobID, err)
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second // 指数バックオフ: 1秒、2秒、4秒
			log.Printf("Retrying Bedrock API call (attempt %d/%d) after %v", i+1, maxRetries, backoff)
			time.Sleep(backoff)
		}
	}
	if err != nil {
		log.Printf("Failed to call Bedrock API after %d retries for job %s: %v", maxRetries, job.JobID, err)
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

// isYouTubeURL はURLがYouTubeかどうか判定する
func isYouTubeURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	return strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be")
}

// extractYouTubeVideoID はYouTube URLから動画IDを抽出する
func extractYouTubeVideoID(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := strings.ToLower(parsed.Host)

	// youtu.be/xxx
	if strings.Contains(host, "youtu.be") {
		id := strings.TrimPrefix(parsed.Path, "/")
		if id == "" {
			return "", fmt.Errorf("no video ID in youtu.be URL")
		}
		return id, nil
	}

	// youtube.com/watch?v=xxx or youtube.com/shorts/xxx
	if strings.Contains(host, "youtube.com") {
		if strings.HasPrefix(parsed.Path, "/shorts/") {
			id := strings.TrimPrefix(parsed.Path, "/shorts/")
			if id == "" {
				return "", fmt.Errorf("no video ID in shorts URL")
			}
			return id, nil
		}
		v := parsed.Query().Get("v")
		if v != "" {
			return v, nil
		}
		return "", fmt.Errorf("no video ID in youtube.com URL")
	}

	return "", fmt.Errorf("not a YouTube URL")
}

// fetchYouTubeVideoInfo はYouTube Data API v3でタイトル・説明文を取得する
func fetchYouTubeVideoInfo(ctx context.Context, videoID string) (title, description string, err error) {
	if youtubeAPIKey == "" {
		return "", "", fmt.Errorf("YOUTUBE_API_KEY is not set")
	}

	apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?id=%s&part=snippet&key=%s", url.QueryEscape(videoID), url.QueryEscape(youtubeAPIKey))

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch YouTube API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("YouTube API returned status %d", resp.StatusCode)
	}

	var result struct {
		Items []struct {
			Snippet struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"snippet"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode YouTube API response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", "", fmt.Errorf("video not found: %s", videoID)
	}

	return result.Items[0].Snippet.Title, result.Items[0].Snippet.Description, nil
}

// fetchYouTubeCaptions はYouTube動画の字幕テキストを取得する
func fetchYouTubeCaptions(ctx context.Context, videoID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	watchURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	req, err := http.NewRequestWithContext(ctx, "GET", watchURL, nil)
	if err != nil {
		return "", nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "ja,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil
	}
	html := string(body)

	// ytInitialPlayerResponse JSONを正規表現で抽出
	re := regexp.MustCompile(`ytInitialPlayerResponse\s*=\s*(\{.+?\});`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		log.Printf("Could not find ytInitialPlayerResponse for video %s", videoID)
		return "", nil
	}

	var playerResp struct {
		Captions struct {
			PlayerCaptionsTracklistRenderer struct {
				CaptionTracks []struct {
					BaseURL      string `json:"baseUrl"`
					LanguageCode string `json:"languageCode"`
					Kind         string `json:"kind"`
				} `json:"captionTracks"`
			} `json:"playerCaptionsTracklistRenderer"`
		} `json:"captions"`
	}

	if err := json.Unmarshal([]byte(matches[1]), &playerResp); err != nil {
		log.Printf("Failed to parse player response for video %s: %v", videoID, err)
		return "", nil
	}

	tracks := playerResp.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks
	if len(tracks) == 0 {
		log.Printf("No caption tracks found for video %s", videoID)
		return "", nil
	}

	// 優先順位: 日本語手動字幕 → 日本語自動生成 → 英語 → その他
	var selected string
	var jaManual, jaAuto, en, fallback string
	for _, track := range tracks {
		if track.LanguageCode == "ja" && track.Kind != "asr" {
			jaManual = track.BaseURL
		} else if track.LanguageCode == "ja" && track.Kind == "asr" {
			jaAuto = track.BaseURL
		} else if track.LanguageCode == "en" && en == "" {
			en = track.BaseURL
		} else if fallback == "" {
			fallback = track.BaseURL
		}
	}
	switch {
	case jaManual != "":
		selected = jaManual
	case jaAuto != "":
		selected = jaAuto
	case en != "":
		selected = en
	case fallback != "":
		selected = fallback
	default:
		return "", nil
	}

	// 字幕XMLをフェッチ
	captionReq, err := http.NewRequestWithContext(ctx, "GET", selected, nil)
	if err != nil {
		return "", nil
	}

	captionResp, err := http.DefaultClient.Do(captionReq)
	if err != nil {
		return "", nil
	}
	defer captionResp.Body.Close()

	captionBody, err := io.ReadAll(captionResp.Body)
	if err != nil {
		return "", nil
	}

	// XMLをパース
	var transcript struct {
		XMLName xml.Name `xml:"transcript"`
		Texts   []struct {
			Text string `xml:",chardata"`
		} `xml:"text"`
	}

	if err := xml.Unmarshal(captionBody, &transcript); err != nil {
		log.Printf("Failed to parse caption XML for video %s: %v", videoID, err)
		return "", nil
	}

	var parts []string
	for _, t := range transcript.Texts {
		text := strings.TrimSpace(t.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " "), nil
}

// buildPromptForYouTube はYouTube動画情報からLLM用プロンプトを構築する
func buildPromptForYouTube(title, description, captions, sourceURL string) string {
	var prompt strings.Builder

	prompt.WriteString("以下のYouTube料理動画の情報からレシピを抽出してください。\n\n")

	prompt.WriteString("【動画タイトル】\n")
	prompt.WriteString(title + "\n\n")

	if description != "" {
		prompt.WriteString("【説明欄】\n")
		prompt.WriteString(description + "\n\n")
	}

	if captions != "" {
		prompt.WriteString("【字幕テキスト】\n")
		prompt.WriteString(captions + "\n\n")
	}

	prompt.WriteString("【ソースURL】\n")
	prompt.WriteString(sourceURL + "\n\n")

	prompt.WriteString("【出力形式】\n")
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
	prompt.WriteString("- 動画タイトルや説明欄から料理名を特定してください\n")
	prompt.WriteString("- 材料は分量を含めて記載してください（例: \"鶏もも肉 200g\"）\n")
	prompt.WriteString("- 手順は具体的で実行可能な内容にしてください\n")
	prompt.WriteString("- 字幕から調理手順を丁寧に抽出してください\n")
	prompt.WriteString("- 必ず有効なJSON形式で返してください\n")
	prompt.WriteString("- 返答はJSONのみ。前後の説明文、Markdown、コードブロックは出力しないでください\n")
	prompt.WriteString("- JSONの先頭は「{」で始め、末尾は「}」で終えてください\n")

	return prompt.String()
}

// generateRecipeFromYouTube はYouTube動画からレシピを生成する
func generateRecipeFromYouTube(ctx context.Context, job *Job) (*Recipe, error) {
	sourceURL := ""
	if job.SourceURL != nil {
		sourceURL = *job.SourceURL
	}

	videoID, err := extractYouTubeVideoID(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract video ID: %w", err)
	}
	log.Printf("Extracted YouTube video ID: %s", videoID)

	// タイトル・説明文を取得
	title, description, err := fetchYouTubeVideoInfo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch video info: %w", err)
	}
	log.Printf("YouTube video title: %s", title)

	// 字幕を取得（失敗してもエラーにしない）
	captions, err := fetchYouTubeCaptions(ctx, videoID)
	if err != nil {
		log.Printf("Failed to fetch captions for video %s (continuing without): %v", videoID, err)
		captions = ""
	}
	if captions != "" {
		log.Printf("Got %d chars of captions for video %s", len(captions), videoID)
	} else {
		log.Printf("No captions available for video %s", videoID)
	}

	// テキストを最大10000文字に切り詰め
	allText := title + "\n" + description + "\n" + captions
	if len(allText) > 10000 {
		maxDesc := 3000
		maxCaptions := 6000
		if len(description) > maxDesc {
			description = description[:maxDesc]
		}
		if len(captions) > maxCaptions {
			captions = captions[:maxCaptions]
		}
	}

	// プロンプト構築
	prompt := buildPromptForYouTube(title, description, captions, sourceURL)

	// Bedrock APIを呼び出し（リトライ付き）
	log.Printf("Calling Bedrock API for YouTube job %s", job.JobID)
	var responseText string
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err == nil {
			log.Printf("Bedrock API call succeeded for YouTube job %s", job.JobID)
			break
		}
		log.Printf("Bedrock API call failed (attempt %d/%d) for YouTube job %s: %v", i+1, maxRetries, job.JobID, err)
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second
			time.Sleep(backoff)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to call Bedrock API after %d retries: %w", maxRetries, err)
	}

	// レスポンスをパース
	recipe, err := parseRecipeResponse(responseText, job)
	if err != nil {
		log.Printf("Failed to parse YouTube response for job %s, retrying: %v", job.JobID, err)
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to retry Bedrock API: %w", err)
		}
		recipe, err = parseRecipeResponse(responseText, job)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response after retry: %w", err)
		}
	}

	log.Printf("Successfully generated YouTube recipe for job %s: %s", job.JobID, recipe.Title)
	return recipe, nil
}

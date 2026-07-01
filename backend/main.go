package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/joho/godotenv"
)

// --- Types ---

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
	// TargetRecipeID が設定されている場合、新規作成せず既存レシピを上書き再生成する
	TargetRecipeID *string   `json:"targetRecipeId,omitempty" dynamodbav:"targetRecipeId,omitempty"`
	CreatedAt      time.Time `json:"createdAt" dynamodbav:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt" dynamodbav:"updatedAt"`
}

type CreateJobRequest struct {
	ImageKeys []string `json:"imageKeys"`
	SourceURL *string  `json:"sourceUrl,omitempty"`
	// 既存レシピの再作成（上書き）時に対象レシピIDを指定する
	TargetRecipeID *string `json:"targetRecipeId,omitempty"`
}

type CreateJobResponse struct {
	JobID string `json:"jobId"`
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

// --- Global clients ---

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

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i == 1 {
		return "1"
	}
	return "2"
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

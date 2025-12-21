//go:build lambda
// +build lambda

package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	chiadapter "github.com/awslabs/aws-lambda-go-api-proxy/chi"
)

var chiLambda *chiadapter.ChiLambda

func initLambda() {
	// 環境変数から設定を読み込む
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET is required")
	}

	ctx := context.Background()
	
	// Lambda関数ではAWS_REGIONが自動的に設定されるが、明示的に指定する
	// Lambda関数の実行環境からリージョンを取得、またはデフォルトを使用
	region := os.Getenv("AWS_REGION")
	if region == "" {
		// Lambda関数の実行環境変数から取得を試みる
		// それでも取得できない場合はデフォルトリージョンを使用
		region = "ap-northeast-1"
	}
	
	log.Printf("Initializing AWS clients with region: %s", region)
	
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		log.Fatalf("LoadDefaultConfig: %v", err)
	}

	// グローバル変数を初期化
	s3Client = s3.NewFromConfig(awsCfg)
	s3Presigner = s3.NewPresignClient(s3Client)
	bedrockClient = bedrockruntime.NewFromConfig(awsCfg)
	bucketName = bucket

	// ルーターをセットアップ
	r := setupRouter()

	// Lambda用にchiルーターをアダプターでラップ
	chiLambda = chiadapter.New(r)
}

// Handler はLambda関数のエントリーポイント
func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if chiLambda == nil {
		initLambda()
	}
	return chiLambda.ProxyWithContext(ctx, req)
}

func init() {
	// Lambda環境でのみ実行される
	initLambda()
	lambda.Start(Handler)
}


//go:build worker
// +build worker

package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func initWorker() {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET is required")
	}

	ctx := context.Background()
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-northeast-1"
	}

	log.Printf("Initializing worker AWS clients with region: %s", region)
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Fatalf("LoadDefaultConfig: %v", err)
	}

	s3Client = s3.NewFromConfig(awsCfg)
	s3Presigner = s3.NewPresignClient(s3Client)
	bedrockClient = bedrockruntime.NewFromConfig(awsCfg)
	dynamoClient = dynamodb.NewFromConfig(awsCfg)
	bucketName = bucket
	youtubeAPIKey = os.Getenv("YOUTUBE_API_KEY")
}

func WorkerHandler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		jobID := strings.TrimSpace(record.Body)

		var msg jobMessage
		if err := json.Unmarshal([]byte(record.Body), &msg); err == nil && msg.JobID != "" {
			jobID = msg.JobID
		}
		if jobID == "" {
			log.Printf("Skipping empty jobId in message: %s", record.MessageId)
			continue
		}

		log.Printf("Processing job from queue: %s", jobID)
		processJob(jobID)
	}
	return nil
}

func init() {
	initWorker()
	lambda.Start(WorkerHandler)
}

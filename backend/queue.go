package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type jobMessage struct {
	JobID string `json:"jobId"`
}

func dispatchJob(ctx context.Context, jobID string) error {
	if jobQueueURL == "" {
		if os.Getenv("_LAMBDA_TELEMETRY_LOG_FD") != "" {
			return fmt.Errorf("JOB_QUEUE_URL is not set")
		}
		log.Printf("JOB_QUEUE_URL not set; processing inline for local dev")
		go processJob(jobID)
		return nil
	}
	if sqsClient == nil {
		return fmt.Errorf("SQS client is not initialized")
	}

	payload, err := json.Marshal(jobMessage{JobID: jobID})
	if err != nil {
		return fmt.Errorf("failed to marshal job message: %w", err)
	}

	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &jobQueueURL,
		MessageBody: aws.String(string(payload)),
	})
	if err != nil {
		return fmt.Errorf("failed to send job message: %w", err)
	}
	return nil
}

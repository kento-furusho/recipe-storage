package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	jobsTableName    = "recipe-jobs"
	recipesTableName = "recipe-recipes"
)

// DynamoDBStore はDynamoDBを使用したストレージ実装
type DynamoDBStore struct{}

// SaveJob はジョブをDynamoDBに保存する
func (s *DynamoDBStore) SaveJob(ctx context.Context, job *Job) error {
	item, err := attributevalue.MarshalMap(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(jobsTableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put job: %w", err)
	}

	return nil
}

// GetJob はジョブをDynamoDBから取得する
func (s *DynamoDBStore) GetJob(ctx context.Context, jobID string) (*Job, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(jobsTableName),
		Key: map[string]types.AttributeValue{
			"jobId": &types.AttributeValueMemberS{Value: jobID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if result.Item == nil {
		return nil, nil // ジョブが見つからない
	}

	var job Job
	if err := attributevalue.UnmarshalMap(result.Item, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// SaveRecipe はレシピをDynamoDBに保存する
func (s *DynamoDBStore) SaveRecipe(ctx context.Context, recipe *Recipe) error {
	item, err := attributevalue.MarshalMap(recipe)
	if err != nil {
		return fmt.Errorf("failed to marshal recipe: %w", err)
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(recipesTableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put recipe: %w", err)
	}

	return nil
}

// GetRecipe はレシピをDynamoDBから取得する
func (s *DynamoDBStore) GetRecipe(ctx context.Context, recipeID string) (*Recipe, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(recipesTableName),
		Key: map[string]types.AttributeValue{
			"recipeId": &types.AttributeValueMemberS{Value: recipeID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recipe: %w", err)
	}

	if result.Item == nil {
		return nil, nil // レシピが見つからない
	}

	var recipe Recipe
	if err := attributevalue.UnmarshalMap(result.Item, &recipe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipe: %w", err)
	}

	return &recipe, nil
}

// DeleteRecipe はレシピをDynamoDBから削除する
func (s *DynamoDBStore) DeleteRecipe(ctx context.Context, recipeID string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(recipesTableName),
		Key: map[string]types.AttributeValue{
			"recipeId": &types.AttributeValueMemberS{Value: recipeID},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete recipe: %w", err)
	}
	return nil
}

// ListRecipes はレシピ一覧をDynamoDBから取得する
func (s *DynamoDBStore) ListRecipes(ctx context.Context, userID string) ([]*Recipe, error) {
	result, err := dynamoClient.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(recipesTableName),
		FilterExpression: aws.String("userId = :userId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":userId": &types.AttributeValueMemberS{Value: userID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan recipes: %w", err)
	}

	recipes := make([]*Recipe, 0, len(result.Items))
	for _, item := range result.Items {
		var recipe Recipe
		if err := attributevalue.UnmarshalMap(item, &recipe); err != nil {
			continue // エラーがあっても続行
		}
		recipes = append(recipes, &recipe)
	}

	return recipes, nil
}

var dbStore = &DynamoDBStore{}

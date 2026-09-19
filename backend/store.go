package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	jobsTableName        = "recipe-jobs"
	recipesTableName     = "recipe-recipes"
	categoriesTableName  = "recipe-categories"
	ingredientsTableName = "recipe-ingredients"

	// ユーザーごとのレシピを作成日順に引くGSI（template.yaml の RecipesTable を参照）
	recipesByUserIndex = "userId-createdAt-index"
)

// errInvalidCursor はページネーションのカーソルが不正な場合のエラー
var errInvalidCursor = errors.New("invalid cursor")

// recipeCursor はレシピ一覧の続きの位置を表す（最後に返したレシピのキー）。
// userId はカーソルに含めず、認証情報から補完する（他人のデータを引けないようにするため）
type recipeCursor struct {
	RecipeID  string `json:"r"`
	CreatedAt string `json:"c"`
}

func encodeRecipeCursor(c recipeCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeRecipeCursor(s string) (recipeCursor, error) {
	var c recipeCursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, errInvalidCursor
	}
	if err := json.Unmarshal(b, &c); err != nil || c.RecipeID == "" || c.CreatedAt == "" {
		return c, errInvalidCursor
	}
	return c, nil
}

// Store はデータストアのインターフェース
type Store interface {
	SaveJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, jobID string) (*Job, error)
	SaveRecipe(ctx context.Context, recipe *Recipe) error
	GetRecipe(ctx context.Context, recipeID string) (*Recipe, error)
	DeleteRecipe(ctx context.Context, recipeID string) error
	ListRecipes(ctx context.Context, userID string) ([]*Recipe, error)
	// ListRecipesPage は作成日の新しい順に最大 limit 件を返す。続きがあれば次ページのカーソルを返す（なければ空文字）
	ListRecipesPage(ctx context.Context, userID, categoryID string, limit int, cursor string) ([]*Recipe, string, error)
	SaveCategory(ctx context.Context, category *Category) error
	GetCategory(ctx context.Context, categoryID string) (*Category, error)
	DeleteCategory(ctx context.Context, categoryID string) error
	ListCategories(ctx context.Context, userID string) ([]*Category, error)
	// ListIngredients は食材マスタ（全ユーザー共通）を全件返す
	ListIngredients(ctx context.Context) ([]*Ingredient, error)
	// CreateIngredientIfNotExists は同名の食材がまだ無い時だけ登録する。既にあれば created=false
	CreateIngredientIfNotExists(ctx context.Context, ingredient *Ingredient) (created bool, err error)
}

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

// ListRecipes はユーザーの全レシピを作成日の新しい順に取得する
func (s *DynamoDBStore) ListRecipes(ctx context.Context, userID string) ([]*Recipe, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(recipesTableName),
		IndexName:              aws.String(recipesByUserIndex),
		KeyConditionExpression: aws.String("userId = :userId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":userId": &types.AttributeValueMemberS{Value: userID},
		},
		ScanIndexForward: aws.Bool(false),
	}

	recipes := make([]*Recipe, 0)
	for {
		result, err := dynamoClient.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to query recipes: %w", err)
		}
		for _, item := range result.Items {
			var recipe Recipe
			if err := attributevalue.UnmarshalMap(item, &recipe); err != nil {
				continue // エラーがあっても続行
			}
			recipes = append(recipes, &recipe)
		}
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}

	return recipes, nil
}

// ListRecipesPage はユーザーのレシピを作成日の新しい順に最大 limit 件取得する。
// categoryID を指定するとそのカテゴリのレシピだけに絞り込む。
func (s *DynamoDBStore) ListRecipesPage(ctx context.Context, userID, categoryID string, limit int, cursor string) ([]*Recipe, string, error) {
	values := map[string]types.AttributeValue{
		":userId": &types.AttributeValueMemberS{Value: userID},
	}
	input := &dynamodb.QueryInput{
		TableName:                 aws.String(recipesTableName),
		IndexName:                 aws.String(recipesByUserIndex),
		KeyConditionExpression:    aws.String("userId = :userId"),
		ExpressionAttributeValues: values,
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(int32(limit + 1)),
	}
	if categoryID != "" {
		input.FilterExpression = aws.String("contains(categoryIds, :categoryId)")
		values[":categoryId"] = &types.AttributeValueMemberS{Value: categoryID}
	}
	if cursor != "" {
		c, err := decodeRecipeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"recipeId":  &types.AttributeValueMemberS{Value: c.RecipeID},
			"userId":    &types.AttributeValueMemberS{Value: userID},
			"createdAt": &types.AttributeValueMemberS{Value: c.CreatedAt},
		}
	}

	// DynamoDBは Limit 件を読んだ後に FilterExpression を適用するため、1回のQueryでは
	// 件数が足りないことがある。続きの有無を正確に判定するため limit+1 件集まるまで読む。
	type entry struct {
		recipe    *Recipe
		createdAt string // カーソル用に保存値そのままを保持
	}
	collected := make([]entry, 0, limit+1)
	for len(collected) <= limit {
		result, err := dynamoClient.Query(ctx, input)
		if err != nil {
			return nil, "", fmt.Errorf("failed to query recipes: %w", err)
		}
		for _, item := range result.Items {
			var recipe Recipe
			if err := attributevalue.UnmarshalMap(item, &recipe); err != nil {
				continue
			}
			createdAt, _ := item["createdAt"].(*types.AttributeValueMemberS)
			if createdAt == nil {
				continue
			}
			collected = append(collected, entry{recipe: &recipe, createdAt: createdAt.Value})
			if len(collected) > limit {
				break
			}
		}
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}

	nextCursor := ""
	if len(collected) > limit {
		collected = collected[:limit]
		last := collected[limit-1]
		nextCursor = encodeRecipeCursor(recipeCursor{RecipeID: last.recipe.RecipeID, CreatedAt: last.createdAt})
	}

	recipes := make([]*Recipe, 0, len(collected))
	for _, e := range collected {
		recipes = append(recipes, e.recipe)
	}
	return recipes, nextCursor, nil
}

// SaveCategory はカテゴリをDynamoDBに保存する
func (s *DynamoDBStore) SaveCategory(ctx context.Context, category *Category) error {
	item, err := attributevalue.MarshalMap(category)
	if err != nil {
		return fmt.Errorf("failed to marshal category: %w", err)
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(categoriesTableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put category: %w", err)
	}
	return nil
}

// GetCategory はカテゴリをDynamoDBから取得する
func (s *DynamoDBStore) GetCategory(ctx context.Context, categoryID string) (*Category, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(categoriesTableName),
		Key: map[string]types.AttributeValue{
			"categoryId": &types.AttributeValueMemberS{Value: categoryID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get category: %w", err)
	}
	if result.Item == nil {
		return nil, nil
	}

	var category Category
	if err := attributevalue.UnmarshalMap(result.Item, &category); err != nil {
		return nil, fmt.Errorf("failed to unmarshal category: %w", err)
	}
	return &category, nil
}

// DeleteCategory はカテゴリをDynamoDBから削除する
func (s *DynamoDBStore) DeleteCategory(ctx context.Context, categoryID string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(categoriesTableName),
		Key: map[string]types.AttributeValue{
			"categoryId": &types.AttributeValueMemberS{Value: categoryID},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}
	return nil
}

// ListCategories はカテゴリ一覧をDynamoDBから取得する
func (s *DynamoDBStore) ListCategories(ctx context.Context, userID string) ([]*Category, error) {
	result, err := dynamoClient.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(categoriesTableName),
		FilterExpression: aws.String("userId = :userId"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":userId": &types.AttributeValueMemberS{Value: userID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan categories: %w", err)
	}

	categories := make([]*Category, 0, len(result.Items))
	for _, item := range result.Items {
		var category Category
		if err := attributevalue.UnmarshalMap(item, &category); err != nil {
			continue
		}
		categories = append(categories, &category)
	}
	return categories, nil
}

// ListIngredients は食材マスタを全件取得する（数百件規模なのでScanで十分）
func (s *DynamoDBStore) ListIngredients(ctx context.Context) ([]*Ingredient, error) {
	input := &dynamodb.ScanInput{TableName: aws.String(ingredientsTableName)}
	ingredients := make([]*Ingredient, 0)
	for {
		result, err := dynamoClient.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ingredients: %w", err)
		}
		for _, item := range result.Items {
			var ingredient Ingredient
			if err := attributevalue.UnmarshalMap(item, &ingredient); err != nil {
				continue
			}
			ingredients = append(ingredients, &ingredient)
		}
		if result.LastEvaluatedKey == nil {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}
	return ingredients, nil
}

// CreateIngredientIfNotExists は同名の食材がまだ無い時だけ登録する（条件付き書き込みで重複を防ぐ）
func (s *DynamoDBStore) CreateIngredientIfNotExists(ctx context.Context, ingredient *Ingredient) (bool, error) {
	item, err := attributevalue.MarshalMap(ingredient)
	if err != nil {
		return false, fmt.Errorf("failed to marshal ingredient: %w", err)
	}
	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(ingredientsTableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(#name)"),
		ExpressionAttributeNames: map[string]string{
			"#name": "name", // name はDynamoDBの予約語
		},
	})
	var conditionFailed *types.ConditionalCheckFailedException
	if errors.As(err, &conditionFailed) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to put ingredient: %w", err)
	}
	return true, nil
}

var dbStore Store = &DynamoDBStore{}

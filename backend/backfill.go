package main

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ダミーレシピ（generateDummyRecipe）の材料。対応づけの対象外にする
var dummyIngredients = []string{"材料1", "材料2", "材料3"}

// dryRunIngredientStore は食材マスタへの書き込みをメモリ上だけで行う（dry-run用）
type dryRunIngredientStore struct {
	Store
	created map[string]*Ingredient
}

func (d *dryRunIngredientStore) ListIngredients(ctx context.Context) ([]*Ingredient, error) {
	list, err := d.Store.ListIngredients(ctx)
	if err != nil {
		return nil, err
	}
	for _, ingredient := range d.created {
		list = append(list, ingredient)
	}
	return list, nil
}

func (d *dryRunIngredientStore) CreateIngredientIfNotExists(_ context.Context, ingredient *Ingredient) (bool, error) {
	if _, exists := d.created[ingredient.Name]; exists {
		return false, nil
	}
	d.created[ingredient.Name] = ingredient
	return true, nil
}

// runBackfillIngredients は既存レシピの材料を食材マスタに対応づける。
//
//	go run . backfill-ingredients [-dry-run] [-force] [-recipe id1,id2]
func runBackfillIngredients(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("backfill-ingredients", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "食材マスタにもレシピにも書き込まず、結果だけ表示する")
	force := fs.Bool("force", false, "対応づけ済みのレシピもやり直す")
	recipeIDs := fs.String("recipe", "", "指定したレシピID（カンマ区切り）だけをやり直す（-force を含む）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	onlyIDs := make(map[string]bool)
	for _, id := range strings.Split(*recipeIDs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			onlyIDs[id] = true
			*force = true
		}
	}

	recipes, err := scanAllRecipes(ctx)
	if err != nil {
		return err
	}
	// 古い順に処理し、前のレシピで登録された食材を次のレシピが使えるようにする
	sort.SliceStable(recipes, func(i, j int) bool { return recipes[i].CreatedAt.Before(recipes[j].CreatedAt) })

	targets := make([]*Recipe, 0, len(recipes))
	skippedDone, skippedDummy := 0, 0
	for _, recipe := range recipes {
		if len(onlyIDs) > 0 && !onlyIDs[recipe.RecipeID] {
			continue
		}
		switch {
		case len(recipe.Ingredients) == 0 || slices.Equal(recipe.Ingredients, dummyIngredients):
			skippedDummy++
		case recipe.IngredientNames != nil && !*force:
			skippedDone++
		default:
			targets = append(targets, recipe)
		}
	}

	if *dryRun {
		original := dbStore
		dbStore = &dryRunIngredientStore{Store: original, created: map[string]*Ingredient{}}
		defer func() { dbStore = original }()
	}
	resetIngredientMasterCache()

	master, err := loadIngredientMaster(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(master))
	for _, ingredient := range master {
		known[ingredient.Name] = true
	}

	mode := "本実行"
	if *dryRun {
		mode = "dry-run（書き込みなし）"
	}
	fmt.Printf("== 食材の対応づけ: %s ==\n", mode)
	fmt.Printf("全%d件 / 対象%d件（対応づけ済み%d件・ダミーや材料なし%d件はスキップ） / マスタ既存%d件\n\n",
		len(recipes), len(targets), skippedDone, skippedDummy, len(master))

	succeeded, failed := 0, 0
	for i, recipe := range targets {
		names, err := resolveIngredients(ctx, recipe.Ingredients)
		if err != nil {
			failed++
			fmt.Printf("[%d/%d] ✗ %s: %v\n", i+1, len(targets), recipe.Title, err)
			continue
		}

		labels := make([]string, 0, len(names))
		for _, name := range names {
			if known[name] {
				labels = append(labels, name)
			} else {
				labels = append(labels, "★"+name)
				known[name] = true
			}
		}
		fmt.Printf("[%d/%d] %s\n      → %s\n", i+1, len(targets), recipe.Title, strings.Join(labels, "、"))

		if !*dryRun {
			if err := updateRecipeIngredientNames(ctx, recipe.RecipeID, names); err != nil {
				failed++
				fmt.Printf("      ✗ 保存に失敗: %v\n", err)
				continue
			}
		}
		succeeded++
	}

	fmt.Printf("\n成功 %d件 / 失敗 %d件（★は新しく登録される食材）\n", succeeded, failed)
	return printIngredientMaster(ctx)
}

func scanAllRecipes(ctx context.Context) ([]*Recipe, error) {
	input := &dynamodb.ScanInput{TableName: aws.String(recipesTableName)}
	recipes := make([]*Recipe, 0)
	for {
		result, err := dynamoClient.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recipes: %w", err)
		}
		for _, item := range result.Items {
			var recipe Recipe
			if err := attributevalue.UnmarshalMap(item, &recipe); err != nil {
				continue
			}
			recipes = append(recipes, &recipe)
		}
		if result.LastEvaluatedKey == nil {
			return recipes, nil
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
	}
}

// updateRecipeIngredientNames は ingredientNames だけを書き換える（項目全体は上書きしない）
func updateRecipeIngredientNames(ctx context.Context, recipeID string, names []string) error {
	value, err := attributevalue.Marshal(names)
	if err != nil {
		return err
	}
	_, err = dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(recipesTableName),
		Key:                 map[string]types.AttributeValue{"recipeId": &types.AttributeValueMemberS{Value: recipeID}},
		UpdateExpression:    aws.String("SET ingredientNames = :names"),
		ConditionExpression: aws.String("attribute_exists(recipeId)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":names": value,
		},
	})
	return err
}

func printIngredientMaster(ctx context.Context) error {
	master, err := loadIngredientMaster(ctx)
	if err != nil {
		return err
	}
	byCategory := make(map[string][]string)
	for _, ingredient := range master {
		label := ingredient.Name
		if len(ingredient.Aliases) > 0 {
			label += "（" + strings.Join(ingredient.Aliases, "、") + "）"
		}
		if ingredient.IsStaple {
			label += "［基本調味料］"
		}
		byCategory[ingredient.Category] = append(byCategory[ingredient.Category], label)
	}

	fmt.Printf("\n== 食材マスタ（%d件） ==\n", len(master))
	for _, category := range ingredientCategories {
		labels := byCategory[category]
		if len(labels) == 0 {
			continue
		}
		sort.Strings(labels)
		fmt.Printf("■ %s（%d）: %s\n", category, len(labels), strings.Join(labels, "、"))
	}
	return nil
}

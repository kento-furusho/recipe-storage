package main

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

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

	// 規約で自動取得を禁止しているサイトはページを取得しない。
	// 再作成で既存レシピをダミーで上書きしないよう、レシピは保存せずジョブを失敗にする
	if len(job.ImageKeys) == 0 && job.SourceURL != nil && isBlockedRecipeSite(*job.SourceURL) {
		log.Printf("Job %s skipped: %s does not allow automated access", jobID, *job.SourceURL)
		job.Status = JobStatusFailed
		job.UpdatedAt = time.Now()
		if err := dbStore.SaveJob(ctx, job); err != nil {
			log.Printf("Failed to update job status: %v", err)
		}
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

	// YouTube URL優先 → 画像 → 一般URL → ダミー
	hasSourceURL := job.SourceURL != nil && *job.SourceURL != ""
	generatedByLLM := false
	if hasSourceURL && isYouTubeURL(*job.SourceURL) {
		recipe, recipeErr = generateRecipeFromYouTube(ctx, job)
		if recipeErr != nil {
			log.Printf("YouTube recipe generation failed, falling back to dummy: %v", recipeErr)
			recipe = generateDummyRecipe(job)
			recipeErr = nil
		} else {
			generatedByLLM = true
		}
	} else if len(job.ImageKeys) > 0 {
		recipe, recipeErr = generateRecipeWithLLM(ctx, job)
		if recipeErr != nil {
			log.Printf("Failed to generate recipe with LLM for job %s: %v, falling back to dummy", jobID, recipeErr)
			recipe = generateDummyRecipe(job)
			recipeErr = nil
		} else {
			generatedByLLM = true
		}
	} else if hasSourceURL {
		recipe, recipeErr = generateRecipeFromWebPage(ctx, job)
		if recipeErr != nil {
			log.Printf("Web page recipe generation failed for job %s, falling back to dummy: %v", jobID, recipeErr)
			recipe = generateDummyRecipe(job)
			recipeErr = nil
		} else {
			generatedByLLM = true
		}
	} else {
		recipe = generateDummyRecipe(job)
	}

	// 材料を食材マスタに対応づける（食材からの検索用）。失敗してもレシピ自体は保存する
	if generatedByLLM {
		if names, err := resolveIngredients(ctx, recipe.Ingredients); err != nil {
			log.Printf("Failed to resolve ingredients for job %s (saving without them): %v", jobID, err)
		} else {
			recipe.IngredientNames = names
		}
	}

	// 再作成（上書き）の場合は既存レシピのIDを引き継ぎ、カテゴリと作成日を保持する
	if job.TargetRecipeID != nil && *job.TargetRecipeID != "" {
		if existing, err := dbStore.GetRecipe(ctx, *job.TargetRecipeID); err != nil {
			log.Printf("Failed to load target recipe %s for re-creation: %v", *job.TargetRecipeID, err)
		} else if existing != nil {
			recipe.RecipeID = existing.RecipeID
			recipe.CreatedAt = existing.CreatedAt
			recipe.CategoryIDs = existing.CategoryIDs
		}
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

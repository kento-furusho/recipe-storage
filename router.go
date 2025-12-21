package main

import (
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

// setupRouter はルーターをセットアップして返す
func setupRouter() *chi.Mux {
	r := chi.NewRouter()

	// CORS設定
	allowedOrigins := []string{"http://localhost:3000"}
	// 環境変数から追加のオリジンを取得（Vercelデプロイ用）
	if customOrigin := os.Getenv("ALLOWED_ORIGIN"); customOrigin != "" {
		allowedOrigins = append(allowedOrigins, customOrigin)
	}
	// Vercelのドメインを許可（ワイルドカードは使用できないため、環境変数で設定）
	allowedOrigins = append(allowedOrigins, "https://*.vercel.app")

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// ルートを設定
	r.Post("/jobs", handleCreateJob)
	r.Get("/jobs/{jobId}", handleGetJob)
	r.Get("/recipes", handleGetRecipes)
	r.Get("/recipes/{recipeId}", handleGetRecipe)
	r.Get("/image-url", handleGetImageURL)
	r.Post("/upload-url", handleGetUploadURL)

	return r
}


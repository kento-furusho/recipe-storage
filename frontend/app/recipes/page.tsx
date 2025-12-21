"use client";

import { getImageUrl, getRecipes, Recipe } from "@/lib/api";
import Link from "next/link";
import { useEffect, useState } from "react";

export default function RecipesPage() {
  const [recipes, setRecipes] = useState<Recipe[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});

  useEffect(() => {
    loadRecipes();
  }, []);

  const loadRecipes = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getRecipes();
      setRecipes(data);

      // 各レシピの最初の画像のpresigned URLを取得
      const urls: Record<string, string> = {};
      for (const recipe of data) {
        if (recipe.imageKeys.length > 0) {
          try {
            const url = await getImageUrl(recipe.imageKeys[0]);
            urls[recipe.imageKeys[0]] = url;
          } catch (err) {
            console.error(
              `Failed to get image URL for ${recipe.imageKeys[0]}:`,
              err
            );
          }
        }
      }
      setImageUrls(urls);
    } catch (err) {
      console.error("Failed to load recipes:", err);
      setError(
        err instanceof Error ? err.message : "レシピの読み込みに失敗しました"
      );
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 py-8">
        <div className="container mx-auto px-4">
          <div className="text-center">読み込み中...</div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 py-8">
        <div className="container mx-auto px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded">
            {error}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 py-8">
      <div className="container mx-auto px-4">
        <div className="mb-8 flex items-center justify-between">
          <h1 className="text-4xl font-bold text-gray-900">レシピ一覧</h1>
          <Link
            href="/"
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
          >
            新規登録
          </Link>
        </div>

        {recipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-gray-600 text-lg mb-4">
              まだレシピが登録されていません
            </p>
            <Link
              href="/"
              className="text-blue-600 hover:text-blue-800 underline"
            >
              最初のレシピを登録する
            </Link>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {recipes.map((recipe) => (
              <Link
                key={recipe.recipeId}
                href={`/recipes/${recipe.recipeId}`}
                className="bg-white rounded-lg shadow-md overflow-hidden hover:shadow-lg transition-shadow"
              >
                {recipe.imageKeys.length > 0 && (
                  <div className="aspect-video bg-gray-200 relative">
                    {imageUrls[recipe.imageKeys[0]] ? (
                      <img
                        src={imageUrls[recipe.imageKeys[0]]}
                        alt={recipe.title}
                        className="w-full h-full object-cover"
                      />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-gray-400 text-sm">
                        読み込み中...
                      </div>
                    )}
                  </div>
                )}
                <div className="p-4">
                  <h2 className="text-xl font-semibold text-gray-900 mb-2">
                    {recipe.title}
                  </h2>
                  <p className="text-sm text-gray-500">
                    {new Date(recipe.createdAt).toLocaleDateString("ja-JP")}
                  </p>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

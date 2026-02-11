"use client";

import {
  deleteRecipe,
  getCategories,
  getImageUrl,
  getRecipes,
  Category,
  Recipe,
} from "@/lib/api";
import Link from "next/link";
import { useEffect, useState } from "react";

export default function RecipesPage() {
  const [recipes, setRecipes] = useState<Recipe[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [allCategories, setAllCategories] = useState<Category[]>([]);
  const [categoryMap, setCategoryMap] = useState<Record<string, Category>>({});

  useEffect(() => {
    loadRecipes();
  }, []);

  const loadRecipes = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getRecipes();
      setRecipes(data);
      const categories = await getCategories();
      const sorted = [...categories].sort((a, b) =>
        a.name.localeCompare(b.name, "ja")
      );
      setAllCategories(sorted);
      const map: Record<string, Category> = {};
      for (const category of sorted) {
        map[category.categoryId] = category;
      }
      setCategoryMap(map);

      // 各レシピの最初の画像のpresigned URLを取得
      const urls: Record<string, string> = {};
      for (const recipe of data) {
        if (recipe.imageKeys && recipe.imageKeys.length > 0) {
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

  const filteredRecipes =
    selectedCategory === "all"
      ? recipes
      : recipes.filter((recipe) =>
          (recipe.categoryIds || []).includes(selectedCategory)
        );

  const handleDelete = async (
    event: React.MouseEvent<HTMLButtonElement>,
    recipeId: string
  ) => {
    event.preventDefault();
    event.stopPropagation();

    if (deletingId) {
      return;
    }
    if (!window.confirm("このレシピを削除しますか？")) {
      return;
    }

    try {
      setDeletingId(recipeId);
      setError(null);
      await deleteRecipe(recipeId);
      setRecipes((prev) => prev.filter((recipe) => recipe.recipeId !== recipeId));
    } catch (err) {
      console.error("Failed to delete recipe:", err);
      setError(
        err instanceof Error ? err.message : "レシピの削除に失敗しました"
      );
    } finally {
      setDeletingId(null);
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
          <div>
            <h1 className="text-4xl font-bold text-gray-900">レシピ一覧</h1>
            {allCategories.length > 0 && (
              <div className="mt-3 flex items-center gap-2">
                <label className="text-sm text-gray-600">カテゴリ</label>
                <select
                  value={selectedCategory}
                  onChange={(event) => setSelectedCategory(event.target.value)}
                  className="rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700"
                >
                  <option value="all">すべて</option>
                  {allCategories.map((category) => (
                    <option key={category.categoryId} value={category.categoryId}>
                      {category.name}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </div>
          <Link
            href="/"
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
          >
            新規登録
          </Link>
        </div>
        <div className="mb-6">
          <Link
            href="/categories"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            カテゴリ管理へ
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
        ) : filteredRecipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-gray-600 text-lg mb-4">
              選択したカテゴリのレシピがありません
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredRecipes.map((recipe) => (
              <div
                key={recipe.recipeId}
                className="bg-white rounded-lg shadow-md overflow-hidden hover:shadow-lg transition-shadow"
              >
                <Link href={`/recipes/${recipe.recipeId}`}>
                  {recipe.imageKeys && recipe.imageKeys.length > 0 && (
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
                    {recipe.categoryIds && recipe.categoryIds.length > 0 && (
                      <div className="mb-2 flex flex-wrap gap-2">
                        {recipe.categoryIds.map((categoryId) => {
                          const category = categoryMap[categoryId];
                          if (!category) return null;
                          return (
                            <span
                              key={category.categoryId}
                              className="px-2 py-1 rounded-full text-xs font-medium"
                              style={{
                                backgroundColor: `${category.color}20`,
                                color: category.color,
                              }}
                            >
                              {category.name}
                            </span>
                          );
                        })}
                      </div>
                    )}
                    <p className="text-sm text-gray-500">
                      {new Date(recipe.createdAt).toLocaleString("ja-JP")}
                    </p>
                  </div>
                </Link>
                <div className="px-4 pb-4 flex gap-2">
                  <Link
                    href={`/recipes/${recipe.recipeId}`}
                    className="flex-1 px-3 py-2 rounded-lg border border-gray-200 text-gray-700 hover:bg-gray-50 text-center transition-colors"
                  >
                    編集
                  </Link>
                  <button
                    type="button"
                    onClick={(event) => handleDelete(event, recipe.recipeId)}
                    disabled={deletingId === recipe.recipeId}
                    className="flex-1 px-3 py-2 rounded-lg border border-red-200 text-red-700 hover:bg-red-50 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {deletingId === recipe.recipeId ? "削除中..." : "削除"}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

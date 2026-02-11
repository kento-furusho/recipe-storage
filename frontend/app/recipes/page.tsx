"use client";

import AuthGuard from "@/components/AuthGuard";
import {
  bulkAssignCategories,
  deleteRecipe,
  getCategories,
  getImageUrl,
  getRecipes,
  Category,
  Recipe,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import Link from "next/link";
import { useEffect, useState } from "react";

export default function RecipesPage() {
  return (
    <AuthGuard>
      <RecipesContent />
    </AuthGuard>
  );
}

function RecipesContent() {
  const { signOut } = useAuth();
  const [recipes, setRecipes] = useState<Recipe[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [allCategories, setAllCategories] = useState<Category[]>([]);
  const [categoryMap, setCategoryMap] = useState<Record<string, Category>>({});
  const [selectedRecipeIds, setSelectedRecipeIds] = useState<Set<string>>(
    () => new Set()
  );
  const [bulkCategoryIds, setBulkCategoryIds] = useState<Set<string>>(
    () => new Set()
  );
  const [bulkError, setBulkError] = useState<string | null>(null);
  const [bulkResult, setBulkResult] = useState<{
    updatedIds: string[];
    failed: { recipeId: string; reason: string }[];
  } | null>(null);
  const [bulkLoading, setBulkLoading] = useState(false);
  const bulkLimit = 50;

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
      setSelectedRecipeIds(new Set());
      setBulkResult(null);
      setBulkError(null);

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

  const allSelected =
    filteredRecipes.length > 0 &&
    filteredRecipes.every((recipe) => selectedRecipeIds.has(recipe.recipeId));

  const toggleSelectAll = () => {
    setBulkError(null);
    setBulkResult(null);
    setSelectedRecipeIds((prev) => {
      const next = new Set(prev);
      if (allSelected) {
        for (const recipe of filteredRecipes) {
          next.delete(recipe.recipeId);
        }
        return next;
      }
      const combined = new Set(prev);
      for (const recipe of filteredRecipes) {
        combined.add(recipe.recipeId);
      }
      return combined;
    });
  };

  const toggleSelectRecipe = (recipeId: string) => {
    setBulkError(null);
    setBulkResult(null);
    setSelectedRecipeIds((prev) => {
      const next = new Set(prev);
      if (next.has(recipeId)) {
        next.delete(recipeId);
      } else {
        next.add(recipeId);
      }
      return next;
    });
  };

  const toggleBulkCategory = (categoryId: string) => {
    setBulkError(null);
    setBulkResult(null);
    setBulkCategoryIds((prev) => {
      const next = new Set(prev);
      if (next.has(categoryId)) {
        next.delete(categoryId);
      } else {
        next.add(categoryId);
      }
      return next;
    });
  };

  const handleBulkAssign = async () => {
    const recipeIds = Array.from(selectedRecipeIds);
    const categoryIds = Array.from(bulkCategoryIds);

    if (recipeIds.length === 0) {
      setBulkError("レシピを選択してください");
      return;
    }
    if (recipeIds.length > bulkLimit) {
      setBulkError(`一括操作は最大${bulkLimit}件までです`);
      return;
    }
    if (categoryIds.length === 0) {
      setBulkError("追加するカテゴリを選択してください");
      return;
    }

    if (
      !window.confirm(
        `選択した${recipeIds.length}件にカテゴリを追加します。よろしいですか？`
      )
    ) {
      return;
    }

    try {
      setBulkLoading(true);
      setBulkError(null);
      const result = await bulkAssignCategories(recipeIds, categoryIds);
      setBulkResult(result);
      await loadRecipes();
    } catch (err) {
      console.error("Failed to bulk assign categories:", err);
      setBulkError(
        err instanceof Error ? err.message : "一括割り当てに失敗しました"
      );
    } finally {
      setBulkLoading(false);
    }
  };

  const handleDelete = async (recipeId: string) => {
    if (deletingId) {
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
      <div className="min-h-screen bg-[#F6F1EB] py-8">
        <div className="container mx-auto px-4">
          <div className="text-center">読み込み中...</div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-[#F6F1EB] py-8">
        <div className="container mx-auto px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg">
            {error}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#F6F1EB] py-8">
      <div className="container mx-auto px-4">
        <div className="mb-8 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 className="text-4xl font-bold text-stone-800">レシピ一覧</h1>
            {allCategories.length > 0 && (
              <div className="mt-3 flex items-center gap-2">
                <label className="text-sm text-stone-600">カテゴリ</label>
                <select
                  value={selectedCategory}
                  onChange={(event) => setSelectedCategory(event.target.value)}
                  className="rounded-lg border border-amber-100 bg-white px-3 py-2 text-sm text-stone-700"
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
          <div className="flex items-center gap-3">
            <Link
              href="/"
              className="px-4 py-2 bg-amber-500 text-white rounded-lg hover:bg-amber-600 transition-colors shadow-sm"
            >
              新規登録
            </Link>
            <button
              type="button"
              onClick={() => signOut()}
              className="px-4 py-2 rounded-lg border border-stone-300 text-stone-600 hover:bg-stone-100 transition-colors text-sm"
            >
              サインアウト
            </button>
          </div>
        </div>
        <div className="mb-6">
          <Link
            href="/categories"
            className="inline-flex items-center gap-2 rounded-lg border border-amber-100 bg-white px-4 py-2 text-sm font-semibold text-stone-700 shadow-sm transition hover:bg-amber-50/60 hover:text-stone-900"
          >
            <span
              className="inline-flex h-2.5 w-2.5 rounded-full"
              style={{ background: "linear-gradient(135deg, #f3c2a1, #f0b38a)" }}
            />
            カテゴリ管理へ
          </Link>
        </div>

        {allCategories.length > 0 && (
          <details className="mb-8 rounded-lg border border-amber-50 bg-white p-4 shadow-lg">
            <summary className="flex cursor-pointer list-none items-center justify-between">
              <div>
                <h2 className="text-lg font-semibold text-stone-800">
                  一括カテゴリ追加
                </h2>
                <p className="text-sm text-stone-500">
                  一覧からレシピを選択し、カテゴリを追加します（最大{bulkLimit}件）
                </p>
              </div>
              <span className="text-sm text-stone-500">開く</span>
            </summary>

            <div className="mt-4 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex items-center gap-2">
                <label className="text-sm text-stone-600">全選択</label>
                <input type="checkbox" checked={allSelected} onChange={toggleSelectAll} />
                <span className="text-sm text-stone-500">
                  選択中: {selectedRecipeIds.size}件
                </span>
              </div>
            </div>

            <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
              {allCategories.map((category) => (
                <label
                  key={category.categoryId}
                  className="flex items-center gap-2 rounded border border-amber-50 bg-amber-50/30 px-3 py-2 text-sm text-stone-700"
                >
                  <input
                    type="checkbox"
                    checked={bulkCategoryIds.has(category.categoryId)}
                    onChange={() => toggleBulkCategory(category.categoryId)}
                  />
                  <span
                    className="inline-block h-3 w-3 rounded-full"
                    style={{ backgroundColor: category.color }}
                  />
                  {category.name}
                </label>
              ))}
            </div>

            {bulkError && (
              <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                {bulkError}
              </div>
            )}

            {bulkResult && (
              <div className="mt-4 rounded-lg border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-800">
                更新成功: {bulkResult.updatedIds.length}件
                {bulkResult.failed.length > 0 && (
                  <div className="mt-2 text-sm text-red-700">
                    失敗: {bulkResult.failed.length}件
                    <ul className="mt-1 list-disc list-inside">
                      {bulkResult.failed.map((item) => (
                        <li key={item.recipeId}>
                          {item.recipeId}: {item.reason}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}

            <div className="mt-4">
              <button
                type="button"
                onClick={handleBulkAssign}
                disabled={bulkLoading}
                className="px-4 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50"
              >
                {bulkLoading ? "処理中..." : "一括追加"}
              </button>
            </div>
          </details>
        )}

        {recipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-stone-600 text-lg mb-4">
              まだレシピが登録されていません
            </p>
            <Link
              href="/"
              className="text-amber-700 hover:text-amber-900 underline"
            >
              最初のレシピを登録する
            </Link>
          </div>
        ) : filteredRecipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-stone-600 text-lg mb-4">
              選択したカテゴリのレシピがありません
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredRecipes.map((recipe) => (
              <div
                key={recipe.recipeId}
                className="bg-white rounded-lg shadow-lg overflow-hidden hover:shadow-xl transition-shadow"
              >
                <div
                  className="flex items-center justify-between px-4 py-3 border-b border-amber-50 cursor-pointer"
                  onClick={() => toggleSelectRecipe(recipe.recipeId)}
                >
                  <label className="flex items-center gap-2 text-sm text-stone-600">
                    <input
                      type="checkbox"
                      checked={selectedRecipeIds.has(recipe.recipeId)}
                      onChange={() => toggleSelectRecipe(recipe.recipeId)}
                      onClick={(event) => event.stopPropagation()}
                    />
                    選択
                  </label>
                  <div className="flex items-center gap-2">
                    <Link
                      href={`/recipes/${recipe.recipeId}`}
                      onClick={(event) => event.stopPropagation()}
                      className="p-2 rounded-full border border-amber-50 text-stone-600 hover:bg-amber-50/60"
                      aria-label="編集"
                    >
                      <svg
                        viewBox="0 0 24 24"
                        className="h-4 w-4"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M12 20h9" />
                        <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
                      </svg>
                    </Link>
                    <button
                      type="button"
                      onClick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        if (!window.confirm("このレシピを削除しますか？")) {
                          return;
                        }
                        handleDelete(recipe.recipeId);
                      }}
                      disabled={deletingId === recipe.recipeId}
                      className="p-2 rounded-full border border-red-200 text-red-600 hover:bg-red-50 disabled:opacity-50 disabled:cursor-not-allowed"
                      aria-label="削除"
                    >
                      <svg
                        viewBox="0 0 24 24"
                        className="h-4 w-4"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M3 6h18" />
                        <path d="M8 6V4h8v2" />
                        <path d="M19 6l-1 14H6L5 6" />
                        <path d="M10 11v6" />
                        <path d="M14 11v6" />
                      </svg>
                    </button>
                  </div>
                </div>
                <Link href={`/recipes/${recipe.recipeId}`}>
                  {recipe.imageKeys && recipe.imageKeys.length > 0 && (
                    <div className="aspect-video bg-amber-50/60 relative">
                      {imageUrls[recipe.imageKeys[0]] ? (
                        <img
                          src={imageUrls[recipe.imageKeys[0]]}
                          alt={recipe.title}
                          className="w-full h-full object-cover"
                        />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-stone-400 text-sm">
                          読み込み中...
                        </div>
                      )}
                    </div>
                  )}
                  <div className="p-4">
                    <h2 className="text-xl font-semibold text-stone-800 mb-2">
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
                    <p className="text-sm text-stone-500">
                      {new Date(recipe.createdAt).toLocaleString("ja-JP")}
                    </p>
                  </div>
                </Link>
                <div className="px-4 pb-4" />
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

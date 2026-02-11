"use client";

import AuthGuard from "@/components/AuthGuard";
import MobileHeader from "@/components/MobileHeader";
import {
  bulkAssignCategories,
  getCategories,
  getImageUrl,
  getRecipes,
  Category,
  Recipe,
} from "@/lib/api";
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
  const [recipes, setRecipes] = useState<Recipe[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [allCategories, setAllCategories] = useState<Category[]>([]);
  const [categoryMap, setCategoryMap] = useState<Record<string, Category>>({});
  const [bulkMode, setBulkMode] = useState(false);
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
      if (allSelected) {
        const next = new Set(prev);
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

  if (loading) {
    return (
      <div>
        <MobileHeader title="レシピ一覧" />
        <div className="px-3 py-8 sm:px-4 text-center text-stone-600">読み込み中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <MobileHeader title="レシピ一覧" />
        <div className="px-3 py-4 sm:px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm">
            {error}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <MobileHeader
        title="レシピ一覧"
        rightAction={
          allCategories.length > 0 ? (
            <button
              type="button"
              onClick={() => {
                setBulkMode(!bulkMode);
                if (bulkMode) {
                  setSelectedRecipeIds(new Set());
                  setBulkCategoryIds(new Set());
                  setBulkError(null);
                  setBulkResult(null);
                }
              }}
              className="min-h-[44px] px-3 py-2 text-sm font-medium text-amber-600"
            >
              {bulkMode ? "完了" : "選択"}
            </button>
          ) : undefined
        }
      />

      {/* Sticky filter bar */}
      {allCategories.length > 0 && (
        <div className="sticky top-[48px] z-30 bg-white/80 backdrop-blur-sm border-b border-stone-100 px-3 py-2 sm:px-4">
          <select
            value={selectedCategory}
            onChange={(event) => setSelectedCategory(event.target.value)}
            className="w-full sm:w-auto rounded-lg border border-amber-100 bg-white px-3 py-2 text-base text-stone-700"
          >
            <option value="all">すべてのカテゴリ</option>
            {allCategories.map((category) => (
              <option key={category.categoryId} value={category.categoryId}>
                {category.name}
              </option>
            ))}
          </select>
        </div>
      )}

      <div className="px-3 py-4 sm:px-4 lg:px-6">
        {/* Bulk mode panel */}
        {bulkMode && allCategories.length > 0 && (
          <div className="mb-4 rounded-lg border border-amber-50 bg-white p-4 shadow-sm">
            <div className="flex items-center gap-2 mb-3">
              <label className="flex items-center gap-2 text-sm text-stone-600">
                <input type="checkbox" checked={allSelected} onChange={toggleSelectAll} />
                全選択
              </label>
              <span className="text-sm text-stone-500">
                選択中: {selectedRecipeIds.size}件
              </span>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 mb-3">
              {allCategories.map((category) => (
                <label
                  key={category.categoryId}
                  className="flex items-center gap-2 rounded border border-amber-50 bg-amber-50/30 px-3 py-2 text-sm text-stone-700 min-h-[44px]"
                >
                  <input
                    type="checkbox"
                    checked={bulkCategoryIds.has(category.categoryId)}
                    onChange={() => toggleBulkCategory(category.categoryId)}
                  />
                  <span
                    className="inline-block h-3 w-3 rounded-full shrink-0"
                    style={{ backgroundColor: category.color }}
                  />
                  <span className="truncate">{category.name}</span>
                </label>
              ))}
            </div>

            {bulkError && (
              <div className="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                {bulkError}
              </div>
            )}

            {bulkResult && (
              <div className="mb-3 rounded-lg border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-800">
                更新成功: {bulkResult.updatedIds.length}件
                {bulkResult.failed.length > 0 && (
                  <div className="mt-2 text-sm text-red-700">
                    失敗: {bulkResult.failed.length}件
                  </div>
                )}
              </div>
            )}

            <button
              type="button"
              onClick={handleBulkAssign}
              disabled={bulkLoading}
              className="w-full sm:w-auto min-h-[44px] px-4 py-3 rounded-lg bg-amber-500 text-white text-base font-medium hover:bg-amber-600 disabled:opacity-50"
            >
              {bulkLoading ? "処理中..." : "一括追加"}
            </button>
          </div>
        )}

        {recipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-stone-600 mb-4">
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
            <p className="text-stone-600">
              選択したカテゴリのレシピがありません
            </p>
          </div>
        ) : (
          <div className="space-y-3 sm:grid sm:grid-cols-2 sm:gap-4 sm:space-y-0 lg:grid-cols-3">
            {filteredRecipes.map((recipe) => (
              <div
                key={recipe.recipeId}
                className="bg-white rounded-lg shadow-sm overflow-hidden"
              >
                <Link href={`/recipes/${recipe.recipeId}`} className="flex sm:flex-col">
                  {/* Thumbnail */}
                  {recipe.imageKeys && recipe.imageKeys.length > 0 && (
                    <div className="w-24 h-24 sm:w-full sm:h-auto sm:aspect-video bg-amber-50/60 shrink-0">
                      {imageUrls[recipe.imageKeys[0]] ? (
                        <img
                          src={imageUrls[recipe.imageKeys[0]]}
                          alt={recipe.title}
                          className="w-full h-full object-cover"
                        />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-stone-400 text-xs">
                          ...
                        </div>
                      )}
                    </div>
                  )}
                  {/* Content */}
                  <div className="flex-1 p-3 sm:p-4 min-w-0">
                    <h2 className="text-base sm:text-lg font-semibold text-stone-800 line-clamp-2">
                      {recipe.title}
                    </h2>
                    {recipe.categoryIds && recipe.categoryIds.length > 0 && (
                      <div className="mt-1.5 flex flex-wrap gap-1">
                        {recipe.categoryIds.map((categoryId) => {
                          const category = categoryMap[categoryId];
                          if (!category) return null;
                          return (
                            <span
                              key={category.categoryId}
                              className="px-2 py-0.5 rounded-full text-xs font-medium"
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
                    <p className="text-xs text-stone-400 mt-1.5">
                      {new Date(recipe.createdAt).toLocaleString("ja-JP")}
                    </p>
                  </div>
                </Link>
                {/* Bulk select checkbox */}
                {bulkMode && (
                  <div
                    className="px-3 pb-3 sm:px-4 sm:pb-4"
                    onClick={() => toggleSelectRecipe(recipe.recipeId)}
                  >
                    <label className="flex items-center gap-2 text-sm text-stone-600 min-h-[44px]">
                      <input
                        type="checkbox"
                        checked={selectedRecipeIds.has(recipe.recipeId)}
                        onChange={() => toggleSelectRecipe(recipe.recipeId)}
                        onClick={(event) => event.stopPropagation()}
                      />
                      選択
                    </label>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

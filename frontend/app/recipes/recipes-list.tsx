"use client";

import MobileHeader from "@/components/MobileHeader";
import {
  bulkAssignCategories,
  getCategories,
  getImageUrl,
  getRecipesPage,
  Category,
  Recipe,
} from "@/lib/api";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

const PAGE_SIZE = 10;

// レシピ一覧画面の中身（認証は呼び出し側の page.tsx で行う）
export default function RecipesList() {
  const [recipes, setRecipes] = useState<Recipe[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [listLoading, setListLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // カテゴリを素早く切り替えた時に古いレスポンスで上書きしないためのリクエスト番号
  const requestIdRef = useRef(0);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [keyword, setKeyword] = useState("");
  // 実際に検索に使われた語（結果の見出し用。入力中の語とはずれる）
  const [appliedKeyword, setAppliedKeyword] = useState("");
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

  // 表示するレシピのサムネイルURLを並列で取得してマージする
  const loadImageUrls = useCallback(async (items: Recipe[]) => {
    const keys = items
      .map((recipe) => recipe.imageKeys?.[0])
      .filter((key): key is string => !!key);
    const entries = await Promise.all(
      keys.map(async (key) => {
        try {
          return [key, await getImageUrl(key)] as const;
        } catch (err) {
          console.error(`Failed to get image URL for ${key}:`, err);
          return null;
        }
      })
    );
    setImageUrls((prev) => {
      const next = { ...prev };
      for (const entry of entries) {
        if (entry) next[entry[0]] = entry[1];
      }
      return next;
    });
  }, []);

  // 先頭ページから読み直す（初回・カテゴリ変更・検索・一括操作後）
  const loadFirstPage = useCallback(
    async (category: string, keyword: string) => {
      const requestId = ++requestIdRef.current;
      try {
        setListLoading(true);
        setError(null);
        const page = await getRecipesPage({
          limit: PAGE_SIZE,
          categoryId: category === "all" ? undefined : category,
          q: keyword.trim() || undefined,
        });
        if (requestId !== requestIdRef.current) return;
        setRecipes(page.items);
        setNextCursor(page.nextCursor);
        setSelectedRecipeIds(new Set());
        loadImageUrls(page.items);
      } catch (err) {
        if (requestId !== requestIdRef.current) return;
        console.error("Failed to load recipes:", err);
        setError(
          err instanceof Error ? err.message : "レシピの読み込みに失敗しました"
        );
      } finally {
        if (requestId === requestIdRef.current) {
          setListLoading(false);
          setLoading(false);
        }
      }
    },
    [loadImageUrls]
  );

  const loadMore = async () => {
    if (!nextCursor || loadingMore) return;
    const requestId = requestIdRef.current;
    try {
      setLoadingMore(true);
      const page = await getRecipesPage({
        limit: PAGE_SIZE,
        cursor: nextCursor,
        categoryId: selectedCategory === "all" ? undefined : selectedCategory,
      });
      // 読み込み中にカテゴリが切り替わっていたら捨てる
      if (requestId !== requestIdRef.current) return;
      setRecipes((prev) => [...prev, ...page.items]);
      setNextCursor(page.nextCursor);
      loadImageUrls(page.items);
    } catch (err) {
      console.error("Failed to load more recipes:", err);
      setError(
        err instanceof Error ? err.message : "レシピの読み込みに失敗しました"
      );
    } finally {
      setLoadingMore(false);
    }
  };

  useEffect(() => {
    const loadCategories = async () => {
      try {
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
      } catch (err) {
        console.error("Failed to load categories:", err);
      }
    };
    loadCategories();
  }, []);

  // カテゴリ変更は即時、キーワードは入力から0.3秒後に読み直す
  useEffect(() => {
    setBulkResult(null);
    setBulkError(null);
    const timer = setTimeout(() => {
      setAppliedKeyword(keyword.trim());
      loadFirstPage(selectedCategory, keyword);
    }, keyword === "" ? 0 : 300);
    return () => clearTimeout(timer);
  }, [selectedCategory, keyword, loadFirstPage]);

  const allSelected =
    recipes.length > 0 &&
    recipes.every((recipe) => selectedRecipeIds.has(recipe.recipeId));

  const toggleSelectAll = () => {
    setBulkError(null);
    setBulkResult(null);
    setSelectedRecipeIds((prev) => {
      if (allSelected) {
        const next = new Set(prev);
        for (const recipe of recipes) {
          next.delete(recipe.recipeId);
        }
        return next;
      }
      const combined = new Set(prev);
      for (const recipe of recipes) {
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
      await loadFirstPage(selectedCategory, keyword);
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

  // 一覧を1件も表示できない時だけエラー画面にする（「もっと見る」の失敗では一覧を残す）
  if (error && recipes.length === 0) {
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
      <div className="sticky top-[48px] z-30 bg-white/80 backdrop-blur-sm border-b border-stone-100 px-3 py-2 sm:px-4 flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <input
            type="search"
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="レシピ名・材料で検索"
            className="w-full rounded-lg border border-amber-100 bg-white pl-3 pr-9 py-2 text-base text-stone-700 focus:ring-2 focus:ring-amber-200 focus:border-amber-200 [&::-webkit-search-cancel-button]:appearance-none"
          />
          {keyword && (
            <button
              type="button"
              onClick={() => setKeyword("")}
              aria-label="検索をクリア"
              className="absolute right-1 top-1/2 -translate-y-1/2 w-8 h-8 flex items-center justify-center text-stone-400 hover:text-stone-600"
            >
              ×
            </button>
          )}
        </div>
        {allCategories.length > 0 && (
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
        )}
      </div>

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

        {!listLoading && appliedKeyword && (
          <p className="mb-3 text-sm text-stone-600">
            「{appliedKeyword}」の検索結果 {recipes.length}件
          </p>
        )}

        {listLoading ? (
          <div className="py-12 text-center text-stone-600">読み込み中...</div>
        ) : recipes.length === 0 && appliedKeyword ? (
          <div className="text-center py-12">
            <p className="text-stone-600">
              「{appliedKeyword}」に一致するレシピがありません
            </p>
          </div>
        ) : recipes.length === 0 && selectedCategory === "all" ? (
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
        ) : recipes.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-stone-600">
              選択したカテゴリのレシピがありません
            </p>
          </div>
        ) : (
          <div className="space-y-3 sm:grid sm:grid-cols-2 sm:gap-4 sm:space-y-0 lg:grid-cols-3">
            {recipes.map((recipe) => (
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

        {error && recipes.length > 0 && (
          <div className="mt-4 bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm">
            {error}
          </div>
        )}

        {!listLoading && nextCursor && (
          <div className="mt-6 flex justify-center">
            <button
              type="button"
              onClick={loadMore}
              disabled={loadingMore}
              className="min-h-[44px] w-full sm:w-auto px-8 py-3 rounded-lg border border-amber-200 bg-white text-base font-medium text-amber-700 hover:bg-amber-50 disabled:opacity-50"
            >
              {loadingMore ? "読み込み中..." : "もっと見る"}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

"use client";

import AuthGuard from "@/components/AuthGuard";
import MobileHeader from "@/components/MobileHeader";
import {
  deleteRecipe,
  getCategories,
  getImageUrl,
  getRecipe,
  Recipe,
  Category,
  updateRecipe,
} from "@/lib/api";
import RecipeImageManager, {
  RecipeImageManagerRef,
} from "@/components/RecipeImageManager";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

export default function RecipeDetailPage() {
  return (
    <AuthGuard>
      <RecipeDetailContent />
    </AuthGuard>
  );
}

function RecipeDetailContent() {
  const params = useParams();
  const recipeId = params.recipeId as string;
  const router = useRouter();

  const [recipe, setRecipe] = useState<Recipe | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [deleting, setDeleting] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editingImageKeys, setEditingImageKeys] = useState<string[]>([]);
  const imageManagerRef = useRef<RecipeImageManagerRef>(null);
  const [categories, setCategories] = useState<Category[]>([]);
  const [categoryMap, setCategoryMap] = useState<Record<string, Category>>({});
  const [form, setForm] = useState({
    title: "",
    sourceUrl: "",
    ingredientsText: "",
    stepsText: "",
    selectedCategoryIds: [] as string[],
  });

  useEffect(() => {
    if (recipeId) {
      loadRecipe();
    }
  }, [recipeId]);

  const loadRecipe = async () => {
    try {
      setLoading(true);
      setError(null);
      const [data, categoryList] = await Promise.all([
        getRecipe(recipeId),
        getCategories(),
      ]);
      setRecipe(data);
      const sortedCategories = [...categoryList].sort((a, b) =>
        a.name.localeCompare(b.name, "ja")
      );
      setCategories(sortedCategories);
      const map: Record<string, Category> = {};
      for (const category of sortedCategories) {
        map[category.categoryId] = category;
      }
      setCategoryMap(map);
      setForm({
        title: data.title,
        sourceUrl: data.sourceUrl || "",
        ingredientsText: (data.ingredients || []).join("\n"),
        stepsText: (data.steps || []).join("\n"),
        selectedCategoryIds: data.categoryIds || [],
      });

      const urls: Record<string, string> = {};
      if (data.imageKeys && data.imageKeys.length > 0) {
        for (const imageKey of data.imageKeys) {
          try {
            const url = await getImageUrl(imageKey);
            urls[imageKey] = url;
          } catch (err) {
            console.error(`Failed to get image URL for ${imageKey}:`, err);
          }
        }
      }
      setImageUrls(urls);
    } catch (err) {
      console.error("Failed to load recipe:", err);
      setError(
        err instanceof Error ? err.message : "レシピの読み込みに失敗しました"
      );
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = () => {
    if (!recipe) return;
    setForm({
      title: recipe.title,
      sourceUrl: recipe.sourceUrl || "",
      ingredientsText: (recipe.ingredients || []).join("\n"),
      stepsText: (recipe.steps || []).join("\n"),
      selectedCategoryIds: recipe.categoryIds || [],
    });
    setEditingImageKeys(recipe.imageKeys || []);
    setIsEditing(true);
  };

  const handleCancelEdit = () => {
    if (!recipe) return;
    setForm({
      title: recipe.title,
      sourceUrl: recipe.sourceUrl || "",
      ingredientsText: (recipe.ingredients || []).join("\n"),
      stepsText: (recipe.steps || []).join("\n"),
      selectedCategoryIds: recipe.categoryIds || [],
    });
    setEditingImageKeys(recipe.imageKeys || []);
    setError(null);
    setIsEditing(false);
  };

  const handleDelete = async () => {
    if (!recipeId || deleting) return;
    if (!window.confirm("このレシピを削除しますか？")) return;

    try {
      setDeleting(true);
      setError(null);
      await deleteRecipe(recipeId);
      router.push("/recipes");
    } catch (err) {
      console.error("Failed to delete recipe:", err);
      setError(
        err instanceof Error ? err.message : "レシピの削除に失敗しました"
      );
    } finally {
      setDeleting(false);
    }
  };

  const handleSave = async () => {
    if (!recipeId || saving) return;
    const title = form.title.trim();
    const ingredients = form.ingredientsText
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    const steps = form.stepsText
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    const sourceUrl = form.sourceUrl.trim();

    if (!title || ingredients.length === 0 || steps.length === 0) {
      setError("タイトル、材料、手順は必須です");
      return;
    }

    try {
      setSaving(true);
      setError(null);

      const finalImageKeys = await imageManagerRef.current?.uploadNewImages();

      const validCategoryIds = form.selectedCategoryIds.filter((id) =>
        categories.some((c) => c.categoryId === id)
      );
      const updated = await updateRecipe(recipeId, {
        title,
        ingredients,
        steps,
        sourceUrl: sourceUrl || undefined,
        imageKeys: finalImageKeys,
        categoryIds: validCategoryIds,
      });
      setRecipe(updated);
      setForm({
        title: updated.title,
        sourceUrl: updated.sourceUrl || "",
        ingredientsText: (updated.ingredients || []).join("\n"),
        stepsText: (updated.steps || []).join("\n"),
        selectedCategoryIds: updated.categoryIds || [],
      });
      setIsEditing(false);

      const urls: Record<string, string> = {};
      for (const key of updated.imageKeys || []) {
        try {
          urls[key] = await getImageUrl(key);
        } catch (err) {
          console.error(`Failed to get image URL for ${key}:`, err);
        }
      }
      setImageUrls(urls);
    } catch (err) {
      console.error("Failed to update recipe:", err);
      setError(
        err instanceof Error ? err.message : "レシピの更新に失敗しました"
      );
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div>
        <MobileHeader title="レシピ" backHref="/recipes" />
        <div className="px-3 py-8 sm:px-4 text-center text-stone-600">読み込み中...</div>
      </div>
    );
  }

  if (error && !recipe) {
    return (
      <div>
        <MobileHeader title="レシピ" backHref="/recipes" />
        <div className="px-3 py-4 sm:px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm mb-4">
            {error || "レシピが見つかりません"}
          </div>
          <Link
            href="/recipes"
            className="text-amber-700 hover:text-amber-900 underline"
          >
            レシピ一覧に戻る
          </Link>
        </div>
      </div>
    );
  }

  if (!recipe) return null;

  return (
    <div>
      <MobileHeader
        title={isEditing ? "編集中" : recipe.title}
        backHref="/recipes"
        rightAction={
          isEditing ? (
            <>
              <button
                type="button"
                onClick={handleCancelEdit}
                disabled={saving}
                className="min-h-[44px] px-3 py-2 text-sm text-stone-600 disabled:opacity-50"
              >
                キャンセル
              </button>
              <button
                type="button"
                onClick={handleSave}
                disabled={saving}
                className="min-h-[44px] px-3 py-2 text-sm font-semibold text-amber-600 disabled:opacity-50"
              >
                {saving ? "保存中..." : "保存"}
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                onClick={handleEdit}
                className="min-h-[44px] min-w-[44px] flex items-center justify-center text-stone-600"
                aria-label="編集"
              >
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M12 20h9" />
                  <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
                </svg>
              </button>
              <button
                type="button"
                onClick={handleDelete}
                disabled={deleting}
                className="min-h-[44px] min-w-[44px] flex items-center justify-center text-red-500 disabled:opacity-50"
                aria-label="削除"
              >
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M3 6h18" />
                  <path d="M8 6V4h8v2" />
                  <path d="M19 6l-1 14H6L5 6" />
                </svg>
              </button>
            </>
          )
        }
      />

      {/* Hero image */}
      {!isEditing && recipe.imageKeys && recipe.imageKeys.length > 0 && (
        <div className="aspect-[4/3] sm:aspect-video bg-amber-50/60">
          {imageUrls[recipe.imageKeys[0]] ? (
            <img
              src={imageUrls[recipe.imageKeys[0]]}
              alt={recipe.title}
              className="w-full h-full object-cover"
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center text-stone-400">
              画像読み込み中...
            </div>
          )}
        </div>
      )}

      <div className="px-3 py-4 sm:px-4 lg:max-w-2xl lg:mx-auto">
        {error && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm">
            {error}
          </div>
        )}

        {/* Title */}
        {isEditing ? (
          <div className="mb-4">
            <label className="block text-sm font-semibold text-stone-700 mb-2">
              タイトル
            </label>
            <input
              type="text"
              value={form.title}
              onChange={(event) =>
                setForm((prev) => ({ ...prev, title: event.target.value }))
              }
              className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
            />
          </div>
        ) : (
          <div className="mb-3">
            <h1 className="text-xl sm:text-2xl lg:text-3xl font-bold text-stone-800">
              {recipe.title}
            </h1>
            {recipe.categoryIds && recipe.categoryIds.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1.5">
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
          </div>
        )}

        <p className="text-sm text-stone-500 mb-4">
          作成日: {new Date(recipe.createdAt).toLocaleString("ja-JP")}
        </p>

        {/* Image manager (edit mode) */}
        {isEditing && (
          <div className="mb-4">
            <RecipeImageManager
              ref={imageManagerRef}
              currentImageKeys={recipe.imageKeys || []}
              currentImageUrls={imageUrls}
              recipeId={recipeId}
              onImageKeysChange={setEditingImageKeys}
            />
          </div>
        )}

        {/* Source URL */}
        {isEditing ? (
          <div className="mb-4">
            <label className="block text-sm font-semibold text-stone-700 mb-2">
              参照元URL
            </label>
            <input
              type="url"
              value={form.sourceUrl}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  sourceUrl: event.target.value,
                }))
              }
              className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
            />
          </div>
        ) : (
          recipe.sourceUrl && (
            <div className="mb-4">
              <h2 className="text-sm font-semibold text-stone-700 mb-1">参照元</h2>
              <a
                href={recipe.sourceUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-amber-700 hover:text-amber-900 underline break-all text-sm"
              >
                {recipe.sourceUrl}
              </a>
            </div>
          )
        )}

        {/* Categories (edit mode) */}
        {isEditing && (
          <div className="mb-4">
            <label className="block text-sm font-semibold text-stone-700 mb-2">
              カテゴリ
            </label>
            {categories.length === 0 ? (
              <p className="text-sm text-stone-500">
                カテゴリがまだ登録されていません。
              </p>
            ) : (
              <div className="grid grid-cols-2 gap-2">
                {categories.map((category) => (
                  <label
                    key={category.categoryId}
                    className="flex items-center gap-2 rounded border border-amber-50 bg-amber-50/30 px-3 py-2 min-h-[44px]"
                  >
                    <input
                      type="checkbox"
                      checked={form.selectedCategoryIds.includes(
                        category.categoryId
                      )}
                      onChange={(event) => {
                        const checked = event.target.checked;
                        setForm((prev) => {
                          const current = new Set(prev.selectedCategoryIds);
                          if (checked) {
                            current.add(category.categoryId);
                          } else {
                            current.delete(category.categoryId);
                          }
                          return {
                            ...prev,
                            selectedCategoryIds: Array.from(current),
                          };
                        });
                      }}
                    />
                    <span className="inline-flex items-center gap-2 text-sm text-stone-700">
                      <span
                        className="inline-block h-3 w-3 rounded-full"
                        style={{ backgroundColor: category.color }}
                      />
                      {category.name}
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Ingredients */}
        <div className="mb-4">
          <h2 className="text-lg sm:text-xl font-semibold text-stone-800 mb-2">材料</h2>
          {isEditing ? (
            <textarea
              value={form.ingredientsText}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  ingredientsText: event.target.value,
                }))
              }
              rows={6}
              className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
            />
          ) : (
            <ul className="space-y-1.5">
              {recipe.ingredients.map((ingredient, index) => (
                <li key={index} className="flex items-start gap-2 text-stone-700">
                  <span className="text-amber-400 mt-1.5 text-xs">●</span>
                  <span>{ingredient}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Steps */}
        <div className="mb-4">
          <h2 className="text-lg sm:text-xl font-semibold text-stone-800 mb-2">手順</h2>
          {isEditing ? (
            <textarea
              value={form.stepsText}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  stepsText: event.target.value,
                }))
              }
              rows={8}
              className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
            />
          ) : (
            <ol className="space-y-3">
              {recipe.steps.map((step, index) => (
                <li key={index} className="flex items-start gap-3 text-stone-700">
                  <span className="flex items-center justify-center w-7 h-7 rounded-full bg-amber-100 text-amber-700 text-sm font-semibold shrink-0 mt-0.5">
                    {index + 1}
                  </span>
                  <span className="pt-1">{step}</span>
                </li>
              ))}
            </ol>
          )}
        </div>

        {/* Additional images */}
        {!isEditing && recipe.imageKeys && recipe.imageKeys.length > 1 && (
          <div>
            <h2 className="text-lg sm:text-xl font-semibold text-stone-800 mb-2">
              画像
            </h2>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
              {recipe.imageKeys.slice(1).map((imageKey, index) => (
                <div
                  key={index}
                  className="aspect-square bg-amber-100/60 rounded-lg overflow-hidden"
                >
                  {imageUrls[imageKey] ? (
                    <img
                      src={imageUrls[imageKey]}
                      alt={`${recipe.title} ${index + 2}`}
                      className="w-full h-full object-cover"
                    />
                  ) : (
                    <div className="w-full h-full flex items-center justify-center text-stone-400 text-sm">
                      読み込み中...
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

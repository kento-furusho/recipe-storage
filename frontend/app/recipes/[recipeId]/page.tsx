"use client";

import AuthGuard from "@/components/AuthGuard";
import {
  deleteRecipe,
  getCategories,
  getImageUrl,
  getRecipe,
  Recipe,
  Category,
  updateRecipe,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
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
  const { signOut } = useAuth();
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

      // 画像のpresigned URLを取得
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

  const handleDelete = async () => {
    if (!recipeId || deleting) {
      return;
    }
    if (!window.confirm("このレシピを削除しますか？")) {
      return;
    }

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
    if (!recipeId || saving) {
      return;
    }
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

      // 新規画像のアップロード
      const finalImageKeys = await imageManagerRef.current?.uploadNewImages();

      const updated = await updateRecipe(recipeId, {
        title,
        ingredients,
        steps,
        sourceUrl: sourceUrl || undefined,
        imageKeys: finalImageKeys,
        categoryIds: form.selectedCategoryIds,
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

      // 画像URLを再取得
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
      <div className="min-h-screen bg-[#F6F1EB] py-8">
        <div className="container mx-auto px-4">
          <div className="text-center">読み込み中...</div>
        </div>
      </div>
    );
  }

  if (error || !recipe) {
    return (
      <div className="min-h-screen bg-[#F6F1EB] py-8">
        <div className="container mx-auto px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg mb-4">
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

  return (
    <div className="min-h-screen bg-[#F6F1EB] py-8">
      <div className="container mx-auto px-4 max-w-4xl">
        <div className="mb-6 flex items-center justify-between gap-4">
          <Link
            href="/recipes"
            className="text-amber-700 hover:text-amber-900 underline"
          >
            ← レシピ一覧に戻る
          </Link>
          <div className="flex items-center gap-3">
            <Link
              href="/categories"
              className="text-amber-700 hover:text-amber-900 underline"
            >
              カテゴリ管理
            </Link>
            <button
              type="button"
              onClick={() => signOut()}
              className="px-3 py-1.5 rounded-lg border border-stone-300 text-stone-600 hover:bg-stone-100 transition-colors text-sm"
            >
              サインアウト
            </button>
          </div>
          <div className="flex items-center gap-2">
            {isEditing ? (
              <>
                <button
                  type="button"
                  onClick={() => {
                    if (!recipe) {
                      return;
                    }
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
                  }}
                  disabled={saving}
                  className="px-4 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  キャンセル
                </button>
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="px-4 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {saving ? "保存中..." : "保存"}
                </button>
              </>
            ) : (
              <>
                <button
                  type="button"
                  onClick={handleEdit}
                  className="px-4 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60"
                >
                  編集
                </button>
                <button
                  type="button"
                  onClick={handleDelete}
                  disabled={deleting}
                  className="px-4 py-2 rounded-lg border border-red-200 text-red-700 hover:bg-red-50 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {deleting ? "削除中..." : "削除"}
                </button>
              </>
            )}
          </div>
        </div>

        <div className="bg-white rounded-lg shadow-lg overflow-hidden">
          {/* 画像 */}
          {recipe.imageKeys && recipe.imageKeys.length > 0 && (
            <div className="aspect-video bg-amber-50/60">
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

          <div className="p-6">
            {/* タイトル */}
            {isEditing ? (
              <div className="mb-6">
                <label className="block text-sm font-semibold text-stone-700 mb-2">
                  タイトル
                </label>
                <input
                  type="text"
                  value={form.title}
                  onChange={(event) =>
                    setForm((prev) => ({ ...prev, title: event.target.value }))
                  }
                  className="w-full rounded-lg border border-amber-100 px-3 py-2 text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
                />
              </div>
            ) : (
              <div className="mb-4">
                <h1 className="text-3xl font-bold text-stone-800">
                  {recipe.title}
                </h1>
                {recipe.categoryIds && recipe.categoryIds.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-2">
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

            {/* 作成日 */}
            <p className="text-sm text-stone-500 mb-6">
              作成日: {new Date(recipe.createdAt).toLocaleString("ja-JP")}
            </p>

            {/* 画像管理コンポーネント（編集モード時のみ） */}
            {isEditing && (
              <div className="mb-6">
                <RecipeImageManager
                  ref={imageManagerRef}
                  currentImageKeys={recipe.imageKeys || []}
                  currentImageUrls={imageUrls}
                  recipeId={recipeId}
                  onImageKeysChange={setEditingImageKeys}
                />
              </div>
            )}

            {/* 参照元URL */}
            {isEditing ? (
              <div className="mb-6">
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
                  className="w-full rounded-lg border border-amber-100 px-3 py-2 text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
                />
              </div>
            ) : (
              recipe.sourceUrl && (
              <div className="mb-6">
                <h2 className="text-sm font-semibold text-stone-700 mb-2">
                  参照元
                </h2>
                <a
                  href={recipe.sourceUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-amber-700 hover:text-amber-900 underline break-all"
                >
                  {recipe.sourceUrl}
                </a>
              </div>
              )
            )}

            {/* カテゴリ */}
            {isEditing ? (
              <div className="mb-6">
                <label className="block text-sm font-semibold text-stone-700 mb-2">
                  カテゴリ
                </label>
                {categories.length === 0 ? (
                  <p className="text-sm text-stone-500">
                    カテゴリがまだ登録されていません。カテゴリ管理で追加してください。
                  </p>
                ) : (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                    {categories.map((category) => (
                      <label
                        key={category.categoryId}
                        className="flex items-center gap-2 rounded border border-amber-50 bg-amber-50/30 px-3 py-2"
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
            ) : null}

            {/* 材料 */}
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-stone-800 mb-3">材料</h2>
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
                  className="w-full rounded-lg border border-amber-100 px-3 py-2 text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
                />
              ) : (
                <ul className="list-disc list-inside space-y-1">
                  {recipe.ingredients.map((ingredient, index) => (
                    <li key={index} className="text-stone-700">
                      {ingredient}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {/* 手順 */}
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-stone-800 mb-3">手順</h2>
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
                  className="w-full rounded-lg border border-amber-100 px-3 py-2 text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
                />
              ) : (
                <ol className="list-decimal list-inside space-y-2">
                  {recipe.steps.map((step, index) => (
                    <li key={index} className="text-stone-700">
                      {step}
                    </li>
                  ))}
                </ol>
              )}
            </div>

            {/* 画像一覧 */}
            {recipe.imageKeys && recipe.imageKeys.length > 1 && (
              <div>
                <h2 className="text-xl font-semibold text-stone-800 mb-3">
                  画像
                </h2>
                <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
                  {recipe.imageKeys.slice(1).map((imageKey, index) => (
                    <div
                      key={index}
                      className="aspect-square bg-amber-100/60 rounded overflow-hidden"
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

            {/* 下部の編集・削除ボタン */}
            <div className="mt-6 pt-6 border-t border-amber-50 flex items-center justify-end gap-2">
              {isEditing ? (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      if (!recipe) {
                        return;
                      }
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
                    }}
                    disabled={saving}
                    className="px-4 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    キャンセル
                  </button>
                  <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className="px-4 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {saving ? "保存中..." : "保存"}
                  </button>
                </>
              ) : (
                <>
                  <button
                    type="button"
                    onClick={handleEdit}
                    className="px-4 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60"
                  >
                    編集
                  </button>
                  <button
                    type="button"
                    onClick={handleDelete}
                    disabled={deleting}
                    className="px-4 py-2 rounded-lg border border-red-200 text-red-700 hover:bg-red-50 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {deleting ? "削除中..." : "削除"}
                  </button>
                </>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

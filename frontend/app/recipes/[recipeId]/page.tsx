"use client";

import { deleteRecipe, getImageUrl, getRecipe, Recipe } from "@/lib/api";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

export default function RecipeDetailPage() {
  const params = useParams();
  const recipeId = params.recipeId as string;
  const router = useRouter();

  const [recipe, setRecipe] = useState<Recipe | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (recipeId) {
      loadRecipe();
    }
  }, [recipeId]);

  const loadRecipe = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getRecipe(recipeId);
      setRecipe(data);

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

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 py-8">
        <div className="container mx-auto px-4">
          <div className="text-center">読み込み中...</div>
        </div>
      </div>
    );
  }

  if (error || !recipe) {
    return (
      <div className="min-h-screen bg-gray-50 py-8">
        <div className="container mx-auto px-4">
          <div className="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded mb-4">
            {error || "レシピが見つかりません"}
          </div>
          <Link
            href="/recipes"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            レシピ一覧に戻る
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 py-8">
      <div className="container mx-auto px-4 max-w-4xl">
        <div className="mb-6 flex items-center justify-between gap-4">
          <Link
            href="/recipes"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            ← レシピ一覧に戻る
          </Link>
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="px-4 py-2 rounded-lg border border-red-200 text-red-700 hover:bg-red-50 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {deleting ? "削除中..." : "削除"}
          </button>
        </div>

        <div className="bg-white rounded-lg shadow-md overflow-hidden">
          {/* 画像 */}
          {recipe.imageKeys && recipe.imageKeys.length > 0 && (
            <div className="aspect-video bg-gray-200">
              {imageUrls[recipe.imageKeys[0]] ? (
                <img
                  src={imageUrls[recipe.imageKeys[0]]}
                  alt={recipe.title}
                  className="w-full h-full object-cover"
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center text-gray-400">
                  画像読み込み中...
                </div>
              )}
            </div>
          )}

          <div className="p-6">
            {/* タイトル */}
            <h1 className="text-3xl font-bold text-gray-900 mb-4">
              {recipe.title}
            </h1>

            {/* 作成日 */}
            <p className="text-sm text-gray-500 mb-6">
              作成日: {new Date(recipe.createdAt).toLocaleString("ja-JP")}
            </p>

            {/* 参照元URL */}
            {recipe.sourceUrl && (
              <div className="mb-6">
                <h2 className="text-sm font-semibold text-gray-700 mb-2">
                  参照元
                </h2>
                <a
                  href={recipe.sourceUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:text-blue-800 underline break-all"
                >
                  {recipe.sourceUrl}
                </a>
              </div>
            )}

            {/* 材料 */}
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-gray-900 mb-3">材料</h2>
              <ul className="list-disc list-inside space-y-1">
                {recipe.ingredients.map((ingredient, index) => (
                  <li key={index} className="text-gray-700">
                    {ingredient}
                  </li>
                ))}
              </ul>
            </div>

            {/* 手順 */}
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-gray-900 mb-3">手順</h2>
              <ol className="list-decimal list-inside space-y-2">
                {recipe.steps.map((step, index) => (
                  <li key={index} className="text-gray-700">
                    {step}
                  </li>
                ))}
              </ol>
            </div>

            {/* 画像一覧 */}
            {recipe.imageKeys && recipe.imageKeys.length > 1 && (
              <div>
                <h2 className="text-xl font-semibold text-gray-900 mb-3">
                  画像
                </h2>
                <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
                  {recipe.imageKeys.slice(1).map((imageKey, index) => (
                    <div
                      key={index}
                      className="aspect-square bg-gray-200 rounded overflow-hidden"
                    >
                      {imageUrls[imageKey] ? (
                        <img
                          src={imageUrls[imageKey]}
                          alt={`${recipe.title} ${index + 2}`}
                          className="w-full h-full object-cover"
                        />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-gray-400 text-sm">
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
      </div>
    </div>
  );
}

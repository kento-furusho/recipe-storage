"use client";

import AuthGuard from "@/components/AuthGuard";
import {
  createCategory,
  deleteCategory,
  getCategories,
  updateCategory,
  Category,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import Link from "next/link";
import { useEffect, useState } from "react";

export default function CategoriesPage() {
  return (
    <AuthGuard>
      <CategoriesContent />
    </AuthGuard>
  );
}

function CategoriesContent() {
  const { signOut } = useAuth();
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [form, setForm] = useState({ name: "", color: "#22c55e" });
  const [editForm, setEditForm] = useState({ name: "", color: "#22c55e" });

  useEffect(() => {
    loadCategories();
  }, []);

  const loadCategories = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getCategories();
      const sorted = [...data].sort((a, b) => a.name.localeCompare(b.name, "ja"));
      setCategories(sorted);
    } catch (err) {
      console.error("Failed to load categories:", err);
      setError(err instanceof Error ? err.message : "カテゴリの取得に失敗しました");
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!form.name.trim()) {
      setError("カテゴリ名を入力してください");
      return;
    }
    try {
      setCreating(true);
      setError(null);
      await createCategory({ name: form.name, color: form.color });
      setForm({ name: "", color: "#22c55e" });
      await loadCategories();
    } catch (err) {
      console.error("Failed to create category:", err);
      setError(err instanceof Error ? err.message : "カテゴリの作成に失敗しました");
    } finally {
      setCreating(false);
    }
  };

  const startEdit = (category: Category) => {
    setEditingId(category.categoryId);
    setEditForm({ name: category.name, color: category.color });
  };

  const handleUpdate = async (categoryId: string) => {
    if (!editForm.name.trim()) {
      setError("カテゴリ名を入力してください");
      return;
    }
    try {
      setSavingId(categoryId);
      setError(null);
      await updateCategory(categoryId, {
        name: editForm.name,
        color: editForm.color,
      });
      setEditingId(null);
      await loadCategories();
    } catch (err) {
      console.error("Failed to update category:", err);
      setError(err instanceof Error ? err.message : "カテゴリの更新に失敗しました");
    } finally {
      setSavingId(null);
    }
  };

  const handleDelete = async (categoryId: string) => {
    if (!window.confirm("このカテゴリを削除しますか？")) {
      return;
    }
    try {
      setError(null);
      await deleteCategory(categoryId);
      await loadCategories();
    } catch (err) {
      console.error("Failed to delete category:", err);
      setError(err instanceof Error ? err.message : "カテゴリの削除に失敗しました");
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

  return (
    <div className="min-h-screen bg-[#F6F1EB] py-8">
      <div className="container mx-auto px-4 max-w-4xl">
        <div className="mb-6 flex items-center justify-between gap-4">
          <Link href="/recipes" className="text-amber-700 hover:text-amber-900 underline">
            ← レシピ一覧に戻る
          </Link>
          <button
            type="button"
            onClick={() => signOut()}
            className="px-4 py-2 rounded-lg border border-stone-300 text-stone-600 hover:bg-stone-100 transition-colors text-sm"
          >
            サインアウト
          </button>
        </div>

        <div className="bg-white rounded-lg shadow-lg p-6 mb-8">
          <h1 className="text-2xl font-bold text-stone-800 mb-4">カテゴリ管理</h1>
          {error && (
            <div className="mb-4 bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg">
              {error}
            </div>
          )}
          <div className="flex flex-col sm:flex-row gap-3">
            <input
              type="text"
              value={form.name}
              onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
              placeholder="カテゴリ名"
              className="flex-1 rounded-lg border border-amber-100 px-3 py-2 text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
            />
            <input
              type="color"
              value={form.color}
              onChange={(event) => setForm((prev) => ({ ...prev, color: event.target.value }))}
              className="h-10 w-16 rounded border border-amber-100"
            />
            <button
              type="button"
              onClick={handleCreate}
              disabled={creating}
              className="px-4 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50 shadow-sm"
            >
              {creating ? "追加中..." : "追加"}
            </button>
          </div>
        </div>

        <div className="bg-white rounded-lg shadow-lg p-6">
          <h2 className="text-lg font-semibold text-stone-800 mb-4">カテゴリ一覧</h2>
          {categories.length === 0 ? (
            <p className="text-stone-500">まだカテゴリがありません</p>
          ) : (
            <div className="space-y-3">
              {categories.map((category) => (
                <div
                  key={category.categoryId}
                  className="flex flex-col sm:flex-row sm:items-center gap-3 border border-amber-50 rounded-lg p-3"
                >
                  {editingId === category.categoryId ? (
                    <>
                      <input
                        type="text"
                        value={editForm.name}
                        onChange={(event) =>
                          setEditForm((prev) => ({ ...prev, name: event.target.value }))
                        }
                        className="flex-1 rounded-lg border border-amber-100 px-3 py-2 text-stone-900"
                      />
                      <input
                        type="color"
                        value={editForm.color}
                        onChange={(event) =>
                          setEditForm((prev) => ({ ...prev, color: event.target.value }))
                        }
                        className="h-10 w-16 rounded border border-amber-100"
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => handleUpdate(category.categoryId)}
                          disabled={savingId === category.categoryId}
                          className="px-3 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50"
                        >
                          {savingId === category.categoryId ? "保存中..." : "保存"}
                        </button>
                        <button
                          type="button"
                          onClick={() => setEditingId(null)}
                          className="px-3 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60"
                        >
                          キャンセル
                        </button>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="flex items-center gap-2 flex-1">
                        <span
                          className="inline-block h-3 w-3 rounded-full"
                          style={{ backgroundColor: category.color }}
                        />
                        <span className="text-stone-800">{category.name}</span>
                      </div>
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => startEdit(category)}
                          className="px-3 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60"
                        >
                          編集
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDelete(category.categoryId)}
                          className="px-3 py-2 rounded-lg border border-red-200 text-red-700 hover:bg-red-50"
                        >
                          削除
                        </button>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

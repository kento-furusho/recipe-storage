"use client";

import AuthGuard from "@/components/AuthGuard";
import MobileHeader from "@/components/MobileHeader";
import {
  createCategory,
  deleteCategory,
  getCategories,
  updateCategory,
  Category,
} from "@/lib/api";
import { useEffect, useState } from "react";

export default function CategoriesPage() {
  return (
    <AuthGuard>
      <CategoriesContent />
    </AuthGuard>
  );
}

function CategoriesContent() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [showForm, setShowForm] = useState(false);
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
      setShowForm(false);
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
      <div>
        <MobileHeader title="カテゴリ" />
        <div className="px-3 py-8 sm:px-4 text-center text-stone-600">読み込み中...</div>
      </div>
    );
  }

  return (
    <div>
      <MobileHeader
        title="カテゴリ"
        rightAction={
          <button
            type="button"
            onClick={() => setShowForm(!showForm)}
            className="min-h-[44px] min-w-[44px] flex items-center justify-center text-amber-600"
            aria-label="追加"
          >
            {showForm ? (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-6 h-6">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            ) : (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-6 h-6">
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
              </svg>
            )}
          </button>
        }
      />

      <div className="px-3 py-4 sm:px-4 lg:max-w-2xl lg:mx-auto">
        {error && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm">
            {error}
          </div>
        )}

        {/* Create form (toggle) */}
        {showForm && (
          <div className="bg-white rounded-lg shadow-sm p-4 mb-4">
            <div className="space-y-3">
              <input
                type="text"
                value={form.name}
                onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                placeholder="カテゴリ名"
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
              />
              <div className="flex items-center gap-3">
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
                  className="flex-1 min-h-[44px] px-4 py-3 rounded-lg bg-amber-500 text-white text-base font-medium hover:bg-amber-600 disabled:opacity-50"
                >
                  {creating ? "追加中..." : "追加"}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Category list */}
        {categories.length === 0 ? (
          <p className="text-stone-500 text-center py-8">まだカテゴリがありません</p>
        ) : (
          <div className="bg-white rounded-lg shadow-sm divide-y divide-stone-100">
            {categories.map((category) => (
              <div key={category.categoryId} className="p-3">
                {editingId === category.categoryId ? (
                  <div className="space-y-3">
                    <input
                      type="text"
                      value={editForm.name}
                      onChange={(event) =>
                        setEditForm((prev) => ({ ...prev, name: event.target.value }))
                      }
                      className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900"
                    />
                    <div className="flex items-center gap-3">
                      <input
                        type="color"
                        value={editForm.color}
                        onChange={(event) =>
                          setEditForm((prev) => ({ ...prev, color: event.target.value }))
                        }
                        className="h-10 w-16 rounded border border-amber-100"
                      />
                      <button
                        type="button"
                        onClick={() => handleUpdate(category.categoryId)}
                        disabled={savingId === category.categoryId}
                        className="min-h-[44px] px-4 py-2 rounded-lg bg-amber-500 text-white hover:bg-amber-600 disabled:opacity-50"
                      >
                        {savingId === category.categoryId ? "保存中..." : "保存"}
                      </button>
                      <button
                        type="button"
                        onClick={() => setEditingId(null)}
                        className="min-h-[44px] px-4 py-2 rounded-lg border border-amber-100 text-stone-700 hover:bg-amber-50/60"
                      >
                        キャンセル
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-3">
                    <span
                      className="inline-block h-4 w-4 rounded-full shrink-0"
                      style={{ backgroundColor: category.color }}
                    />
                    <span className="text-stone-800 flex-1 truncate">{category.name}</span>
                    <button
                      type="button"
                      onClick={() => startEdit(category)}
                      className="min-h-[44px] min-w-[44px] flex items-center justify-center text-stone-500 hover:text-stone-700"
                      aria-label="編集"
                    >
                      <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M12 20h9" />
                        <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
                      </svg>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleDelete(category.categoryId)}
                      className="min-h-[44px] min-w-[44px] flex items-center justify-center text-red-500 hover:text-red-700"
                      aria-label="削除"
                    >
                      <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M3 6h18" />
                        <path d="M8 6V4h8v2" />
                        <path d="M19 6l-1 14H6L5 6" />
                      </svg>
                    </button>
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

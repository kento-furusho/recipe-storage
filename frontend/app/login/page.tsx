"use client";

import { useAuth } from "@/lib/auth-context";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

type Mode = "signIn" | "signUp" | "confirm";

export default function LoginPage() {
  const { user, loading, signIn, signUp, confirmSignUp } = useAuth();
  const router = useRouter();
  const [mode, setMode] = useState<Mode>("signIn");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmCode, setConfirmCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!loading && user) {
      router.replace("/");
    }
  }, [user, loading, router]);

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await signIn(email, password);
      router.replace("/");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "サインインに失敗しました"
      );
    } finally {
      setSubmitting(false);
    }
  };

  const handleSignUp = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await signUp(email, password);
      setMode("confirm");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "アカウント作成に失敗しました"
      );
    } finally {
      setSubmitting(false);
    }
  };

  const handleConfirm = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await confirmSignUp(email, confirmCode);
      setMode("signIn");
      setConfirmCode("");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "確認コードが正しくありません"
      );
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-stone-600">読み込み中...</div>
      </div>
    );
  }

  if (user) {
    return null;
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-4">
      <div className="w-full max-w-md bg-white rounded-lg shadow-sm p-6 sm:p-8">
        <h1 className="text-2xl font-bold text-stone-800 text-center mb-2">
          レシピ保存アプリ
        </h1>
        <p className="text-sm text-stone-500 text-center mb-6">
          {mode === "signIn" && "アカウントにサインイン"}
          {mode === "signUp" && "新しいアカウントを作成"}
          {mode === "confirm" && "メールに届いた確認コードを入力"}
        </p>

        {error && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg text-sm">
            {error}
          </div>
        )}

        {mode === "signIn" && (
          <form onSubmit={handleSignIn} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-stone-700 mb-1">
                メールアドレス
              </label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-stone-700 mb-1">
                パスワード
              </label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
              />
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="w-full py-3 rounded-lg bg-amber-500 text-white text-base font-semibold hover:bg-amber-600 disabled:opacity-50 transition-colors"
            >
              {submitting ? "サインイン中..." : "サインイン"}
            </button>
            <p className="text-sm text-center text-stone-500">
              アカウントをお持ちでない方は{" "}
              <button
                type="button"
                onClick={() => {
                  setMode("signUp");
                  setError(null);
                }}
                className="text-amber-700 hover:text-amber-900 underline"
              >
                新規登録
              </button>
            </p>
          </form>
        )}

        {mode === "signUp" && (
          <form onSubmit={handleSignUp} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-stone-700 mb-1">
                メールアドレス
              </label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-stone-700 mb-1">
                パスワード
              </label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
              />
              <p className="mt-1 text-xs text-stone-400">
                8文字以上、大文字・小文字・数字を含む
              </p>
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="w-full py-3 rounded-lg bg-amber-500 text-white text-base font-semibold hover:bg-amber-600 disabled:opacity-50 transition-colors"
            >
              {submitting ? "登録中..." : "アカウント作成"}
            </button>
            <p className="text-sm text-center text-stone-500">
              既にアカウントをお持ちの方は{" "}
              <button
                type="button"
                onClick={() => {
                  setMode("signIn");
                  setError(null);
                }}
                className="text-amber-700 hover:text-amber-900 underline"
              >
                サインイン
              </button>
            </p>
          </form>
        )}

        {mode === "confirm" && (
          <form onSubmit={handleConfirm} className="space-y-4">
            <p className="text-sm text-stone-600">
              <strong>{email}</strong> に確認コードを送信しました。
            </p>
            <div>
              <label className="block text-sm font-medium text-stone-700 mb-1">
                確認コード
              </label>
              <input
                type="text"
                value={confirmCode}
                onChange={(e) => setConfirmCode(e.target.value)}
                required
                className="w-full rounded-lg border border-amber-100 px-3 py-3 text-base text-stone-900 focus:outline-none focus:ring-2 focus:ring-amber-200"
                placeholder="123456"
              />
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="w-full py-3 rounded-lg bg-amber-500 text-white text-base font-semibold hover:bg-amber-600 disabled:opacity-50 transition-colors"
            >
              {submitting ? "確認中..." : "確認"}
            </button>
            <p className="text-sm text-center text-stone-500">
              <button
                type="button"
                onClick={() => {
                  setMode("signIn");
                  setError(null);
                }}
                className="text-amber-700 hover:text-amber-900 underline"
              >
                サインインに戻る
              </button>
            </p>
          </form>
        )}
      </div>
    </div>
  );
}

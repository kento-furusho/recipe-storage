"use client";

import AuthGuard from "@/components/AuthGuard";
import ImageUpload from "@/components/ImageUpload";
import { useAuth } from "@/lib/auth-context";
import Link from "next/link";

export default function Home() {
  return (
    <AuthGuard>
      <HomeContent />
    </AuthGuard>
  );
}

function HomeContent() {
  const { signOut } = useAuth();

  return (
    <div className="min-h-screen bg-[#F6F1EB] py-8">
      <main className="container mx-auto">
        <div className="mb-8 flex items-center justify-between">
          <div>
            <h1 className="text-4xl font-bold text-stone-800 mb-2">
              レシピ保存アプリ
            </h1>
            <p className="text-stone-600">
              料理サイトやYouTube、スクリーンショットからレシピを保存
            </p>
          </div>
          <div className="flex items-center gap-3">
            <Link
              href="/recipes"
              className="px-4 py-2 bg-amber-500 text-white rounded-lg hover:bg-amber-600 transition-colors shadow-sm"
            >
              レシピ一覧
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
        <ImageUpload />
      </main>
    </div>
  );
}

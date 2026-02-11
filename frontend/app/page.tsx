"use client";

import AuthGuard from "@/components/AuthGuard";
import ImageUpload from "@/components/ImageUpload";
import MobileHeader from "@/components/MobileHeader";

export default function Home() {
  return (
    <AuthGuard>
      <HomeContent />
    </AuthGuard>
  );
}

function HomeContent() {
  return (
    <div>
      <MobileHeader title="レシピを登録" />
      <main className="px-3 py-4 sm:px-4 lg:max-w-2xl lg:mx-auto">
        {/* Hero */}
        <div className="text-center mb-6">
          <div className="inline-flex items-center gap-1.5 bg-amber-50 text-amber-700 text-xs font-medium px-3 py-1.5 rounded-full mb-3">
            <svg viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5">
              <path d="M10 1a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 1zM5.05 3.05a.75.75 0 011.06 0l1.062 1.06A.75.75 0 116.11 5.173L5.05 4.11a.75.75 0 010-1.06zm9.9 0a.75.75 0 010 1.06l-1.06 1.062a.75.75 0 01-1.062-1.061l1.061-1.06a.75.75 0 011.06 0zM3 8a.75.75 0 01.75-.75h1.5a.75.75 0 010 1.5h-1.5A.75.75 0 013 8zm11 0a.75.75 0 01.75-.75h1.5a.75.75 0 010 1.5h-1.5A.75.75 0 0114 8zm-6.828 2.828a.75.75 0 010 1.061L6.11 12.95a.75.75 0 01-1.06-1.06l1.06-1.06a.75.75 0 011.06 0zm5.656 0a.75.75 0 011.06 0l1.06 1.06a.75.75 0 11-1.06 1.06l-1.06-1.06a.75.75 0 010-1.06zM10 14a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 14z" />
            </svg>
            AI自動読み取り
          </div>
          <h2 className="text-lg font-bold text-stone-800 mb-1">
            写真を送るだけでレシピを保存
          </h2>
          <p className="text-sm text-stone-500">
            料理の写真やスクリーンショットからAIがレシピを自動で読み取ります
          </p>
        </div>

        <ImageUpload />

        {/* Steps */}
        <div className="mt-8 grid grid-cols-3 gap-3 text-center">
          <div>
            <div className="w-9 h-9 rounded-full bg-amber-100 text-amber-600 flex items-center justify-center mx-auto mb-1.5 text-sm font-bold">1</div>
            <p className="text-xs text-stone-600 font-medium">写真を送る</p>
          </div>
          <div>
            <div className="w-9 h-9 rounded-full bg-amber-100 text-amber-600 flex items-center justify-center mx-auto mb-1.5 text-sm font-bold">2</div>
            <p className="text-xs text-stone-600 font-medium">AIが解析</p>
          </div>
          <div>
            <div className="w-9 h-9 rounded-full bg-amber-100 text-amber-600 flex items-center justify-center mx-auto mb-1.5 text-sm font-bold">3</div>
            <p className="text-xs text-stone-600 font-medium">レシピ保存</p>
          </div>
        </div>
      </main>
    </div>
  );
}

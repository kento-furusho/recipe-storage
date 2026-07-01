"use client";

import AuthGuard from "@/components/AuthGuard";
import ImageUpload from "@/components/ImageUpload";
import MobileHeader from "@/components/MobileHeader";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";

function extractUrl(text: string): string {
  const match = text.match(/https?:\/\/[^\s]+/);
  return match ? match[0] : "";
}

export default function Home() {
  return (
    <AuthGuard>
      <Suspense>
        <HomeContent />
      </Suspense>
    </AuthGuard>
  );
}

function HomeContent() {
  const searchParams = useSearchParams();
  const sharedUrl = searchParams.get("shared_url") || "";
  const sharedText = searchParams.get("shared_text") || "";
  const initialSourceUrl = sharedUrl || extractUrl(sharedText);

  return (
    <div>
      <MobileHeader title="レシピを追加" />
      <main className="px-[18px] pt-[22px] pb-4 lg:max-w-2xl lg:mx-auto">
        {/* セクション見出し */}
        <div className="mb-2">
          <h2 className="text-[19px] font-extrabold text-[#2B2723] mb-1">
            どこから保存しますか?
          </h2>
          <p className="text-[13.5px] leading-[1.5] text-[#8A8178]">
            選ぶだけ。AIが読み取ってレシピにします。
          </p>
        </div>

        <ImageUpload initialSourceUrl={initialSourceUrl} />
      </main>
    </div>
  );
}

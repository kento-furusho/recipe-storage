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
        <ImageUpload />
      </main>
    </div>
  );
}

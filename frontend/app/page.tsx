import ImageUpload from "@/components/ImageUpload";
import Link from "next/link";

export default function Home() {
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
          <Link
            href="/recipes"
            className="px-4 py-2 bg-amber-500 text-white rounded-lg hover:bg-amber-600 transition-colors shadow-sm"
          >
            レシピ一覧
          </Link>
        </div>
        <ImageUpload />
      </main>
    </div>
  );
}

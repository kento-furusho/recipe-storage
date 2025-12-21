import ImageUpload from "@/components/ImageUpload";
import Link from "next/link";

export default function Home() {
  return (
    <div className="min-h-screen bg-gray-50 py-8">
      <main className="container mx-auto">
        <div className="mb-8 flex items-center justify-between">
          <div>
            <h1 className="text-4xl font-bold text-gray-900 mb-2">
              レシピ保存アプリ
            </h1>
            <p className="text-gray-600">
              料理サイトやYouTube、スクリーンショットからレシピを保存
            </p>
          </div>
          <Link
            href="/recipes"
            className="px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors"
          >
            レシピ一覧
          </Link>
        </div>
        <ImageUpload />
      </main>
    </div>
  );
}

"use client";

import AuthGuard from "@/components/AuthGuard";
import RecipeDetail from "./recipe-detail";

export default function RecipeDetailPage() {
  return (
    <AuthGuard>
      <RecipeDetail />
    </AuthGuard>
  );
}

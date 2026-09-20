"use client";

import AuthGuard from "@/components/AuthGuard";
import RecipesList from "./recipes-list";

export default function RecipesPage() {
  return (
    <AuthGuard>
      <RecipesList />
    </AuthGuard>
  );
}

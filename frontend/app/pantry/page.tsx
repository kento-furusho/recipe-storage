"use client";

import AuthGuard from "@/components/AuthGuard";
import PantrySearch from "./pantry-search";

export default function PantryPage() {
  return (
    <AuthGuard>
      <PantrySearch />
    </AuthGuard>
  );
}

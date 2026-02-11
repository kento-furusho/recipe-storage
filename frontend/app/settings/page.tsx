"use client";

import AuthGuard from "@/components/AuthGuard";
import MobileHeader from "@/components/MobileHeader";
import { useAuth } from "@/lib/auth-context";

export default function SettingsPage() {
  return (
    <AuthGuard>
      <SettingsContent />
    </AuthGuard>
  );
}

function SettingsContent() {
  const { user, signOut } = useAuth();

  return (
    <div>
      <MobileHeader title="設定" />
      <div className="px-3 py-4 sm:px-4 lg:max-w-2xl lg:mx-auto">
        <div className="bg-white rounded-lg shadow-sm p-4">
          <h2 className="text-sm font-semibold text-stone-500 mb-3">アカウント</h2>
          {user && (
            <p className="text-stone-800 mb-4">{user.email}</p>
          )}
          <button
            type="button"
            onClick={() => signOut()}
            className="w-full sm:w-auto min-h-[44px] px-4 py-3 rounded-lg border border-red-200 text-red-700 hover:bg-red-50 transition-colors text-base font-medium"
          >
            サインアウト
          </button>
        </div>
      </div>
    </div>
  );
}

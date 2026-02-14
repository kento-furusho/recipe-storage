"use client";

import { useState } from "react";
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
  const { user, signOut, updatePassword, updateEmail, confirmEmailUpdate } = useAuth();

  // パスワード変更
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordMessage, setPasswordMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);
  const [passwordLoading, setPasswordLoading] = useState(false);

  // メールアドレス変更
  const [newEmail, setNewEmail] = useState("");
  const [emailCode, setEmailCode] = useState("");
  const [emailStep, setEmailStep] = useState<"input" | "confirm">("input");
  const [emailMessage, setEmailMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);
  const [emailLoading, setEmailLoading] = useState(false);

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordMessage(null);

    if (newPassword !== confirmPassword) {
      setPasswordMessage({ type: "error", text: "新しいパスワードが一致しません" });
      return;
    }

    setPasswordLoading(true);
    try {
      await updatePassword(oldPassword, newPassword);
      setPasswordMessage({ type: "success", text: "パスワードを変更しました" });
      setOldPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err) {
      setPasswordMessage({ type: "error", text: err instanceof Error ? err.message : "パスワードの変更に失敗しました" });
    } finally {
      setPasswordLoading(false);
    }
  };

  const handleEmailSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setEmailMessage(null);
    setEmailLoading(true);
    try {
      await updateEmail(newEmail);
      setEmailStep("confirm");
      setEmailMessage({ type: "success", text: "確認コードを新しいメールアドレスに送信しました" });
    } catch (err) {
      setEmailMessage({ type: "error", text: err instanceof Error ? err.message : "メールアドレスの変更に失敗しました" });
    } finally {
      setEmailLoading(false);
    }
  };

  const handleEmailConfirm = async (e: React.FormEvent) => {
    e.preventDefault();
    setEmailMessage(null);
    setEmailLoading(true);
    try {
      await confirmEmailUpdate(emailCode);
      setEmailMessage({ type: "success", text: "メールアドレスを変更しました" });
      setEmailStep("input");
      setNewEmail("");
      setEmailCode("");
    } catch (err) {
      setEmailMessage({ type: "error", text: err instanceof Error ? err.message : "確認コードが正しくありません" });
    } finally {
      setEmailLoading(false);
    }
  };

  return (
    <div>
      <MobileHeader title="設定" />
      <div className="px-3 py-4 sm:px-4 lg:max-w-2xl lg:mx-auto space-y-4">
        {/* アカウント情報 */}
        <div className="bg-white rounded-lg shadow-sm p-4">
          <h2 className="text-sm font-semibold text-stone-500 mb-3">アカウント情報</h2>
          {user && (
            <p className="text-stone-800">{user.email}</p>
          )}
        </div>

        {/* メールアドレス変更 */}
        <div className="bg-white rounded-lg shadow-sm p-4">
          <h2 className="text-sm font-semibold text-stone-500 mb-3">メールアドレス変更</h2>
          {emailMessage && (
            <p className={`text-sm mb-3 ${emailMessage.type === "success" ? "text-green-600" : "text-red-600"}`}>
              {emailMessage.text}
            </p>
          )}
          {emailStep === "input" ? (
            <form onSubmit={handleEmailSubmit} className="space-y-3">
              <input
                type="email"
                placeholder="新しいメールアドレス"
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                required
                className="w-full min-h-[44px] px-3 py-2 border border-stone-300 rounded-lg text-base focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent"
              />
              <button
                type="submit"
                disabled={emailLoading}
                className="w-full sm:w-auto min-h-[44px] px-4 py-3 rounded-lg bg-amber-600 text-white hover:bg-amber-700 transition-colors text-base font-medium disabled:opacity-50"
              >
                {emailLoading ? "送信中..." : "確認コードを送信"}
              </button>
            </form>
          ) : (
            <form onSubmit={handleEmailConfirm} className="space-y-3">
              <input
                type="text"
                placeholder="確認コード"
                value={emailCode}
                onChange={(e) => setEmailCode(e.target.value)}
                required
                className="w-full min-h-[44px] px-3 py-2 border border-stone-300 rounded-lg text-base focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent"
              />
              <div className="flex gap-2">
                <button
                  type="submit"
                  disabled={emailLoading}
                  className="min-h-[44px] px-4 py-3 rounded-lg bg-amber-600 text-white hover:bg-amber-700 transition-colors text-base font-medium disabled:opacity-50"
                >
                  {emailLoading ? "確認中..." : "確認"}
                </button>
                <button
                  type="button"
                  onClick={() => { setEmailStep("input"); setEmailMessage(null); setEmailCode(""); }}
                  className="min-h-[44px] px-4 py-3 rounded-lg border border-stone-300 text-stone-700 hover:bg-stone-50 transition-colors text-base font-medium"
                >
                  キャンセル
                </button>
              </div>
            </form>
          )}
        </div>

        {/* パスワード変更 */}
        <div className="bg-white rounded-lg shadow-sm p-4">
          <h2 className="text-sm font-semibold text-stone-500 mb-3">パスワード変更</h2>
          {passwordMessage && (
            <p className={`text-sm mb-3 ${passwordMessage.type === "success" ? "text-green-600" : "text-red-600"}`}>
              {passwordMessage.text}
            </p>
          )}
          <form onSubmit={handlePasswordChange} className="space-y-3">
            <input
              type="password"
              placeholder="現在のパスワード"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              required
              className="w-full min-h-[44px] px-3 py-2 border border-stone-300 rounded-lg text-base focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent"
            />
            <input
              type="password"
              placeholder="新しいパスワード"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              className="w-full min-h-[44px] px-3 py-2 border border-stone-300 rounded-lg text-base focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent"
            />
            <input
              type="password"
              placeholder="新しいパスワード（確認）"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              className="w-full min-h-[44px] px-3 py-2 border border-stone-300 rounded-lg text-base focus:outline-none focus:ring-2 focus:ring-amber-500 focus:border-transparent"
            />
            <button
              type="submit"
              disabled={passwordLoading}
              className="w-full sm:w-auto min-h-[44px] px-4 py-3 rounded-lg bg-amber-600 text-white hover:bg-amber-700 transition-colors text-base font-medium disabled:opacity-50"
            >
              {passwordLoading ? "変更中..." : "パスワードを変更"}
            </button>
          </form>
        </div>

        {/* サインアウト */}
        <div className="bg-white rounded-lg shadow-sm p-4">
          <h2 className="text-sm font-semibold text-stone-500 mb-3">サインアウト</h2>
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

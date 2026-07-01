"use client";

import { createJob, generateJobId, uploadImages } from "@/lib/api";
import { useCallback, useEffect, useRef, useState } from "react";

interface ImagePreview {
  file: File;
  preview: string;
}

interface ImageUploadProps {
  initialSourceUrl?: string;
}

type CardKey = "youtube" | "photo" | "site";

function isYouTubeUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    return parsed.hostname.includes("youtube.com") || parsed.hostname.includes("youtu.be");
  } catch {
    return false;
  }
}

export default function ImageUpload({ initialSourceUrl = "" }: ImageUploadProps) {
  const [images, setImages] = useState<ImagePreview[]>([]);
  const [sourceUrl, setSourceUrl] = useState<string>(initialSourceUrl);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState<string>("");
  const [openCard, setOpenCard] = useState<CardKey | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const autoSubmittedRef = useRef(false);

  const MAX_IMAGES = 3;

  // YouTube URLの場合、自動でジョブ作成
  useEffect(() => {
    if (!initialSourceUrl || autoSubmittedRef.current || !isYouTubeUrl(initialSourceUrl)) {
      return;
    }
    autoSubmittedRef.current = true;

    const autoSubmit = async () => {
      setIsUploading(true);
      setUploadStatus("YouTube動画からレシピを生成中...");
      try {
        await createJob([], initialSourceUrl);
        setUploadStatus("AIがYouTube動画からレシピを生成中です。完了すると「レシピ」一覧に追加されます。");
        setSourceUrl("");
      } catch (error) {
        console.error("Auto-submit error:", error);
        setUploadStatus(
          `自動送信に失敗しました。下のフォームから手動で送信してください。`
        );
        autoSubmittedRef.current = false;
      } finally {
        setIsUploading(false);
      }
    };
    autoSubmit();
  }, [initialSourceUrl]);

  const createPreview = (file: File): string => {
    return URL.createObjectURL(file);
  };

  const handleFiles = useCallback(
    (files: FileList | null) => {
      if (!files) return;

      const fileArray = Array.from(files);
      const imageFiles = fileArray.filter((file) =>
        file.type.startsWith("image/")
      );

      const remainingSlots = MAX_IMAGES - images.length;
      const filesToAdd = imageFiles.slice(0, remainingSlots);

      if (filesToAdd.length === 0) {
        setUploadStatus("画像ファイルを選択してください（最大3枚まで）");
        return;
      }

      if (imageFiles.length > remainingSlots) {
        setUploadStatus(`${remainingSlots}枚まで追加しました（最大3枚まで）`);
      } else {
        setUploadStatus("");
      }

      const newImages: ImagePreview[] = filesToAdd.map((file) => ({
        file,
        preview: createPreview(file),
      }));

      setImages((prev) => [...prev, ...newImages]);
    },
    [images.length]
  );

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      handleFiles(e.target.files);
    },
    [handleFiles]
  );

  const removeImage = useCallback((index: number) => {
    setImages((prev) => {
      const newImages = [...prev];
      URL.revokeObjectURL(newImages[index].preview);
      newImages.splice(index, 1);
      return newImages;
    });
    setUploadStatus("");
  }, []);

  const pasteFromClipboard = useCallback(async () => {
    try {
      const text = await navigator.clipboard.readText();
      const url = text.match(/https?:\/\/[^\s]+/)?.[0];
      if (url) setSourceUrl(url);
    } catch {}
  }, []);

  const handleUpload = useCallback(async () => {
    if (images.length === 0 && !sourceUrl.trim()) {
      setUploadStatus("画像またはURLを入力してください");
      return;
    }

    setIsUploading(true);
    setUploadStatus("AIに送信中...");

    try {
      let s3Keys: string[] = [];

      if (images.length > 0) {
        const jobId = generateJobId();
        const files = images.map((img) => img.file);
        s3Keys = await uploadImages(files, jobId);
      }

      const urlToSubmit = sourceUrl.trim() || undefined;
      await createJob(s3Keys, urlToSubmit);

      const successMessage =
        images.length > 0
          ? `AIがレシピを読み取り中です。完了すると「レシピ」一覧に追加されます。`
          : `AIがレシピを生成中です。完了すると「レシピ」一覧に追加されます。`;

      setUploadStatus(successMessage);
      images.forEach((img) => URL.revokeObjectURL(img.preview));
      setImages([]);
      setSourceUrl("");
      setOpenCard(null);
    } catch (error) {
      console.error("Upload error:", error);
      setUploadStatus(
        `アップロードエラー: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    } finally {
      setIsUploading(false);
    }
  }, [images, sourceUrl]);

  const toggleCard = useCallback((key: CardKey) => {
    setOpenCard((prev) => (prev === key ? null : key));
  }, []);

  return (
    <div className="flex flex-col gap-[14px]">
      {/* YouTube・動画 */}
      <EntryCard
        open={openCard === "youtube"}
        onToggle={() => toggleCard("youtube")}
        tileBg="#FDECEA"
        title="YouTube・動画"
        description="動画のURLを貼って手順を保存"
        icon={
          <svg viewBox="0 0 24 24" className="w-6 h-6" aria-hidden>
            <rect x="2" y="5" width="20" height="14" rx="4.5" fill="#E5342B" />
            <path d="M10 8.5l6 3.5-6 3.5V8.5z" fill="#fff" />
          </svg>
        }
      >
        <UrlPanel
          placeholder="https://youtube.com/watch?v=..."
          value={sourceUrl}
          onChange={setSourceUrl}
          onPaste={pasteFromClipboard}
          onSubmit={handleUpload}
          isUploading={isUploading}
          canSubmit={!!sourceUrl.trim()}
        />
      </EntryCard>

      {/* 写真・スクリーンショット */}
      <EntryCard
        open={openCard === "photo"}
        onToggle={() => toggleCard("photo")}
        tileBg="#FBEFDD"
        title="写真・スクリーンショット"
        description="カメラロールの画像から読み取り"
        icon={
          <svg viewBox="0 0 24 24" fill="none" stroke="#D98A2B" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round" className="w-6 h-6" aria-hidden>
            <path d="M3 8a2 2 0 0 1 2-2h1.5l1-1.5h5l1 1.5H20a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8z" />
            <circle cx="12" cy="12.5" r="3.2" />
          </svg>
        }
      >
        <div className="pt-3 flex flex-col gap-3">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            onChange={handleFileInput}
            className="hidden"
            disabled={images.length >= MAX_IMAGES}
          />

          {images.length > 0 && (
            <div className="grid grid-cols-3 gap-3">
              {images.map((image, index) => (
                <div key={index} className="relative group">
                  <div className="aspect-square rounded-[13px] overflow-hidden border border-[#EEE4D5] bg-[#FBEFDD]/40">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={image.preview}
                      alt={`Preview ${index + 1}`}
                      className="w-full h-full object-cover"
                    />
                  </div>
                  <button
                    type="button"
                    onClick={() => removeImage(index)}
                    className="absolute top-2 right-2 bg-red-500 text-white rounded-full p-1 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity"
                    aria-label="画像を削除"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
          )}

          {images.length < MAX_IMAGES && (
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="w-full min-h-[48px] rounded-[13px] border border-dashed border-[#E3D8C4] bg-[#FBEFDD]/50 text-[14px] font-medium text-[#B26B18] transition-colors hover:bg-[#FBEFDD]"
            >
              写真・スクショを選ぶ（{images.length}/{MAX_IMAGES}）
            </button>
          )}

          <PrimaryButton
            onClick={handleUpload}
            isUploading={isUploading}
            disabled={images.length === 0}
          />
        </div>
      </EntryCard>

      {/* レシピサイトのURL */}
      <EntryCard
        open={openCard === "site"}
        onToggle={() => toggleCard("site")}
        tileBg="#E9F0EF"
        title="レシピサイトのURL"
        description="ページのリンクを貼って保存"
        icon={
          <svg viewBox="0 0 24 24" fill="none" stroke="#4E7C77" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round" className="w-6 h-6" aria-hidden>
            <path d="M9.5 14.5l5-5" />
            <path d="M8 12l-2 2a3 3 0 1 0 4.2 4.2l2-2" />
            <path d="M16 12l2-2a3 3 0 1 0-4.2-4.2l-2 2" />
          </svg>
        }
      >
        <UrlPanel
          placeholder="https://example.com/recipe"
          value={sourceUrl}
          onChange={setSourceUrl}
          onPaste={pasteFromClipboard}
          onSubmit={handleUpload}
          isUploading={isUploading}
          canSubmit={!!sourceUrl.trim()}
        />
      </EntryCard>

      {/* 結果アウトカム帯 */}
      <div className="flex items-center gap-[10px] px-[15px] py-[13px] bg-[#F0EFE6] rounded-[14px] mt-1">
        <svg viewBox="0 0 24 24" fill="none" stroke="#7A9160" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[19px] h-[19px] shrink-0" aria-hidden>
          <path d="M20 6L9 17l-5-5" />
        </svg>
        <p className="text-[13px] leading-[1.5] font-medium text-[#5C6B4A]">
          読み取り後、<span className="text-[#3D4A31] font-semibold">「レシピ」一覧</span>に自動で追加されます
        </p>
      </div>

      {/* ステータス表示 */}
      {uploadStatus && (
        <div
          className={`
            p-3 rounded-lg text-sm
            ${
              uploadStatus.includes("読み取り中") || uploadStatus.includes("生成中")
                ? "bg-green-50 text-green-800"
                : uploadStatus.includes("エラー")
                ? "bg-red-50 text-red-800"
                : "bg-amber-50 text-amber-800"
            }
          `}
        >
          {uploadStatus}
        </div>
      )}
    </div>
  );
}

/* --- サブコンポーネント --- */

interface EntryCardProps {
  open: boolean;
  onToggle: () => void;
  tileBg: string;
  title: string;
  description: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}

function EntryCard({ open, onToggle, tileBg, title, description, icon, children }: EntryCardProps) {
  return (
    <div
      className={`bg-white border rounded-[18px] transition-shadow ${
        open ? "border-[#E3D8C4] shadow-[0_4px_12px_rgba(0,0,0,0.06)]" : "border-[#EEE4D5] shadow-[0_1px_2px_rgba(0,0,0,0.03)]"
      }`}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="w-full flex items-center gap-[14px] p-4 text-left active:scale-[0.99] transition-transform"
      >
        <span
          className="flex items-center justify-center w-[50px] h-[50px] rounded-[14px] shrink-0"
          style={{ backgroundColor: tileBg }}
        >
          {icon}
        </span>
        <span className="flex-1 min-w-0">
          <span className="block text-[15.5px] font-bold text-[#2B2723]">{title}</span>
          <span className="block text-[13px] text-[#8A8178] mt-0.5">{description}</span>
        </span>
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="#C9BEAD"
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
          className={`w-4 h-4 shrink-0 transition-transform ${open ? "rotate-90" : ""}`}
          aria-hidden
        >
          <polyline points="9 6 15 12 9 18" />
        </svg>
      </button>

      {open && <div className="px-4 pb-4">{children}</div>}
    </div>
  );
}

interface UrlPanelProps {
  placeholder: string;
  value: string;
  onChange: (v: string) => void;
  onPaste: () => void;
  onSubmit: () => void;
  isUploading: boolean;
  canSubmit: boolean;
}

function UrlPanel({ placeholder, value, onChange, onPaste, onSubmit, isUploading, canSubmit }: UrlPanelProps) {
  return (
    <div className="pt-3 flex flex-col gap-3">
      <div className="relative">
        <input
          type="url"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="w-full px-3 py-3 pr-12 text-base border border-[#E7DBC7] rounded-[13px] focus:ring-2 focus:ring-amber-200 focus:border-amber-200"
        />
        {typeof navigator !== "undefined" && navigator.clipboard && (
          <button
            type="button"
            onClick={onPaste}
            className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 text-[#C77B1F] hover:text-[#B26B18] transition-colors"
            aria-label="クリップボードから貼り付け"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
            </svg>
          </button>
        )}
      </div>
      <PrimaryButton onClick={onSubmit} isUploading={isUploading} disabled={!canSubmit} />
    </div>
  );
}

interface PrimaryButtonProps {
  onClick: () => void;
  isUploading: boolean;
  disabled: boolean;
}

function PrimaryButton({ onClick, isUploading, disabled }: PrimaryButtonProps) {
  const inactive = isUploading || disabled;
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={inactive}
      className={`h-[54px] w-full rounded-[15px] text-white text-[15px] font-extrabold transition-colors ${
        inactive
          ? "bg-stone-300 cursor-not-allowed"
          : "bg-[#E38A29] hover:bg-[#d47f22] shadow-[0_6px_16px_rgba(227,138,41,0.32)]"
      }`}
    >
      {isUploading ? "送信中…" : "AIでレシピを読み取る"}
    </button>
  );
}

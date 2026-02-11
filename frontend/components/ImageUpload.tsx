"use client";

import { createJob, generateJobId, uploadImages } from "@/lib/api";
import { useCallback, useRef, useState } from "react";

interface ImagePreview {
  file: File;
  preview: string;
}

export default function ImageUpload() {
  const [images, setImages] = useState<ImagePreview[]>([]);
  const [sourceUrl, setSourceUrl] = useState<string>("");
  const [isDragging, setIsDragging] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState<string>("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  const MAX_IMAGES = 3;

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

  const handleDrop = useCallback(
    (e: React.DragEvent<HTMLDivElement>) => {
      e.preventDefault();
      setIsDragging(false);
      handleFiles(e.dataTransfer.files);
    },
    [handleFiles]
  );

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

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

  const handleUpload = useCallback(async () => {
    if (images.length === 0 && !sourceUrl.trim()) {
      setUploadStatus("画像またはURLを入力してください");
      return;
    }

    setIsUploading(true);
    setUploadStatus("アップロード中...");

    try {
      let s3Keys: string[] = [];

      if (images.length > 0) {
        const jobId = generateJobId();
        const files = images.map((img) => img.file);
        s3Keys = await uploadImages(files, jobId);
      }

      const urlToSubmit = sourceUrl.trim() || undefined;
      const { jobId: createdJobId } = await createJob(s3Keys, urlToSubmit);

      const successMessage =
        images.length > 0
          ? `アップロード成功！${s3Keys.length}枚の画像をアップロードしました。レシピ生成を開始しました（ジョブID: ${createdJobId}）。`
          : `レシピ生成を開始しました（ジョブID: ${createdJobId}）。`;

      setUploadStatus(successMessage);
      setImages([]);
      setSourceUrl("");

      images.forEach((img) => URL.revokeObjectURL(img.preview));
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

  return (
    <div className="space-y-4">
      {/* URL input */}
      <div className="space-y-2">
        <label
          htmlFor="sourceUrl"
          className="block text-sm font-medium text-stone-700"
        >
          レシピURL（任意）
        </label>
        <input
          id="sourceUrl"
          type="url"
          value={sourceUrl}
          onChange={(e) => setSourceUrl(e.target.value)}
          placeholder="https://example.com/recipe"
          className="w-full px-3 py-3 text-base border border-amber-100 rounded-lg focus:ring-2 focus:ring-amber-200 focus:border-amber-200"
        />
        <p className="text-xs text-stone-500">
          レシピサイトのURLやYouTube動画URLを入力できます
        </p>
      </div>

      {/* Drag & drop area */}
      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        className={`
          border-2 border-dashed rounded-lg p-4 sm:p-8 text-center transition-colors
          ${
            isDragging
              ? "border-amber-400 bg-amber-50/60"
              : "border-amber-100 bg-amber-50/40 hover:border-amber-200"
          }
          ${
            images.length >= MAX_IMAGES
              ? "opacity-50 cursor-not-allowed"
              : "cursor-pointer"
          }
        `}
        onClick={() => {
          if (images.length < MAX_IMAGES && fileInputRef.current) {
            fileInputRef.current.click();
          }
        }}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          onChange={handleFileInput}
          className="hidden"
          disabled={images.length >= MAX_IMAGES}
        />
        <div className="space-y-2">
          <svg
            className="mx-auto h-10 w-10 text-stone-400"
            stroke="currentColor"
            fill="none"
            viewBox="0 0 48 48"
          >
            <path
              d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <p className="text-sm text-stone-600">
            {images.length >= MAX_IMAGES
              ? "最大3枚までアップロードできます"
              : "画像をドラッグ&ドロップするか、クリックして選択"}
          </p>
          <p className="text-xs text-stone-500">
            {images.length}/{MAX_IMAGES} 枚
          </p>
        </div>
      </div>

      {/* Image previews */}
      {images.length > 0 && (
        <div className="grid grid-cols-3 gap-3">
          {images.map((image, index) => (
            <div key={index} className="relative group">
              <div className="aspect-square rounded-lg overflow-hidden border border-amber-50 bg-amber-50/40">
                <img
                  src={image.preview}
                  alt={`Preview ${index + 1}`}
                  className="w-full h-full object-cover"
                />
              </div>
              <button
                onClick={() => removeImage(index)}
                className="absolute top-2 right-2 bg-red-500 text-white rounded-full p-1 opacity-100 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity"
                aria-label="画像を削除"
              >
                <svg
                  className="w-4 h-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Upload button */}
      {(images.length > 0 || sourceUrl.trim()) && (
        <button
          onClick={handleUpload}
          disabled={isUploading}
          className={`
            w-full sm:w-auto px-6 py-3 rounded-lg font-medium text-base transition-colors min-h-[44px]
            ${
              isUploading
                ? "bg-stone-300 cursor-not-allowed"
                : "bg-amber-500 hover:bg-amber-600"
            }
            text-white
          `}
        >
          {isUploading ? "アップロード中..." : "レシピを登録"}
        </button>
      )}

      {/* Status message */}
      {uploadStatus && (
        <div
          className={`
            p-3 rounded-lg text-sm
            ${
              uploadStatus.includes("成功") || uploadStatus.includes("追加")
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

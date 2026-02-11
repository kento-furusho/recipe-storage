"use client";

import { getUploadURL, uploadImageToS3 } from "@/lib/api";
import { useCallback, useImperativeHandle, useRef, useState, forwardRef, useEffect } from "react";

interface ImagePreview {
  file: File;
  preview: string;
}

interface RecipeImageManagerProps {
  currentImageKeys: string[];
  currentImageUrls: Record<string, string>;
  recipeId: string;
  onImageKeysChange: (newImageKeys: string[]) => void;
  disabled?: boolean;
}

export interface RecipeImageManagerRef {
  uploadNewImages: () => Promise<string[]>;
}

const RecipeImageManager = forwardRef<RecipeImageManagerRef, RecipeImageManagerProps>(
  ({ currentImageKeys, currentImageUrls, recipeId, onImageKeysChange, disabled }, ref) => {
    const [imageKeys, setImageKeys] = useState<string[]>(currentImageKeys);
    const [newImages, setNewImages] = useState<ImagePreview[]>([]);
    const [isDragging, setIsDragging] = useState(false);
    const [isUploading, setIsUploading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);

    const MAX_IMAGES = 3;

    // 親から渡されたcurrentImageKeysが変更されたら同期
    useEffect(() => {
      setImageKeys(currentImageKeys);
    }, [currentImageKeys]);

    const createPreview = (file: File): string => {
      return URL.createObjectURL(file);
    };

    // 合計画像数
    const totalImages = imageKeys.length + newImages.length;
    const remainingSlots = MAX_IMAGES - totalImages;

    const handleFiles = useCallback(
      (files: FileList | null) => {
        if (!files || disabled) return;

        const fileArray = Array.from(files);
        const imageFiles = fileArray.filter((file) =>
          file.type.startsWith("image/")
        );

        if (imageFiles.length === 0) {
          setError("画像ファイルを選択してください");
          return;
        }

        const filesToAdd = imageFiles.slice(0, remainingSlots);

        if (imageFiles.length > remainingSlots) {
          setError(`${remainingSlots}枚まで追加できます（最大3枚まで）`);
        } else {
          setError(null);
        }

        const newImagePreviews: ImagePreview[] = filesToAdd.map((file) => ({
          file,
          preview: createPreview(file),
        }));

        setNewImages((prev) => [...prev, ...newImagePreviews]);
      },
      [remainingSlots, disabled]
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

    // 既存画像を削除
    const removeExistingImage = useCallback(
      (index: number) => {
        if (disabled) return;
        const newKeys = [...imageKeys];
        newKeys.splice(index, 1);
        setImageKeys(newKeys);
        onImageKeysChange(newKeys);
        setError(null);
      },
      [imageKeys, onImageKeysChange, disabled]
    );

    // 新規画像を削除
    const removeNewImage = useCallback(
      (index: number) => {
        if (disabled) return;
        setNewImages((prev) => {
          const newList = [...prev];
          URL.revokeObjectURL(newList[index].preview);
          newList.splice(index, 1);
          return newList;
        });
        setError(null);
      },
      [disabled]
    );

    // 新規画像をアップロードして最終的なimageKeys配列を返す
    const uploadNewImages = useCallback(async (): Promise<string[]> => {
      if (newImages.length === 0) {
        // 新規画像がない場合は既存のimageKeysをそのまま返す
        return imageKeys;
      }

      setIsUploading(true);
      setError(null);

      try {
        // 既存の最大インデックスを計算
        const existingIndices = imageKeys.map((key) => {
          const match = key.match(/\/(\d+)\.jpg$/);
          return match ? parseInt(match[1], 10) : -1;
        });
        const maxExistingIndex = existingIndices.length > 0
          ? Math.max(...existingIndices)
          : -1;

        // 新規画像を既存のインデックスと衝突しないようにアップロード
        const uploadedKeys: string[] = [];

        for (let i = 0; i < newImages.length; i++) {
          const file = newImages[i].file;
          const newIndex = maxExistingIndex + 1 + i;

          try {
            // presigned URLを取得（recipeIdをjobIdとして使用、新しいインデックスを指定）
            const { uploadUrl, s3Key } = await getUploadURL(
              recipeId,
              newIndex,
              file.type
            );

            // S3に直接アップロード
            await uploadImageToS3(uploadUrl, file);

            uploadedKeys.push(s3Key);
          } catch (error) {
            console.error(`[ERROR] Upload failed for image ${i + 1}:`, error);
            throw new Error(
              `画像 ${i + 1}/${newImages.length} のアップロードに失敗: ${
                error instanceof Error ? error.message : String(error)
              }`
            );
          }
        }

        // 最終的なimageKeys配列（既存 + 新規）
        const finalImageKeys = [...imageKeys, ...uploadedKeys];

        // 新規画像のプレビューをクリーンアップ
        newImages.forEach((img) => URL.revokeObjectURL(img.preview));
        setNewImages([]);

        return finalImageKeys;
      } catch (err) {
        console.error("Failed to upload new images:", err);
        const errorMessage = err instanceof Error ? err.message : "画像のアップロードに失敗しました";
        setError(errorMessage);
        throw err;
      } finally {
        setIsUploading(false);
      }
    }, [newImages, imageKeys, recipeId]);

    // 親コンポーネントから呼び出せるようにrefを公開
    useImperativeHandle(ref, () => ({
      uploadNewImages,
    }));

    // クリーンアップ
    useEffect(() => {
      return () => {
        newImages.forEach((img) => URL.revokeObjectURL(img.preview));
      };
    }, [newImages]);

    return (
      <div className="space-y-4">
        <label className="block text-sm font-medium text-stone-700">
          画像 ({totalImages}/{MAX_IMAGES}枚)
        </label>

        {/* 既存画像と新規画像のプレビュー */}
        {(imageKeys.length > 0 || newImages.length > 0) && (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            {/* 既存画像 */}
            {imageKeys.map((key, index) => (
              <div key={`existing-${key}`} className="relative group">
                <div className="aspect-square rounded-lg overflow-hidden border border-amber-50 bg-amber-50/40">
                  <img
                    src={currentImageUrls[key] || ""}
                    alt={`既存画像 ${index + 1}`}
                    className="w-full h-full object-cover"
                  />
                </div>
                {!disabled && (
                  <button
                    type="button"
                    onClick={() => removeExistingImage(index)}
                    className="absolute top-2 right-2 bg-red-500 text-white rounded-full p-1 opacity-0 group-hover:opacity-100 transition-opacity hover:bg-red-600"
                    aria-label="画像を削除"
                  >
                    <svg
                      className="w-5 h-5"
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
                )}
              </div>
            ))}

            {/* 新規画像 */}
            {newImages.map((image, index) => (
              <div key={`new-${index}`} className="relative group">
                <div className="aspect-square rounded-lg overflow-hidden border border-amber-100 bg-amber-50/40">
                  <img
                    src={image.preview}
                    alt={`新規画像 ${index + 1}`}
                    className="w-full h-full object-cover"
                  />
                </div>
                <div className="absolute top-2 left-2 bg-amber-400 text-white text-xs px-2 py-1 rounded">
                  新規
                </div>
                {!disabled && (
                  <button
                    type="button"
                    onClick={() => removeNewImage(index)}
                    className="absolute top-2 right-2 bg-red-500 text-white rounded-full p-1 opacity-0 group-hover:opacity-100 transition-opacity hover:bg-red-600"
                    aria-label="画像を削除"
                  >
                    <svg
                      className="w-5 h-5"
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
                )}
              </div>
            ))}
          </div>
        )}

        {/* ドラッグ&ドロップエリア */}
        {!disabled && totalImages < MAX_IMAGES && (
          <div
            onDrop={handleDrop}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            className={`
              border-2 border-dashed rounded-lg p-6 text-center transition-colors cursor-pointer
              ${
                isDragging
                  ? "border-amber-400 bg-amber-50/60"
                  : "border-amber-100 bg-amber-50/40 hover:border-amber-200"
              }
            `}
            onClick={() => {
              if (fileInputRef.current) {
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
                画像をドラッグ&ドロップするか、クリックして選択
              </p>
              <p className="text-xs text-stone-500">
                残り{remainingSlots}枚追加できます
              </p>
            </div>
          </div>
        )}

        {/* エラーメッセージ */}
        {error && (
          <div className="p-3 rounded-lg text-sm bg-red-50 text-red-800">
            {error}
          </div>
        )}

        {/* アップロード中の表示 */}
        {isUploading && (
          <div className="p-3 rounded-lg text-sm bg-amber-50/60 text-amber-800">
            画像をアップロード中...
          </div>
        )}
      </div>
    );
  }
);

RecipeImageManager.displayName = "RecipeImageManager";

export default RecipeImageManager;

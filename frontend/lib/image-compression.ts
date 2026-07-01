/**
 * アップロード前にブラウザ側で画像を圧縮する。
 *
 * - 長辺を MAX_DIMENSION に収まるよう縮小（拡大はしない）
 * - JPEG で再エンコード（バックエンドのS3キーは常に .jpg 固定・Bedrockへは
 *   image/jpeg として渡されるため、JPEG化は整合性の面でも都合が良い）
 * - EXIF の向き情報を反映（iPhone等の写真が回転して保存されるのを防ぐ）
 * - 何らかの理由で圧縮に失敗した場合は元ファイルをそのまま返す（アップロードを止めない）
 */

const MAX_DIMENSION = 1600; // 長辺の最大ピクセル
const JPEG_QUALITY = 0.8;

export interface CompressOptions {
  maxDimension?: number;
  quality?: number;
}

export async function compressImage(
  file: File,
  options: CompressOptions = {}
): Promise<File> {
  // 画像以外・ブラウザ外（SSR）では何もしない
  if (typeof document === "undefined" || !file.type.startsWith("image/")) {
    return file;
  }

  const maxDimension = options.maxDimension ?? MAX_DIMENSION;
  const quality = options.quality ?? JPEG_QUALITY;

  try {
    const bitmap = await loadBitmap(file);

    const scale = Math.min(1, maxDimension / Math.max(bitmap.width, bitmap.height));
    const targetWidth = Math.round(bitmap.width * scale);
    const targetHeight = Math.round(bitmap.height * scale);

    const canvas = document.createElement("canvas");
    canvas.width = targetWidth;
    canvas.height = targetHeight;
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      closeBitmap(bitmap);
      return file;
    }
    ctx.drawImage(bitmap, 0, 0, targetWidth, targetHeight);
    closeBitmap(bitmap);

    const blob = await new Promise<Blob | null>((resolve) => {
      canvas.toBlob((b) => resolve(b), "image/jpeg", quality);
    });

    // 圧縮できなかった / かえって大きくなった場合は元ファイルを使う
    if (!blob || blob.size >= file.size) {
      return file;
    }

    const newName = file.name.replace(/\.[^./\\]+$/, "") + ".jpg";
    return new File([blob], newName, {
      type: "image/jpeg",
      lastModified: Date.now(),
    });
  } catch {
    // デコード不可（HEIC等）や予期しないエラーは元ファイルにフォールバック
    return file;
  }
}

/**
 * EXIF の向きを反映して画像を読み込む。createImageBitmap が使えない環境では
 * HTMLImageElement にフォールバックする。
 */
async function loadBitmap(
  file: File
): Promise<ImageBitmap | HTMLImageElement> {
  if (typeof createImageBitmap === "function") {
    try {
      return await createImageBitmap(file, { imageOrientation: "from-image" });
    } catch {
      // 一部フォーマットで失敗しうるので下のフォールバックへ
    }
  }

  return await new Promise<HTMLImageElement>((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve(img);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("image decode failed"));
    };
    img.src = url;
  });
}

function closeBitmap(bitmap: ImageBitmap | HTMLImageElement): void {
  if (typeof ImageBitmap !== "undefined" && bitmap instanceof ImageBitmap) {
    bitmap.close();
  }
}

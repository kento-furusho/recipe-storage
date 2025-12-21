const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export interface UploadURLRequest {
  jobId: string;
  index: number; // 0..2
  contentType: string; // image/jpeg, image/png
}

export interface UploadURLResponse {
  uploadUrl: string;
  s3Key: string;
}

/**
 * presigned URLを取得する
 */
export async function getUploadURL(
  jobId: string,
  index: number,
  contentType: string
): Promise<UploadURLResponse> {
  const response = await fetch(`${API_BASE_URL}/upload-url`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      jobId,
      index,
      contentType,
    } as UploadURLRequest),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get upload URL: ${errorText}`);
  }

  return response.json();
}

/**
 * 画像をS3に直接アップロードする
 */
export async function uploadImageToS3(
  uploadUrl: string,
  file: File
): Promise<void> {
  try {
    // presigned URLの検証
    if (!uploadUrl || !uploadUrl.startsWith("http")) {
      throw new Error(`無効なpresigned URL: ${uploadUrl}`);
    }

    const response = await fetch(uploadUrl, {
      method: "PUT",
      headers: {
        "Content-Type": file.type,
      },
      body: file,
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => response.statusText);
      throw new Error(`S3アップロード失敗 (${response.status}): ${errorText}`);
    }
  } catch (error) {
    if (
      error instanceof TypeError &&
      error.message.includes("Failed to fetch")
    ) {
      // ネットワークエラーまたはCORSエラー
      throw new Error(
        `CORSエラーまたはネットワークエラー: 以下を確認してください\n` +
          `1. S3バケットのCORS設定が正しく適用されているか\n` +
          `2. ブラウザのキャッシュをクリアして再試行\n` +
          `3. バックエンドが起動しているか (http://localhost:8080)\n` +
          `詳細: ${error.message}`
      );
    }
    if (error instanceof Error) {
      throw error;
    }
    throw new Error(`予期しないエラー: ${String(error)}`);
  }
}

/**
 * 複数の画像をアップロードする
 */
export async function uploadImages(
  files: File[],
  jobId: string
): Promise<string[]> {
  const s3Keys: string[] = [];

  for (let i = 0; i < files.length; i++) {
    const file = files[i];

    try {
      // presigned URLを取得
      const { uploadUrl, s3Key } = await getUploadURL(jobId, i, file.type);

      // S3に直接アップロード
      await uploadImageToS3(uploadUrl, file);

      s3Keys.push(s3Key);
    } catch (error) {
      console.error(`[ERROR] Upload failed for image ${i + 1}:`, error);
      throw new Error(
        `画像 ${i + 1}/${files.length} のアップロードに失敗: ${
          error instanceof Error ? error.message : String(error)
        }`
      );
    }
  }

  return s3Keys;
}

/**
 * UUIDを生成する（簡易版）
 */
export function generateJobId(): string {
  return `${Date.now()}-${Math.random().toString(36).substring(2, 15)}`;
}

export interface CreateJobRequest {
  imageKeys: string[];
  sourceUrl?: string;
}

export interface CreateJobResponse {
  jobId: string;
}

export interface Job {
  jobId: string;
  userId: string;
  status: "PENDING" | "PROCESSING" | "SUCCEEDED" | "FAILED";
  imageKeys: string[];
  sourceUrl?: string | null;
  recipeId?: string | null;
  createdAt: string;
  updatedAt: string;
}

/**
 * レシピ生成ジョブを作成する
 */
export async function createJob(
  imageKeys: string[] = [],
  sourceUrl?: string
): Promise<CreateJobResponse> {
  const body: CreateJobRequest = {
    imageKeys,
  };
  if (sourceUrl) {
    body.sourceUrl = sourceUrl;
  }

  const response = await fetch(`${API_BASE_URL}/jobs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to create job: ${errorText}`);
  }

  return response.json();
}

/**
 * ジョブの状態を取得する
 */
export async function getJob(jobId: string): Promise<Job> {
  const response = await fetch(`${API_BASE_URL}/jobs/${jobId}`);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get job: ${errorText}`);
  }

  return response.json();
}

export interface Recipe {
  recipeId: string;
  userId: string;
  title: string;
  ingredients: string[];
  steps: string[];
  imageKeys: string[];
  sourceUrl?: string | null;
  createdAt: string;
}

/**
 * レシピ一覧を取得する
 */
export async function getRecipes(): Promise<Recipe[]> {
  const response = await fetch(`${API_BASE_URL}/recipes`);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get recipes: ${errorText}`);
  }

  return response.json();
}

/**
 * レシピ詳細を取得する
 */
export async function getRecipe(recipeId: string): Promise<Recipe> {
  const response = await fetch(`${API_BASE_URL}/recipes/${recipeId}`);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get recipe: ${errorText}`);
  }

  return response.json();
}

/**
 * レシピを削除する
 */
export async function deleteRecipe(recipeId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/recipes/${recipeId}`, {
    method: "DELETE",
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to delete recipe: ${errorText}`);
  }
}

/**
 * S3画像のpresigned URLを取得する
 */
export async function getImageUrl(s3Key: string): Promise<string> {
  const encodedKey = encodeURIComponent(s3Key);
  const response = await fetch(`${API_BASE_URL}/image-url?key=${encodedKey}`);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get image URL: ${errorText}`);
  }

  const data = await response.json();
  return data.url;
}

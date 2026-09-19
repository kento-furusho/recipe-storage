import { fetchAuthSession } from "aws-amplify/auth";
import { compressImage } from "./image-compression";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/**
 * 認証付きfetch: IDトークンをAuthorizationヘッダーに付与する
 */
async function authFetch(
  input: string,
  init?: RequestInit
): Promise<Response> {
  const headers = new Headers(init?.headers);
  try {
    const session = await fetchAuthSession();
    const token = session.tokens?.idToken?.toString();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
  } catch {
    // トークン取得失敗時はヘッダーなしで送信（ローカル開発用）
  }
  return fetch(input, { ...init, headers });
}

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
  const response = await authFetch(`${API_BASE_URL}/upload-url`, {
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
    // アップロード前にブラウザ側で圧縮（縮小＋JPEG化）して保存サイズを削減
    const file = await compressImage(files[i]);

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
  targetRecipeId?: string;
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
  sourceUrl?: string,
  targetRecipeId?: string
): Promise<CreateJobResponse> {
  const body: CreateJobRequest = {
    imageKeys,
  };
  if (sourceUrl) {
    body.sourceUrl = sourceUrl;
  }
  if (targetRecipeId) {
    body.targetRecipeId = targetRecipeId;
  }

  const response = await authFetch(`${API_BASE_URL}/jobs`, {
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
  const response = await authFetch(`${API_BASE_URL}/jobs/${jobId}`);

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
  categoryIds?: string[];
  sourceUrl?: string | null;
  createdAt: string;
}

export interface UpdateRecipeRequest {
  title: string;
  ingredients: string[];
  steps: string[];
  sourceUrl?: string;
  imageKeys?: string[];
  categoryIds?: string[];
}

export interface Category {
  categoryId: string;
  userId: string;
  name: string;
  color: string;
  createdAt: string;
  updatedAt: string;
}

export interface RecipePage {
  items: Recipe[];
  nextCursor: string | null;
}

/**
 * レシピ一覧を新しい順に1ページ分取得する。nextCursor があれば続きがある
 */
export async function getRecipesPage(options: {
  limit?: number;
  cursor?: string | null;
  categoryId?: string;
} = {}): Promise<RecipePage> {
  const params = new URLSearchParams({ limit: String(options.limit ?? 10) });
  if (options.cursor) params.set("cursor", options.cursor);
  if (options.categoryId) params.set("categoryId", options.categoryId);

  const response = await authFetch(`${API_BASE_URL}/recipes?${params}`);

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
  const response = await authFetch(`${API_BASE_URL}/recipes/${recipeId}`);

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
  const response = await authFetch(`${API_BASE_URL}/recipes/${recipeId}`, {
    method: "DELETE",
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to delete recipe: ${errorText}`);
  }
}

/**
 * レシピを更新する
 */
export async function updateRecipe(
  recipeId: string,
  body: UpdateRecipeRequest
): Promise<Recipe> {
  const response = await authFetch(`${API_BASE_URL}/recipes/${recipeId}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to update recipe: ${errorText}`);
  }

  return response.json();
}

export async function getCategories(): Promise<Category[]> {
  const response = await authFetch(`${API_BASE_URL}/categories`);
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get categories: ${errorText}`);
  }
  return response.json();
}

export async function createCategory(
  body: Pick<Category, "name" | "color">
): Promise<Category> {
  const response = await authFetch(`${API_BASE_URL}/categories`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to create category: ${errorText}`);
  }
  return response.json();
}

export async function updateCategory(
  categoryId: string,
  body: Pick<Category, "name" | "color">
): Promise<Category> {
  const response = await authFetch(
    `${API_BASE_URL}/categories/${categoryId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    }
  );
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to update category: ${errorText}`);
  }
  return response.json();
}

export async function deleteCategory(categoryId: string): Promise<void> {
  const response = await authFetch(
    `${API_BASE_URL}/categories/${categoryId}`,
    {
      method: "DELETE",
    }
  );
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to delete category: ${errorText}`);
  }
}

export interface BulkAssignCategoriesResponse {
  updatedIds: string[];
  failed: { recipeId: string; reason: string }[];
}

export async function bulkAssignCategories(
  recipeIds: string[],
  categoryIds: string[]
): Promise<BulkAssignCategoriesResponse> {
  const response = await authFetch(
    `${API_BASE_URL}/recipes/bulk-categories`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ recipeIds, categoryIds }),
    }
  );
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to bulk assign categories: ${errorText}`);
  }
  return response.json();
}

/**
 * S3画像のpresigned URLを取得する
 */
export async function getImageUrl(s3Key: string): Promise<string> {
  const encodedKey = encodeURIComponent(s3Key);
  const response = await authFetch(
    `${API_BASE_URL}/image-url?key=${encodedKey}`
  );

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get image URL: ${errorText}`);
  }

  const data = await response.json();
  return data.url;
}

export interface IngredientItem {
  name: string;
  aliases: string[];
  category: string;
  usageCount: number;
}

export interface IngredientSearchResult {
  recipe: Recipe;
  missing: string[];
  matched: string[];
}

export interface IngredientSearchResponse {
  results: IngredientSearchResult[];
  unknownInputs: string[];
  unprocessedCount: number;
}

/**
 * 食材マスタ（調味料を除く）を、自分のレシピでよく使う順に取得する
 */
export async function getIngredients(): Promise<IngredientItem[]> {
  const response = await authFetch(`${API_BASE_URL}/ingredients`);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to get ingredients: ${errorText}`);
  }

  const data = await response.json();
  return data.ingredients;
}

/**
 * 手持ちの食材で作れる（あと1〜2品で作れる）レシピを探す
 */
export async function searchRecipesByIngredients(
  ingredients: string[]
): Promise<IngredientSearchResponse> {
  const response = await authFetch(
    `${API_BASE_URL}/recipes/search-by-ingredients`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ingredients }),
    }
  );

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to search recipes: ${errorText}`);
  }

  return response.json();
}

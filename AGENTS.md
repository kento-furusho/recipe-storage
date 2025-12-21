# レシピ保存アプリケーション 仕様書（MVP）

## 1. 概要

料理サイトや YouTube、スクリーンショットから「あとでまた作りたいレシピ」を迷わず見つけられるようにするための個人向け Web アプリ。

**コンセプト**: 「あれもう一回作りたいけど、どのレシピやったっけ？」を無くす

- レシピは URL / 画像（最大 3 枚） から登録
- 将来的に AI によるレシピ生成・献立提案を行う

## 2. 目的

- レシピ情報を **自分用に整理・蓄積** する
- 情報源（サイト・YouTube・スクショ）がバラバラでも一元管理できる
- 将来的に **嗜好分析・献立提案** につなげるための土台を作る

## 3. 想定ユーザー

- 個人利用（本人のみ）
- 初期段階では認証なし
- 内部的には `userId` を持つ設計（将来認証追加前提）

## 4. 機能一覧（MVP）

### 4.1 レシピ登録

#### 入力方法

- **画像アップロード（最大 3 枚）**
  - レシピサイトのスクショ
  - YouTube 概要欄のスクショ
- **URL 入力（任意）**
  - レシピサイト URL
  - YouTube 動画 URL（概要欄にレシピがない場合はリンクのみ保存）

#### 処理

- 画像は S3 へ presigned URL を使って直接アップロード
- 登録時に「レシピ生成ジョブ」を作成（非同期）

### 4.2 レシピ生成（非同期）

バックエンドで LLM を使用し、以下を抽出・生成：

- 料理名
- 材料一覧
- 手順
- 元情報（URL）

- 処理中は「生成中」ステータス
- 完了後にレシピとして保存

> **注**: MVP では ダミー実装 or 手動 JSON 生成でも可

詳細仕様は [4.2.1 LLM によるレシピ生成の詳細仕様](#421-llm-によるレシピ生成の詳細仕様) を参照

### 4.3 レシピ一覧

保存済みレシピを一覧表示

**表示内容**:

- 料理名
- サムネイル（1 枚目の画像）
- 作成日

### 4.4 レシピ詳細

レシピの詳細表示

**表示内容**:

- 料理名
- 材料
- 手順
- 参照元 URL
- 元画像（S3）

## 5. 技術構成

### フロントエンド

- **Next.js（TypeScript）**
- **デプロイ**: Vercel

**役割**:

- UI 表示
- API 呼び出し
- presigned URL を使った S3 PUT

### バックエンド（API）

- **Go**
- **API Gateway + Lambda**（予定）
- ローカル開発では `go run`

**主な API**:

- `POST /upload-url` - presigned URL 取得
- `POST /jobs` - ジョブ作成
- `GET /jobs/{jobId}` - ジョブ状態取得
- `GET /recipes` - レシピ一覧
- `GET /recipes/{recipeId}` - レシピ詳細
- `GET /image-url` - 画像の presigned URL 取得

### ストレージ

#### S3

- 画像保存
- パス例: `uploads/{userId}/{jobId}/{index}.jpg`

#### DynamoDB

- Users
- Jobs（レシピ生成ジョブ）
- Recipes

### 非同期処理

- **SQS**
- **Worker Lambda（Go）**

### AI（LLM）

- **AWS Bedrock Claude 3 Haiku**
- 詳細仕様は [4.2.1 LLM によるレシピ生成の詳細仕様](#421-llm-によるレシピ生成の詳細仕様) を参照

## 6. データモデル（簡易）

### Recipe

```json
{
  "recipeId": "uuid",
  "userId": "local",
  "title": "親子丼",
  "ingredients": ["鶏もも肉", "卵", "玉ねぎ"],
  "steps": ["下準備", "炒める", "煮る"],
  "imageKeys": ["uploads/local/xxx/0.jpg"],
  "sourceUrl": "https://example.com",
  "createdAt": "2025-12-21T00:00:00Z"
}
```

### Job

```json
{
  "jobId": "uuid",
  "userId": "local",
  "status": "PENDING | PROCESSING | SUCCEEDED | FAILED",
  "imageKeys": ["uploads/local/xxx/0.jpg"],
  "sourceUrl": null,
  "recipeId": null,
  "createdAt": "2025-12-21T00:00:00Z",
  "updatedAt": "2025-12-21T00:00:00Z"
}
```

### 4.2.1 LLM によるレシピ生成の詳細仕様

#### 使用する LLM サービス

**AWS Bedrock Claude 3 Haiku**

- マルチモーダル対応（画像 + テキスト）
- 高速処理と低コスト
- AWS 統合が容易
- MVP に適したコスト効率

> **注**: Claude Haiku は高速・低コストだが、画像解析精度は Sonnet/Opus より低い可能性がある。MVP 段階ではコスト効率を優先。将来的に精度が必要になった場合は、Sonnet/Opus への移行も検討可能。

#### 入力データ

**必須**:

- 画像（最大 3 枚、base64 エンコードまたは S3 URL）
  - 形式: JPEG, PNG
  - 最大サイズ: 10MB/枚
  - 推奨解像度: 1024x1024 以上

**任意**:

- ソース URL（レシピサイト、YouTube 等）
  - URL からメタデータを取得可能な場合は補助情報として使用

#### プロンプト設計

**基本プロンプト構造**:

```text
あなたは料理レシピを画像から抽出する専門家です。
以下の画像からレシピ情報を抽出し、JSON形式で返してください。

【画像】
[画像1, 画像2, 画像3 を base64 または URL で提供]

【補助情報】
- ソースURL: {sourceUrl}（存在する場合）

【出力形式】
以下のJSON形式で返してください：
{
  "title": "料理名（日本語）",
  "ingredients": [
    "材料1（分量含む）",
    "材料2（分量含む）",
    ...
  ],
  "steps": [
    "手順1（具体的な調理方法）",
    "手順2（具体的な調理方法）",
    ...
  ]
}

【注意事項】
- 画像にレシピ情報が含まれていない場合は、可能な限り推測してください
- 材料は分量を含めて記載してください（例: "鶏もも肉 200g"）
- 手順は具体的で実行可能な内容にしてください
- 不明な点は推測せず、空欄にするか「要確認」と記載してください
```

**プロンプトのバリエーション**:

- **画像のみの場合**: 上記の基本プロンプト
- **URL ありの場合**: URL から取得可能な情報（タイトル、説明文等）を補助情報として追加
- **複数画像の場合**: 各画像の役割を説明（例: "1 枚目: 完成写真、2 枚目: 材料、3 枚目: 手順"）

#### 出力形式（JSON スキーマ）

```json
{
  "title": "string (必須)",
  "ingredients": ["string"] (必須、空配列不可),
  "steps": ["string"] (必須、空配列不可)
}
```

**バリデーション**:

- `title`: 1 文字以上、100 文字以下
- `ingredients`: 1 個以上、50 個以下
- `steps`: 1 個以上、30 個以下

#### エラーハンドリング

**エラーケースと対応**:

1. **LLM API エラー**

   - タイムアウト: 60 秒でタイムアウト
   - レート制限: 指数バックオフでリトライ（最大 3 回）
   - API エラー: ジョブステータスを `FAILED` に設定

2. **画像解析失敗**

   - レシピ情報が抽出できない場合: デフォルト値を使用
     - `title`: URL から推測、または "レシピ"
     - `ingredients`: 空配列
     - `steps`: 空配列

3. **JSON パースエラー**
   - LLM の出力が不正な JSON の場合: リトライ（最大 2 回）
   - それでも失敗: デフォルト値を使用

#### 処理フロー

1. **ジョブ受信** (SQS または直接呼び出し)
2. **画像取得**: S3 から画像をダウンロード
3. **画像前処理**:
   - サイズ調整（必要に応じて）
   - base64 エンコード（API が要求する場合）
4. **LLM API 呼び出し**:
   - プロンプト構築
   - API リクエスト送信
   - レスポンス受信
5. **レスポンス処理**:
   - JSON パース
   - バリデーション
   - レシピオブジェクト生成
6. **保存**:
   - DynamoDB にレシピ保存
   - ジョブステータス更新

#### パフォーマンス要件

- **処理時間**: 30-60 秒以内（LLM API 呼び出し含む）
- **タイムアウト**: 120 秒
- **リトライ**: 最大 3 回（指数バックオフ: 1 秒、2 秒、4 秒）

#### コスト管理

- **画像サイズ最適化**: 10MB 以下に圧縮
- **レート制限**: 1 ユーザーあたり 10 リクエスト/分
- **コスト監視**: CloudWatch で API 呼び出し回数とコストを監視

#### 実装方針（MVP）

**フェーズ 1（現在）**: ダミー実装

- URL からタイトルを推測
- 固定の材料・手順を返す

**フェーズ 2**: LLM 統合

- AWS Bedrock Claude 3 Haiku を統合
- 画像解析機能を実装
- エラーハンドリングを追加

**フェーズ 3**: 最適化

- プロンプトの改善
- キャッシュ機能（同じ画像の再解析を避ける）
- バッチ処理の最適化

## 7. 処理フロー（画像登録）

1. フロントで画像選択
2. `POST /upload-url` → presigned URL 取得
3. ブラウザ → S3 へ PUT
4. `POST /jobs` でジョブ作成
5. Worker がジョブを処理
6. Recipe 生成・保存
7. フロントで一覧・詳細表示

## 8. 非スコープ（MVP ではやらない）

- ユーザー認証
- レシピの公開・共有
- お気に入り・評価
- 献立自動提案（将来）
- 栄養計算

## 9. 今後の拡張アイデア（メモ）

- レシピの再作成頻度分析
- 好み傾向から献立提案
- 「最近作ってないけど好きな料理」通知
- モバイル対応（PWA / Expo）

## 10. Cursor 向け一言コンテキスト

This project is a personal recipe management web app.
Users can upload up to 3 screenshots or paste recipe/YouTube URLs.
Images are uploaded directly to S3 using presigned URLs generated by a Go backend.
A background job (SQS + worker) processes images/URLs to generate structured recipes using an LLM.
Frontend is Next.js on Vercel, backend is Go (API + worker), storage is S3 + DynamoDB.
Current focus is MVP functionality and clean cloud-native architecture.

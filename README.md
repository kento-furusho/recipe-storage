# レシピ保存アプリケーション API

## セットアップ

### 1. 環境変数の設定

#### バックエンド（Go）

ルートディレクトリに `.env` ファイルを作成：

```bash
cp .env.example .env
```

`.env` ファイルを編集：

```env
S3_BUCKET=your-bucket-name
AWS_REGION=ap-northeast-1  # オプション
```

#### フロントエンド（Next.js）

`frontend` ディレクトリに `.env.local` ファイルを作成：

```bash
cd frontend
# .env.local.example を参考に作成
```

`.env.local` ファイルの内容：

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
```

### 2. S3バケットのCORS設定

S3バケットにCORS設定を適用：

```bash
# 方法1: スクリプトを使用
export S3_BUCKET=your-bucket-name
./apply-cors.sh

# 方法2: AWS CLIを直接使用
aws s3api put-bucket-cors \
  --bucket your-bucket-name \
  --cors-configuration file://cors.json
```

### 3. バックエンドの起動

```bash
go run main.go
```

バックエンドは `http://localhost:8080` で起動します。

### 4. フロントエンドの起動

```bash
cd frontend
nvm use  # Node.js 20を使用
npm run dev
```

フロントエンドは `http://localhost:3000` で起動します。

## 環境変数

### バックエンド

- `S3_BUCKET`: S3バケット名（必須）
- `AWS_ACCESS_KEY_ID`: AWSアクセスキー（オプション - デフォルトでは `~/.aws/credentials` を使用）
- `AWS_SECRET_ACCESS_KEY`: AWSシークレットキー（オプション）
- `AWS_REGION`: AWSリージョン（オプション）

### フロントエンド

- `NEXT_PUBLIC_API_URL`: バックエンドAPIのURL（デフォルト: `http://localhost:8080`）

## API エンドポイント

- `POST /upload-url`: presigned URLを取得

## 開発

### 必要な環境

- Go 1.25.5以上
- Node.js 20.9.0以上
- AWS CLI（S3バケットのCORS設定用）


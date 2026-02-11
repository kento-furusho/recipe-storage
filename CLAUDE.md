# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

### Backend (Go)
```bash
go run .                    # ローカル開発サーバー (port 8080)
sam build                   # Lambda用ビルド (API + Worker)
sam deploy                  # AWSへデプロイ
sam build && sam deploy     # ビルド & デプロイ
```

### Frontend (Next.js)
```bash
cd frontend && npm run dev      # 開発サーバー (port 3000)
cd frontend && npm run build    # プロダクションビルド
```

## Architecture

### Overview
レシピ画像をAI（Bedrock Claude 3 Haiku）で解析し、レシピデータとして保存するサーバーレスアプリケーション。

### Backend: 2つのLambda関数
- **RecipeAPIFunction** - API Gateway経由のHTTPリクエスト処理（chi router）
- **RecipeWorkerFunction** - SQSキューからジョブを非同期処理（LLMによるレシピ生成）

Go build tagsで同一コードベースから2つのバイナリを生成:
- `lambda_handler.go` (`//go:build lambda`) - API用、chi adapterでAPI Gatewayイベントをhttp.Requestに変換
- `worker_lambda.go` (`//go:build worker`) - Worker用、SQSイベントを処理
- `main.go` - ローカル開発用、Lambda環境変数(`_LAMBDA_TELEMETRY_LOG_FD`)がなければHTTPサーバー起動

### AWS Resources (template.yaml)
- **DynamoDB**: recipe-jobs, recipe-recipes, recipe-categories（すべてPAY_PER_REQUEST）
- **S3**: 画像ストレージ（ブラウザからpresigned URLで直接アップロード）
- **SQS**: ジョブキュー（API → Worker間の非同期処理）
- **Cognito**: User Pool + API Gateway Authorizer（認証）
- **API Gateway**: REST API、Cognito Authorizerがデフォルト（OPTIONSは除外）

### Frontend: Next.js 16 (App Router)
- `lib/api.ts` - 全API呼び出しの集約。`authFetch()`でCognito IDトークンを自動付与
- `lib/auth-context.tsx` - Cognito認証のReact Context (`useAuth()`)
- `components/AuthGuard.tsx` - 未認証ユーザーを`/login`にリダイレクト

### Key Patterns
- **ユーザーID**: `auth.go`の`getUserID(r)`がAPI Gatewayコンテキストから`claims.sub`を抽出、ローカルでは`"local"`
- **画像アップロード**: presigned URL経由でブラウザ→S3直接（Lambdaを経由しない）
- **ジョブ処理フロー**: Job作成 → SQS → Worker Lambda → Bedrock API → Recipe保存
- **LLMフォールバック**: Bedrock API失敗時はダミーレシピを生成（ジョブ自体は失敗しない）

## Environment Variables

### Backend (.env)
```
S3_BUCKET=<bucket-name>           # 必須
JOB_QUEUE_URL=<sqs-url>          # Lambda環境で自動設定
```

### Frontend (frontend/.env.local)
```
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_COGNITO_USER_POOL_ID=ap-northeast-1_XXXXXXXXX
NEXT_PUBLIC_COGNITO_CLIENT_ID=xxxxxxxxxxxxxxxxxxxxxxxxxx
```

## Deployment
- Backend: `sam deploy` → ap-northeast-1 (samconfig.toml)
- Frontend: Vercel（git push で自動デプロイ）。Cognito環境変数はVercel側にも設定が必要

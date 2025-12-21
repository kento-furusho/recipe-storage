# AWS 設定ガイド

このドキュメントでは、レシピ生成アプリで AWS Bedrock Claude 3 Haiku を使用するための設定方法を説明します。

## 1. AWS 認証情報の設定

### 方法 1: AWS CLI を使用（推奨）

1. **AWS CLI のインストール**

   ```bash
   # macOSの場合
   brew install awscli

   # または公式インストーラーを使用
   # https://aws.amazon.com/cli/
   ```

2. **AWS 認証情報の設定**

   ```bash
   aws configure
   ```

   以下の情報を入力します：

   - **AWS Access Key ID**: IAM ユーザーのアクセスキー ID
   - **AWS Secret Access Key**: IAM ユーザーのシークレットアクセスキー
   - **Default region name**: `ap-northeast-1`（東京リージョン）など
   - **Default output format**: `json`

3. **認証情報の確認**

   ```bash
   aws sts get-caller-identity
   ```

   正常に設定されていれば、アカウント情報が表示されます。

### 方法 2: 環境変数を使用

`.env`ファイルまたは環境変数で設定：

```bash
export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
export AWS_REGION=ap-northeast-1
```

または`.env`ファイルに追加：

```env
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key
AWS_REGION=ap-northeast-1
```

## 2. IAM ユーザーの作成と権限設定

### ステップ 1: IAM ユーザーの作成

1. AWS コンソールにログイン
2. **IAM**サービスに移動
3. **ユーザー** → **ユーザーを追加**
4. ユーザー名を入力（例: `recipe-api-user`）
5. **プログラムによるアクセス**を選択
6. **次へ**をクリック

### ステップ 2: 権限の付与

#### オプション A: 既存のポリシーをアタッチ（推奨）

以下のポリシーをアタッチ：

1. **AmazonS3FullAccess**（または S3 バケットへの最小権限）

   - S3 への画像アップロード・取得に必要

2. **AmazonBedrockFullAccess** または **AmazonBedrockLimitedAccess**
   - Bedrock API 呼び出しに必要
   - `AmazonBedrockLimitedAccess` は最小権限で推奨（セキュリティベストプラクティス）

#### オプション B: カスタムポリシーを作成（セキュリティ重視）

最小権限の原則に基づいたカスタムポリシー：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::recipe-uploads-dev-kento-1766215077/*"
    },
    {
      "Effect": "Allow",
      "Action": ["bedrock:InvokeModel"],
      "Resource": "arn:aws:bedrock:ap-northeast-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0"
    }
  ]
}
```

### ステップ 3: アクセスキーの作成

1. ユーザー作成後、**セキュリティ認証情報**タブに移動
2. **アクセスキーを作成**をクリック
3. **コマンドラインインターフェース (CLI)**を選択
4. アクセスキー ID とシークレットアクセスキーをコピー
   - ⚠️ **重要**: シークレットアクセスキーはこの時点でしか表示されません

## 3. Bedrock モデルアクセスの有効化

### 方法 1: AWS CLI で確認・有効化（推奨）

#### ステップ 1: 利用可能なモデルを確認

```bash
# Claude Haikuモデルを検索
aws bedrock list-foundation-models \
  --region ap-northeast-1 \
  --query "modelSummaries[?contains(modelName, 'Claude') && contains(modelName, 'Haiku')]" \
  --output table
```

#### ステップ 2: モデルアクセスの確認

```bash
# モデルが利用可能かテスト（実際に呼び出してみる）
aws bedrock invoke-model \
  --region ap-northeast-1 \
  --model-id anthropic.claude-3-haiku-20240307-v1:0 \
  --body '{"anthropic_version":"bedrock-2023-05-31","max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}' \
  --cli-binary-format raw-in-base64-out \
  --output json
```

**エラーが発生する場合**:

- `AccessDeniedException`: モデルアクセスが有効化されていない
- `ValidationException`: リクエスト形式が間違っている（アクセスは有効）

#### ステップ 3: モデルアクセスの有効化

**重要**: Anthropic（Claude）モデルを使用するには、**利用目的フォーム**の提出が必要です。

##### 3-1. 利用目的フォームの提出（必須）

1. AWS コンソール → **Amazon Bedrock** に移動
2. 左メニューで **Foundation models** または **Get started** を選択
3. 「Claude 3 Haiku」を検索
4. モデルを選択して **Request access** または **Enable** をクリック
5. **利用目的フォーム**が表示されるので、以下を記入：
   - 利用目的（例: "個人用レシピ管理アプリでの画像からのレシピ情報抽出"）
   - 使用ケースの説明
   - その他の必要事項
6. フォームを提出

**注意**:

- フォーム提出後、**約 15 分**で有効化されます
- 有効化されるまで、以下のエラーが発生します：
  ```
  ResourceNotFoundException: Model use case details have not been submitted for this account.
  Fill out the Anthropic use case details form before using the model.
  ```

##### 3-2. アクセス状態の確認

以下のコマンドでアクセス状態を確認：

```bash
# モデル一覧が取得できれば、基本的なアクセス権限はある
aws bedrock list-foundation-models --region ap-northeast-1

# 特定のモデルの詳細を確認
aws bedrock get-foundation-model \
  --region ap-northeast-1 \
  --model-identifier anthropic.claude-3-haiku-20240307-v1:0

# 実際にAPIを呼び出してテスト（有効化後）
aws bedrock invoke-model \
  --region ap-northeast-1 \
  --model-id anthropic.claude-3-haiku-20240307-v1:0 \
  --body '{"anthropic_version":"bedrock-2023-05-31","max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}' \
  --cli-binary-format raw-in-base64-out \
  --output json
```

### 方法 2: AWS コンソールから有効化

1. AWS コンソールで**Amazon Bedrock**サービスに移動
2. 左メニューで**Foundation models**を選択（または**Get started**ページ）
3. 検索バーで「Claude Haiku」を検索
4. モデルを選択して**Enable**または**Request access**をクリック

### ステップ 4: リージョンの確認

- Claude Haiku は以下のリージョンで利用可能：
  - `us-east-1` (バージニア北部)
  - `us-west-2` (オレゴン)
  - `ap-northeast-1` (東京) ← 推奨
  - `eu-west-1` (アイルランド)

**注意**: リージョンごとにモデルアクセスを有効化する必要があります。

## 4. 動作確認

### ステップ 1: AWS CLI で Bedrock アクセスを確認

```bash
# モデル一覧を取得（アクセス権限の確認）
aws bedrock list-foundation-models --region ap-northeast-1

# Claude 3 Haikuが表示されればOK
```

### ステップ 2: アプリケーションでテスト

1. `.env`ファイルに認証情報を設定（方法 2 を使用する場合）
2. バックエンドを起動：
   ```bash
   cd /Users/kento/Documents/programing/recipe-api
   go run main.go
   ```
3. フロントエンドから画像をアップロード
4. ログで Bedrock API 呼び出しを確認

## 5. トラブルシューティング

### エラー: "AccessDeniedException"

**原因**: Bedrock モデルへのアクセスが有効化されていない

**解決方法**:

1. Bedrock コンソールでモデルアクセスを確認
2. 必要なリージョンでモデルを有効化
3. IAM ポリシー（`AmazonBedrockLimitedAccess`など）がアタッチされているか確認

### エラー: "ResourceNotFoundException: Model use case details have not been submitted"

**原因**: Anthropic（Claude）モデルの利用目的フォームが提出されていない

**エラーメッセージ例**:

```
ResourceNotFoundException: Model use case details have not been submitted for this account.
Fill out the Anthropic use case details form before using the model.
If you have already filled out the form, try again in 15 minutes.
```

**解決方法**:

1. AWS コンソール → **Amazon Bedrock** → **Foundation models**
2. Claude 3 Haiku を検索して選択
3. **Request access** をクリック
4. 利用目的フォームに記入して提出
   - 利用目的（例: "個人用レシピ管理アプリでの画像からのレシピ情報抽出"）
   - 使用ケースの説明
   - その他の必要事項
5. **約 15 分待つ**（有効化まで時間がかかります）
6. 再度レシピ登録を試す

**確認方法**:

```bash
# API呼び出しでテスト（成功すれば有効化済み）
aws bedrock invoke-model \
  --region ap-northeast-1 \
  --model-id anthropic.claude-3-haiku-20240307-v1:0 \
  --body '{"anthropic_version":"bedrock-2023-05-31","max_tokens":10,"messages":[{"role":"user","content":[{"type":"text","text":"test"}]}]}' \
  --cli-binary-format raw-in-base64-out
```

### エラー: "UnrecognizedClientException"

**原因**: AWS 認証情報が正しく設定されていない

**解決方法**:

```bash
# 認証情報を確認
aws sts get-caller-identity

# 設定を確認
aws configure list
```

### エラー: "Model not found" または "ResourceNotFoundException"

**原因**: モデル ID が間違っている、またはリージョンが異なる

**解決方法**:

- モデル ID を確認: `anthropic.claude-3-haiku-20240307-v1:0`
- リージョンが正しいか確認（`ap-northeast-1`など）

### エラー: "ThrottlingException"

**原因**: レート制限に達している

**解決方法**:

- リトライロジックが実装されているので、自動的に再試行されます
- 頻繁に発生する場合は、リクエスト頻度を下げる

## 6. セキュリティのベストプラクティス

1. **最小権限の原則**: 必要最小限の権限のみを付与
2. **アクセスキーのローテーション**: 定期的にアクセスキーを更新
3. **環境変数の保護**: `.env`ファイルを`.gitignore`に追加（既に追加済み）
4. **IAM ロールの使用**: EC2 や Lambda で実行する場合は、IAM ロールを使用（アクセスキーより安全）

## 7. コスト管理

- Claude 3 Haiku は低コストですが、使用量を監視することを推奨
- CloudWatch で API 呼び出し回数とコストを確認可能
- レート制限を実装済み（1 ユーザーあたり 10 リクエスト/分）

## 参考リンク

- [AWS Bedrock ドキュメント](https://docs.aws.amazon.com/bedrock/)
- [Claude 3 Haiku モデルカード](https://docs.anthropic.com/claude/docs/models-overview)
- [IAM ベストプラクティス](https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html)

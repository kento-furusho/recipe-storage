#!/bin/bash

# S3バケット名を環境変数から取得、または引数から取得
BUCKET_NAME=${S3_BUCKET:-$1}

if [ -z "$BUCKET_NAME" ]; then
  echo "エラー: S3バケット名を指定してください"
  echo "使用方法: ./apply-cors.sh <バケット名>"
  echo "または環境変数: export S3_BUCKET=<バケット名> && ./apply-cors.sh"
  exit 1
fi

echo "S3バケット '$BUCKET_NAME' にCORS設定を適用しています..."

# CORS設定を適用
aws s3api put-bucket-cors \
  --bucket "$BUCKET_NAME" \
  --cors-configuration file://cors.json

if [ $? -eq 0 ]; then
  echo "✅ CORS設定が正常に適用されました！"
  echo ""
  echo "現在のCORS設定を確認:"
  aws s3api get-bucket-cors --bucket "$BUCKET_NAME"
else
  echo "❌ CORS設定の適用に失敗しました"
  echo "以下を確認してください:"
  echo "1. AWS CLIがインストールされているか"
  echo "2. AWS認証情報が正しく設定されているか"
  echo "3. バケット名が正しいか"
  echo "4. バケットへのアクセス権限があるか"
  exit 1
fi


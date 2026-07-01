#!/bin/bash
set -e

echo "Building Lambda function..."

# Goソースは backend/ に配置（sam build が frontend をコピーしないための構成）
cd "$(dirname "$0")/backend"

# Lambda用にビルド（Linux環境、静的リンク）
# lambdaビルドタグを使用してlambda_handler.goをエントリーポイントとして使用
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -tags lambda,lambda.noroot \
  -o ../lambda/bootstrap \
  -ldflags="-s -w" \
  .

echo "Build complete! Output: lambda/bootstrap"


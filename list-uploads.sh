#!/bin/bash

BUCKET_NAME="recipe-uploads-dev-kento-1766215077"

echo "📦 S3バケット内のアップロード済み画像:"
echo ""
aws s3 ls s3://${BUCKET_NAME}/uploads/local/ --recursive --human-readable | tail -20

echo ""
echo "📊 統計情報:"
TOTAL=$(aws s3 ls s3://${BUCKET_NAME}/uploads/local/ --recursive | wc -l | tr -d ' ')
echo "総ファイル数: ${TOTAL}"

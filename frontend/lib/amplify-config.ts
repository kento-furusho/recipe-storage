import { Amplify } from "aws-amplify";

const userPoolId = process.env.NEXT_PUBLIC_COGNITO_USER_POOL_ID || "";
const userPoolClientId = process.env.NEXT_PUBLIC_COGNITO_CLIENT_ID || "";

// User Pool IDからリージョンを抽出 (例: "ap-northeast-1_XXXXXXXXX" → "ap-northeast-1")
const region = userPoolId.split("_")[0] || "ap-northeast-1";

export function configureAmplify() {
  if (!userPoolId || !userPoolClientId) {
    console.warn(
      "Cognito環境変数が設定されていません。NEXT_PUBLIC_COGNITO_USER_POOL_ID と NEXT_PUBLIC_COGNITO_CLIENT_ID を確認してください。"
    );
    return;
  }

  Amplify.configure({
    Auth: {
      Cognito: {
        userPoolId,
        userPoolClientId,
      },
    },
  });
}

import { Amplify } from "aws-amplify";

const userPoolId = process.env.NEXT_PUBLIC_COGNITO_USER_POOL_ID || "";
const userPoolClientId = process.env.NEXT_PUBLIC_COGNITO_CLIENT_ID || "";

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

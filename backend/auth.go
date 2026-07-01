package main

import (
	"net/http"

	"github.com/awslabs/aws-lambda-go-api-proxy/core"
)

// getUserID はリクエストからCognitoユーザーIDを取得する。
// API Gateway Cognito Authorizerのclaimsからsubを抽出し、
// Lambda外（ローカル開発）では "local" を返す。
func getUserID(r *http.Request) string {
	apiGwContext, ok := core.GetAPIGatewayContextFromContext(r.Context())
	if !ok {
		return "local"
	}

	claims, ok := apiGwContext.Authorizer["claims"]
	if !ok {
		return "local"
	}

	claimsMap, ok := claims.(map[string]interface{})
	if !ok {
		return "local"
	}

	sub, ok := claimsMap["sub"].(string)
	if !ok || sub == "" {
		return "local"
	}

	return sub
}

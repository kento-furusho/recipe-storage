package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
)

// newRequestWithCognitoClaims creates an *http.Request with API Gateway context
// containing the given Cognito authorizer claims.
func newRequestWithCognitoClaims(claims map[string]interface{}) *http.Request {
	accessor := core.RequestAccessor{}
	apiGwReq := events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/test",
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"claims": claims,
			},
		},
	}
	req, _ := accessor.EventToRequestWithContext(context.Background(), apiGwReq)
	return req
}

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name: "valid Cognito sub",
			request: newRequestWithCognitoClaims(map[string]interface{}{
				"sub":   "3754ca28-b0b1-70d1-e639-015dae933ac4",
				"email": "user@example.com",
			}),
			expected: "3754ca28-b0b1-70d1-e639-015dae933ac4",
		},
		{
			name:     "no API Gateway context (local dev)",
			request:  httptest.NewRequest("GET", "/test", nil),
			expected: "local",
		},
		{
			name: "API Gateway context but no claims",
			request: func() *http.Request {
				accessor := core.RequestAccessor{}
				apiGwReq := events.APIGatewayProxyRequest{
					HTTPMethod: "GET",
					Path:       "/test",
					RequestContext: events.APIGatewayProxyRequestContext{
						Authorizer: map[string]interface{}{},
					},
				}
				req, _ := accessor.EventToRequestWithContext(context.Background(), apiGwReq)
				return req
			}(),
			expected: "local",
		},
		{
			name: "claims is not a map",
			request: func() *http.Request {
				accessor := core.RequestAccessor{}
				apiGwReq := events.APIGatewayProxyRequest{
					HTTPMethod: "GET",
					Path:       "/test",
					RequestContext: events.APIGatewayProxyRequestContext{
						Authorizer: map[string]interface{}{
							"claims": "invalid-string",
						},
					},
				}
				req, _ := accessor.EventToRequestWithContext(context.Background(), apiGwReq)
				return req
			}(),
			expected: "local",
		},
		{
			name: "sub is empty string",
			request: newRequestWithCognitoClaims(map[string]interface{}{
				"sub": "",
			}),
			expected: "local",
		},
		{
			name: "sub is not a string",
			request: newRequestWithCognitoClaims(map[string]interface{}{
				"sub": 12345,
			}),
			expected: "local",
		},
		{
			name: "claims map exists but sub key missing",
			request: newRequestWithCognitoClaims(map[string]interface{}{
				"email": "user@example.com",
			}),
			expected: "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUserID(tt.request)
			if got != tt.expected {
				t.Errorf("getUserID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

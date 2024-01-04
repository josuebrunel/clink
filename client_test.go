package clink_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/davesavic/clink"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	testCases := []struct {
		name   string
		opts   []clink.Option
		result func(*clink.Client) bool
	}{
		{
			name: "default client with no options",
			opts: []clink.Option{},
			result: func(client *clink.Client) bool {
				return client.HttpClient != nil && client.Headers != nil && len(client.Headers) == 0
			},
		},
		{
			name: "client with custom http client",
			opts: []clink.Option{
				clink.WithClient(nil),
			},
			result: func(client *clink.Client) bool {
				return client.HttpClient == nil
			},
		},
		{
			name: "client with custom headers",
			opts: []clink.Option{
				clink.WithHeaders(map[string]string{"key": "value"}),
			},
			result: func(client *clink.Client) bool {
				return client.Headers != nil && len(client.Headers) == 1
			},
		},
		{
			name: "client with custom header",
			opts: []clink.Option{
				clink.WithHeader("key", "value"),
			},
			result: func(client *clink.Client) bool {
				return client.Headers != nil && len(client.Headers) == 1
			},
		},
		{
			name: "client with custom rate limit",
			opts: []clink.Option{
				clink.WithRateLimit(60),
			},
			result: func(client *clink.Client) bool {
				return client.RateLimiter != nil && client.RateLimiter.Limit() == 1
			},
		},
		{
			name: "client with basic auth",
			opts: []clink.Option{
				clink.WithBasicAuth("username", "password"),
			},
			result: func(client *clink.Client) bool {
				b64, err := base64.StdEncoding.DecodeString(
					strings.Replace(client.Headers["Authorization"], "Basic ", "", 1),
				)
				if err != nil {
					return false
				}

				return string(b64) == "username:password"
			},
		},
		{
			name: "client with bearer token",
			opts: []clink.Option{
				clink.WithBearerAuth("token"),
			},
			result: func(client *clink.Client) bool {
				return client.Headers["Authorization"] == "Bearer token"
			},
		},
		{
			name: "client with user agent",
			opts: []clink.Option{
				clink.WithUserAgent("user-agent"),
			},
			result: func(client *clink.Client) bool {
				return client.Headers["User-Agent"] == "user-agent"
			},
		},
		{
			name: "client with retries",
			opts: []clink.Option{
				clink.WithRetries(3, func(request *http.Request, response *http.Response, err error) bool {
					return true
				}),
			},
			result: func(client *clink.Client) bool {
				return client.MaxRetries == 3 && client.ShouldRetryFunc != nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := clink.NewClient(tc.opts...)

			if c == nil {
				t.Error("expected client to be created")
			}

			if !tc.result(c) {
				t.Errorf("expected client to be created with options: %+v", tc.opts)
			}
		})
	}
}

func TestClient_Do(t *testing.T) {
	testCases := []struct {
		name        string
		opts        []clink.Option
		setupServer func() *httptest.Server
		resultFunc  func(*http.Response, error) bool
	}{
		{
			name: "successful response no body",
			opts: []clink.Option{},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			resultFunc: func(response *http.Response, err error) bool {
				return response != nil && err == nil && response.StatusCode == http.StatusOK
			},
		},
		{
			name: "successful response with text body",
			opts: []clink.Option{},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("response"))
				}))
			},
			resultFunc: func(response *http.Response, err error) bool {
				bodyContents, err := io.ReadAll(response.Body)
				if err != nil {
					return false
				}

				return string(bodyContents) == "response"
			},
		},
		{
			name: "successful response with json body",
			opts: []clink.Option{},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(map[string]string{"key": "value"})
				}))
			},
			resultFunc: func(response *http.Response, err error) bool {
				var target map[string]string
				er := clink.ResponseToJson(response, &target)
				if er != nil {
					return false
				}

				return target["key"] == "value"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := tc.setupServer()
			defer server.Close()

			opts := append(tc.opts, clink.WithClient(server.Client()))
			c := clink.NewClient(opts...)

			if c == nil {
				t.Error("expected client to be created")
			}

			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			if err != nil {
				t.Errorf("failed to create request: %v", err)
			}

			resp, err := c.Do(req)
			if !tc.resultFunc(resp, err) {
				t.Errorf("expected result to be successful")
			}
		})
	}
}

func TestClient_ResponseToJson(t *testing.T) {
	testCases := []struct {
		name       string
		response   *http.Response
		target     any
		resultFunc func(*http.Response, any) bool
	}{
		{
			name: "successful response with json body",
			response: &http.Response{
				Body: io.NopCloser(strings.NewReader(`{"key": "value"}`)),
			},
			resultFunc: func(response *http.Response, target any) bool {
				var t map[string]string
				er := clink.ResponseToJson(response, &t)
				if er != nil {
					return false
				}

				return t["key"] == "value"
			},
		},
		{
			name:     "response is nil",
			response: nil,
			resultFunc: func(response *http.Response, target any) bool {
				var t map[string]string
				er := clink.ResponseToJson(response, &t)
				if er == nil {
					return false
				}

				return er.Error() == "response is nil"
			},
		},
		{
			name: "response body is nil",
			response: &http.Response{
				Body: nil,
			},
			resultFunc: func(response *http.Response, target any) bool {
				var t map[string]string
				er := clink.ResponseToJson(response, &t)
				if er == nil {
					return false
				}

				return er.Error() == "response body is nil"
			},
		},
		{
			name: "json decode error",
			response: &http.Response{
				Body: io.NopCloser(strings.NewReader(`{"key": "value`)),
			},
			target: nil,
			resultFunc: func(response *http.Response, target any) bool {
				var t map[string]string
				er := clink.ResponseToJson(response, &t)
				if er == nil {
					return false
				}

				return strings.Contains(er.Error(), "failed to decode response")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.resultFunc(tc.response, tc.target) {
				t.Errorf("expected result to be successful")
			}
		})
	}
}

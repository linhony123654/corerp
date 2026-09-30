package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func TestProxyFallbackCannotEscapeRestrictedPathAllowlist(t *testing.T) {
	ctx := context.Background()
	_, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "proxy-paths.db"))
	var fallbackCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" || r.URL.Path == "/models" {
			fallbackCalls.Add(1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	handler.(*Server).endpointPolicy = endpointpolicy.Policy{
		LocalOrigins: []string{upstream.URL},
		LocalPaths:   map[string][]string{upstream.URL: {"/v1/chat/completions", "/v1/models"}},
	}
	response := performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, ProxyTestRequest{Endpoint: upstream.URL + "/v1/chat/completions", Model: "fixture"})
	assertStatus(t, response, http.StatusOK)
	if result := decodeData[ProxyTestResult](t, response); result.OK {
		t.Fatalf("restricted chat fallback succeeded: %+v", result)
	}
	response = performJSON(t, handler, "/api/v1/proxy/models", rpPlayerToken, ProxyModelsRequest{Endpoint: upstream.URL + "/v1/models"})
	assertStatus(t, response, http.StatusOK)
	if result := decodeData[ProxyModelsResult](t, response); result.OK {
		t.Fatalf("restricted models fallback succeeded: %+v", result)
	}
	if fallbackCalls.Load() != 0 {
		t.Fatalf("out-of-policy fallback reached upstream %d times", fallbackCalls.Load())
	}
}

func TestProxyTestAndModels(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-http.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	defer store.Close()

	// Mock upstream LLM server
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		switch r.URL.Path {
		case "/v1/chat/completions":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
		case "/v1/models":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"mock-gpt-4o"},{"id":"mock-deepseek-chat"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	handler.(*Server).endpointPolicy = endpointpolicy.Policy{LocalOrigins: []string{upstream.URL}}

	// Proxy routes are authenticated before any upstream request is initiated.
	testReq := ProxyTestRequest{
		Endpoint:       upstream.URL + "/v1/chat/completions",
		APIKey:         "sk-mock-key",
		Model:          "mock-model",
		TimeoutSeconds: 5,
	}
	resp := performJSON(t, handler, "/api/v1/proxy/test", "", testReq)
	assertAPIError(t, resp, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED")
	if upstreamCalls.Load() != 0 {
		t.Fatalf("unauthenticated proxy request reached upstream: %d", upstreamCalls.Load())
	}
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, testReq)
	assertStatus(t, resp, http.StatusOK)
	testResult := decodeData[ProxyTestResult](t, resp)
	if !testResult.OK {
		t.Fatalf("expected proxy test to succeed, got: %+v", testResult)
	}

	// 1b. Test Proxy Test Connection with Base URL ending in /v1 (auto-resolution)
	testBaseReq := ProxyTestRequest{
		Endpoint:       upstream.URL + "/v1",
		APIKey:         "sk-mock-key",
		Model:          "mock-model",
		TimeoutSeconds: 5,
	}
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, testBaseReq)
	assertStatus(t, resp, http.StatusOK)
	testBaseResult := decodeData[ProxyTestResult](t, resp)
	if !testBaseResult.OK {
		t.Fatalf("expected proxy test with /v1 base URL to succeed, got: %+v", testBaseResult)
	}

	// 1c. Test Proxy Test Connection with bare domain URL (auto-resolution)
	testBareReq := ProxyTestRequest{
		Endpoint:       upstream.URL,
		APIKey:         "sk-mock-key",
		Model:          "mock-model",
		TimeoutSeconds: 5,
	}
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, testBareReq)
	assertStatus(t, resp, http.StatusOK)
	testBareResult := decodeData[ProxyTestResult](t, resp)
	if !testBareResult.OK {
		t.Fatalf("expected proxy test with bare URL to succeed, got: %+v", testBareResult)
	}

	// 2. Test Proxy Models Pulling without auth (public)
	modelsReq := ProxyModelsRequest{
		Endpoint:       upstream.URL + "/v1/chat/completions",
		APIKey:         "sk-mock-key",
		TimeoutSeconds: 5,
	}
	beforeUnauthModels := upstreamCalls.Load()
	resp = performJSON(t, handler, "/api/v1/proxy/models", "", modelsReq)
	assertAPIError(t, resp, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED")
	if upstreamCalls.Load() != beforeUnauthModels {
		t.Fatal("unauthenticated model-list request reached upstream")
	}
	resp = performJSON(t, handler, "/api/v1/proxy/models", rpPlayerToken, modelsReq)
	assertStatus(t, resp, http.StatusOK)
	modelsResult := decodeData[ProxyModelsResult](t, resp)
	if !modelsResult.OK || len(modelsResult.Models) != 2 {
		t.Fatalf("expected proxy models to return 2 models, got: %+v", modelsResult)
	}
	if modelsResult.Models[0] != "mock-deepseek-chat" || modelsResult.Models[1] != "mock-gpt-4o" {
		t.Fatalf("unexpected models list: %+v", modelsResult.Models)
	}

	// Metadata destinations are blocked even under the explicit local test allowance.
	callsBeforeBlocked := upstreamCalls.Load()
	blockedReq := ProxyTestRequest{Endpoint: "http://169.254.169.254/latest/meta-data/", APIKey: "must-not-leak"}
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, blockedReq)
	assertStatus(t, resp, http.StatusOK)
	if result := decodeData[ProxyTestResult](t, resp); result.OK || strings.Contains(result.Message, "169.254") || strings.Contains(result.Message, "must-not-leak") {
		t.Fatalf("unsafe metadata destination or detail leaked: %+v", result)
	}
	if upstreamCalls.Load() != callsBeforeBlocked {
		t.Fatal("metadata destination reached upstream")
	}
	for _, endpoint := range []string{upstream.URL + "/admin/export", upstream.URL + "/v1/%2e%2e/admin"} {
		resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, ProxyTestRequest{Endpoint: endpoint, APIKey: "secret"})
		result := decodeData[ProxyTestResult](t, resp)
		if result.OK || strings.Contains(result.Message, endpoint) || upstreamCalls.Load() != callsBeforeBlocked {
			t.Fatalf("same-origin arbitrary/traversal path was not rejected: endpoint=%q result=%+v calls=%d", endpoint, result, upstreamCalls.Load())
		}
	}

	// 3. Test Proxy with bad endpoint
	badReq := ProxyTestRequest{
		Endpoint: "http://127.0.0.1:59999/does-not-exist",
	}
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, badReq)
	assertStatus(t, resp, http.StatusOK)
	badResult := decodeData[ProxyTestResult](t, resp)
	if badResult.OK {
		t.Fatalf("expected proxy test to fail for unreachable endpoint")
	}

	secret := "upstream-secret-error"
	privateErrorEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"` + secret + `"}}`))
	}))
	defer privateErrorEndpoint.Close()
	resp = performJSON(t, handler, "/api/v1/proxy/test", rpPlayerToken, ProxyTestRequest{
		Endpoint: privateErrorEndpoint.URL + "/v1/chat/completions", APIKey: "private-key", Model: "fixture",
	})
	assertStatus(t, resp, http.StatusOK)
	if result := decodeData[ProxyTestResult](t, resp); result.OK || strings.Contains(result.Message, secret) || strings.Contains(result.Message, privateErrorEndpoint.URL) || strings.Contains(result.Message, "private-key") {
		t.Fatalf("upstream error details were not redacted: %+v", result)
	}
}

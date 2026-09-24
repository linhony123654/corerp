package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestBrowserOriginPolicyValidationAndPreflight(t *testing.T) {
	for _, origin := range []string{"", "*", "null", "file://", "https://*.example.com", "https://user:pass@example.com", "https://example.com/", "https://example.com/path", "https://example.com?x=1", "https://example.com#fragment", " https://example.com", "https://example.com\n", "https://", "http://example.com:bad"} {
		if _, err := BrowserOriginPolicy([]string{origin}); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid origin accepted: %q %v", origin, err)
		}
	}
	const origin = "http://127.0.0.1:8000"
	policy, err := BrowserOriginPolicy([]string{origin, "https://tavern.example.com", "http://[::1]:8000"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusAccepted) })
	handler := policy(next)
	for _, test := range []struct {
		name, origin, method, headers, path string
		status                              int
	}{
		{"post", origin, "POST", "Authorization, Content-Type", "/api/v1/rp/observe", 204},
		{"stream", origin, "GET", "authorization, last-event-id", "/api/v1/rp/events/stream", 204},
		{"no-extra-headers", origin, "GET", "", "/api/v1/rp/events", 204},
		{"foreign", "http://evil.example.com", "POST", "authorization", "/api/v1/rp/observe", 403},
		{"suffix", origin + ".evil.example.com", "POST", "authorization", "/api/v1/rp/observe", 403},
		{"opaque", "null", "POST", "authorization", "/api/v1/rp/observe", 403},
		{"method", origin, "DELETE", "authorization", "/api/v1/rp/observe", 400},
		{"header", origin, "POST", "authorization, x-principal-id", "/api/v1/rp/observe", 400},
		{"outside-api", origin, "GET", "authorization", "/healthz", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodOptions, test.path, nil)
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Access-Control-Request-Method", test.method)
			if test.headers != "" {
				r.Header.Set("Access-Control-Request-Headers", test.headers)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			assertStatus(t, w, test.status)
			if calls != 0 {
				t.Fatal("preflight invoked business handler")
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" || w.Header().Get("Access-Control-Allow-Origin") == "*" || w.Header().Get("Access-Control-Allow-Private-Network") != "" {
				t.Fatal("overbroad browser grant")
			}
			if test.status == 204 && (w.Header().Get("Access-Control-Allow-Origin") != origin || !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Origin")) {
				t.Fatal("missing exact origin/cache boundary")
			}
		})
	}
	for _, configured := range []bool{false, true} {
		middleware := policy
		if !configured {
			middleware, err = BrowserOriginPolicy(nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		w := httptest.NewRecorder()
		middleware(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/rp/events", nil))
		assertStatus(t, w, http.StatusAccepted)
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("non-browser/default request gained CORS grant")
		}
	}
}

func TestBrowserOriginPolicyKeepsRealAPIAuthentication(t *testing.T) {
	ctx := context.Background()
	s, api := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "origins.db"))
	defer s.Close()
	const origin = "http://127.0.0.1:8000"
	policy, err := BrowserOriginPolicy([]string{origin})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		origin, token string
		status        int
	}{
		{origin, "", 401}, {origin, "not-a-token", 401}, {origin, rpPlayerToken, 404}, {"http://localhost:8000", rpPlayerToken, 403},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/rp/sessions/read", strings.NewReader(`{"session_id":"session_absent"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", test.origin)
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		w := httptest.NewRecorder()
		policy(api).ServeHTTP(w, r)
		assertStatus(t, w, test.status)
		if test.origin == origin && w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("allowed browser cannot read authenticated error")
		}
		if test.origin != origin && w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("foreign browser received grant")
		}
	}
}

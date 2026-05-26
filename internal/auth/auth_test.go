package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInitNoAuth(t *testing.T) {
	Init("")
	SetSecureCookie(true)

	if IsEnabled() {
		t.Error("should not be enabled with empty secret")
	}
}

func TestInitWithSecret(t *testing.T) {
	Init("test-secret")
	SetSecureCookie(true)

	if !IsEnabled() {
		t.Error("should be enabled with secret")
	}
}

func TestGenerateAndVerifyToken(t *testing.T) {
	Init("my-secret-key")
	SetSecureCookie(true)

	token := GenerateToken()
	if token == "" {
		t.Fatal("token should not be empty")
	}

	if !VerifyToken(token) {
		t.Error("token should verify successfully")
	}

	if VerifyToken("bogus.token") {
		t.Error("bogus token should not verify")
	}

	if VerifyToken("") {
		t.Error("empty token should not verify")
	}
}

func TestMiddlewareNoAuth(t *testing.T) {
	Init("")
	SetSecureCookie(true)

	called := false
	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/state", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler should be called when auth is disabled")
	}
}

func TestMiddlewareNoCookie(t *testing.T) {
	Init("secret")
	SetSecureCookie(false)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth")
	})

	req := httptest.NewRequest("GET", "/api/state", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddlewareValidToken(t *testing.T) {
	Init("secret")
	SetSecureCookie(false)

	token := GenerateToken()

	called := false
	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler should be called with valid cookie")
	}
}

func TestMiddlewareBearerToken(t *testing.T) {
	Init("secret")
	SetSecureCookie(false)

	token := GenerateToken()

	called := false
	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler should be called with valid bearer token")
	}
}

func TestLoginPageGet(t *testing.T) {
	Init("password")
	SetSecureCookie(false)

	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /login → %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "CoreRP") {
		t.Error("login page should contain 'CoreRP'")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	Init("correct")
	SetSecureCookie(false)

	form := strings.NewReader("password=wrong")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("wrong password → %d, want 200 (re-display form)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "密码错误") {
		t.Error("should show error message for wrong password")
	}
}

func TestLoginCorrectPassword(t *testing.T) {
	Init("correct")
	SetSecureCookie(false)

	form := strings.NewReader("password=correct")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("correct password → %d, want 303 redirect", rec.Code)
	}

	// Should set cookie
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == cookieName {
			found = true
			if !c.HttpOnly {
				t.Error("cookie should be HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Error("cookie should be SameSite=Lax")
			}
			break
		}
	}
	if !found {
		t.Error("should set session cookie on successful login")
	}
}

func TestSecureCookieFlag(t *testing.T) {
	Init("pass")
	SetSecureCookie(true)

	form := strings.NewReader("password=pass")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			if !c.Secure {
				t.Error("cookie should be Secure when SetSecureCookie(true)")
			}
			return
		}
	}
	t.Error("cookie not found")
}

func TestSecureCookieFlagFalse(t *testing.T) {
	Init("pass")
	SetSecureCookie(false)

	form := strings.NewReader("password=pass")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			if c.Secure {
				t.Error("cookie should NOT be Secure when SetSecureCookie(false)")
			}
			return
		}
	}
	t.Error("cookie not found")
}

func TestMiddlewareBrowserRedirect(t *testing.T) {
	Init("secret")
	SetSecureCookie(false)

	handler := Middleware(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach handler")
	})

	// Browser request to non-API page without cookie
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("browser without cookie → %d, want 303 redirect to /login", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if loc != "/login" {
		t.Errorf("redirect location = %s, want /login", loc)
	}
}

func TestMiddlewareLoginPageAccessible(t *testing.T) {
	Init("secret")
	SetSecureCookie(false)

	// Login page is normally registered WITHOUT middleware (per server.go Register)
	// This tests the bare handler, not the wrapped version.
	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /login (no middleware) → %d, want 200", rec.Code)
	}
}

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const cookieName = "corerp_session"
const tokenTTL = 24 * time.Hour

var sharedSecret string

// Init sets the shared secret for token signing.
func Init(secret string) {
	sharedSecret = secret
}

// IsEnabled returns true if authentication is configured.
func IsEnabled() bool {
	return sharedSecret != ""
}

// GenerateToken creates a time-limited HMAC token.
func GenerateToken() string {
	ts := time.Now().UTC().Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(sharedSecret))
	mac.Write([]byte(ts))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString([]byte(ts)), sig)
}

// VerifyToken checks if a token is valid and not expired.
func VerifyToken(token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	tsBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	ts, err := time.Parse(time.RFC3339, string(tsBytes))
	if err != nil {
		return false
	}
	if time.Since(ts) > tokenTTL {
		return false
	}
	// Verify signature
	mac := hmac.New(sha256.New, []byte(sharedSecret))
	mac.Write([]byte(string(tsBytes)))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// Middleware wraps a handler with authentication check.
func Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !IsEnabled() {
			next(w, r)
			return
		}

		// Check cookie first
		cookie, err := r.Cookie(cookieName)
		if err == nil && VerifyToken(cookie.Value) {
			next(w, r)
			return
		}

		// Check Authorization header (for API clients)
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if VerifyToken(token) {
				next(w, r)
				return
			}
		}

		// Accept JSON
		if strings.Contains(r.Header.Get("Accept"), "application/json") ||
			strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		// Redirect browser to login
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// HandleLogin processes login form submission.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		serveLoginPage(w)
		return
	}

	if r.Method == "POST" {
		password := r.FormValue("password")
		if password != sharedSecret {
			serveLoginPage(w, "密码错误")
			return
		}
		token := GenerateToken()
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // set true behind HTTPS
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(tokenTTL.Seconds()),
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func serveLoginPage(w http.ResponseWriter, errMsg ...string) {
	msg := ""
	if len(errMsg) > 0 {
		msg = `<p style="color:#EF4444;text-align:center;margin-bottom:16px;">` + errMsg[0] + `</p>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CoreRP — 登录</title>
<link href="https://fonts.googleapis.com/css2?family=Libre+Bodoni:ital,wght@0,600;1,400&family=Public+Sans:wght@400;500&display=swap" rel="stylesheet">
<style>
*{box-sizing:border-box;margin:0;padding:0;}
body{font-family:'Public Sans',system-ui,sans-serif;background:#18181B;color:#F4F4F5;display:flex;align-items:center;justify-content:center;height:100dvh;}
form{width:360px;padding:32px;text-align:center;}
h1{font-family:'Libre Bodoni',serif;font-size:28px;margin-bottom:8px;letter-spacing:0.5px;}
h1 em{color:#EC4899;font-style:italic;}
h2{font-size:12px;color:#71717A;font-weight:400;margin-bottom:32px;text-transform:uppercase;letter-spacing:3px;}
input{width:100%%;padding:12px;background:#27272A;color:#F4F4F5;border:1px solid #3F3F46;font-family:monospace;font-size:14px;outline:none;text-align:center;margin-bottom:16px;}
input:focus{border-color:#EC4899;}
button{width:100%%;padding:12px;background:transparent;color:#A1A1AA;border:1px solid #3F3F46;font-size:12px;font-weight:600;letter-spacing:2px;text-transform:uppercase;cursor:pointer;transition:border-color .2s,color .2s;}
button:hover{border-color:#EC4899;color:#EC4899;}
</style></head>
<body>
<form method="POST">
<h1>Core<em>RP</em></h1>
<h2>Persistent Narrative Runtime</h2>
%s
<input type="password" name="password" placeholder="输入访问密码" autofocus>
<button type="submit">进入</button>
</form>
</body></html>`, msg)
}

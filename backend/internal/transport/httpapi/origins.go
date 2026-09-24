package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"corerp.local/backend/internal/core"
)

// BrowserOriginPolicy grants explicit browser origins access, not principals
// or capabilities. Tokens still go through the normal API authentication.
// No cookies, wildcard origins, proxy trust or private-network grants are added.
func BrowserOriginPolicy(origins []string) (func(http.Handler) http.Handler, error) {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || origin != u.Scheme+"://"+u.Host || strings.ContainsAny(origin, "* \t\r\n") {
			return nil, core.NewError(core.CodeInvalidArgument, "browser origins must be exact HTTP(S) origins without credentials, paths or wildcards")
		}
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			if len(r.Header.Values("Origin")) != 1 || !allowed[origin] || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
				writeError(w, "", core.NewError(core.CodeUnauthorized, "browser origin is not permitted for this API"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			if r.Method != http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			method := r.Header.Get("Access-Control-Request-Method")
			if method != http.MethodGet && method != http.MethodPost {
				writeError(w, "", core.NewError(core.CodeInvalidArgument, "browser preflight method must be GET or POST"))
				return
			}
			for _, value := range r.Header.Values("Access-Control-Request-Headers") {
				for _, name := range strings.Split(value, ",") {
					switch strings.ToLower(strings.TrimSpace(name)) {
					case "authorization", "content-type", "last-event-id":
					default:
						writeError(w, "", core.NewError(core.CodeInvalidArgument, "browser preflight header is not permitted"))
						return
					}
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID")
			w.WriteHeader(http.StatusNoContent)
		})
	}, nil
}

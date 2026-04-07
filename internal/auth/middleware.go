package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// StaticBearerMiddleware protects a route tree with a single shared bearer token.
// When token is empty, the middleware is a no-op so local development can stay open.
func StaticBearerMiddleware(token string) func(http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !matchesBearerToken(r.Header.Get("Authorization"), token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="platform"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func matchesBearerToken(headerValue, expectedToken string) bool {
	token := bearerToken(headerValue)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}

func bearerToken(headerValue string) string {
	parts := strings.Fields(strings.TrimSpace(headerValue))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

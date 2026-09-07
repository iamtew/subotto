package web

import (
	"crypto/subtle"
	"net/http"
)

// basicAuth wraps a handler so the browser must send HTTP Basic credentials.
// Meat Bag: username is always "admin". Password is ADMIN_PASSWORD from .env.
func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || !secureEqual(user, "admin") || !secureEqual(pass, s.password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Subotto Admin"`)
			http.Error(w, "Unauthorized — use username admin and your ADMIN_PASSWORD", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureEqual compares strings in constant time (harder to time-guess passwords).
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		// Still compare something so length leaks less via early return timing.
		_ = subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

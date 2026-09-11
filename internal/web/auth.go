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
		if !ok || !s.adminCredsOK(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Subotto Admin"`)
			http.Error(w, "Unauthorized — use username admin and your ADMIN_PASSWORD", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminOrAPIAuth accepts admin/ADMIN_PASSWORD or api/API_PASSWORD (fire GETs).
// Empty API_PASSWORD means the api user is disabled; admin still works.
func (s *Server) adminOrAPIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || (!s.adminCredsOK(user, pass) && !s.apiCredsOK(user, pass)) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Subotto API"`)
			http.Error(w, "Unauthorized — use admin/ADMIN_PASSWORD or api/API_PASSWORD", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) adminCredsOK(user, pass string) bool {
	return secureEqual(user, "admin") && secureEqual(pass, s.password)
}

func (s *Server) apiCredsOK(user, pass string) bool {
	if s.apiPassword == "" {
		return false
	}
	return secureEqual(user, "api") && secureEqual(pass, s.apiPassword)
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

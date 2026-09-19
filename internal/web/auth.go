package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "subotto_admin"
	sessionTTL        = 7 * 24 * time.Hour
)

type ctxKey int

const adminIdentKey ctxKey = 1

// adminIdent is who passed requireAdmin / adminOrAPIAuth.
type adminIdent struct {
	DiscordID  string
	Superadmin bool
	ViaBasic   bool
	ViaAPI     bool
}

func withAdminIdent(ctx context.Context, id adminIdent) context.Context {
	return context.WithValue(ctx, adminIdentKey, id)
}

func adminIdentFrom(ctx context.Context) (adminIdent, bool) {
	id, ok := ctx.Value(adminIdentKey).(adminIdent)
	return id, ok
}

func (s *Server) basicEnabled() bool {
	return s.password != ""
}

func (s *Server) discordOAuthEnabled() bool {
	return strings.TrimSpace(s.superadminID) != "" &&
		s.discordClientID != "" &&
		s.discordClientSecret != "" &&
		s.discordRedirectURL != ""
}

// requireAdmin: Discord session cookie or admin Basic (if ADMIN_PASSWORD is set).
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.protect(next, false)
}

// adminOrAPIAuth: cookie, admin Basic, or api / API_PASSWORD (broadcast fire).
func (s *Server) adminOrAPIAuth(next http.Handler) http.Handler {
	return s.protect(next, true)
}

func (s *Server) protect(next http.Handler, allowAPI bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ident, ok := s.sessionIdentity(r); ok {
			next.ServeHTTP(w, r.WithContext(withAdminIdent(r.Context(), ident)))
			return
		}
		user, pass, hasBasic := r.BasicAuth()
		if hasBasic && s.adminCredsOK(user, pass) {
			next.ServeHTTP(w, r.WithContext(withAdminIdent(r.Context(), adminIdent{
				Superadmin: true,
				ViaBasic:   true,
			})))
			return
		}
		if allowAPI && hasBasic && s.apiCredsOK(user, pass) {
			next.ServeHTTP(w, r.WithContext(withAdminIdent(r.Context(), adminIdent{ViaAPI: true})))
			return
		}
		s.denyAdmin(w, allowAPI)
	})
}

func (s *Server) denyAdmin(w http.ResponseWriter, allowAPI bool) {
	if s.basicEnabled() {
		realm := "Subotto Admin"
		msg := "Unauthorized — Discord login or username admin and your ADMIN_PASSWORD"
		if allowAPI {
			realm = "Subotto API"
			msg = "Unauthorized — use admin/ADMIN_PASSWORD or api/API_PASSWORD"
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
		http.Error(w, msg, http.StatusUnauthorized)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

func (s *Server) adminCredsOK(user, pass string) bool {
	if !s.basicEnabled() {
		return false
	}
	return secureEqual(user, "admin") && secureEqual(pass, s.password)
}

func (s *Server) apiCredsOK(user, pass string) bool {
	if s.apiPassword == "" {
		return false
	}
	return secureEqual(user, "api") && secureEqual(pass, s.apiPassword)
}

func (s *Server) sessionIdentity(r *http.Request) (adminIdent, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return adminIdent{}, false
	}
	userID, ok := s.parseSessionCookie(c.Value)
	if !ok {
		return adminIdent{}, false
	}
	allowed, super := s.discordUserAllowed(r.Context(), userID)
	if !allowed {
		return adminIdent{}, false
	}
	return adminIdent{DiscordID: userID, Superadmin: super}, true
}

func (s *Server) discordUserAllowed(ctx context.Context, userID string) (allowed bool, super bool) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, false
	}
	if s.superadminID != "" && userID == s.superadminID {
		return true, true
	}
	extra, err := s.store.ExtraAdminDiscordIDs(ctx)
	if err != nil {
		return false, false
	}
	for _, id := range extra {
		if id == userID {
			return true, false
		}
	}
	return false, false
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, userID string) {
	exp := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.signSession(userID, exp.Unix()),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(r),
		Expires:  exp,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(r),
		MaxAge:   -1,
	})
}

func cookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) signSession(userID string, expUnix int64) string {
	exp := strconv.FormatInt(expUnix, 10)
	mac := s.sessionMAC(userID, exp)
	return "v1|" + userID + "|" + exp + "|" + mac
}

func (s *Server) parseSessionCookie(raw string) (string, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) != 4 || parts[0] != "v1" {
		return "", false
	}
	userID, exp, gotMAC := parts[1], parts[2], parts[3]
	if userID == "" || strings.Contains(userID, "|") {
		return "", false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > unix {
		return "", false
	}
	want := s.sessionMAC(userID, exp)
	if subtle.ConstantTimeCompare([]byte(gotMAC), []byte(want)) != 1 {
		return "", false
	}
	return userID, true
}

func (s *Server) sessionMAC(userID, exp string) string {
	mac := hmac.New(sha256.New, []byte(s.discordClientSecret))
	mac.Write([]byte("v1|" + userID + "|" + exp))
	return hex.EncodeToString(mac.Sum(nil))
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		_ = subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

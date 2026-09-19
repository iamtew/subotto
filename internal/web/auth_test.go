package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLandingIsPublic(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "basic") {
		t.Fatalf("expected password login on landing, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "discord") {
		t.Fatalf("did not expect Discord login without superadmin: %s", rec.Body.String())
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate when Basic is on")
	}
}

func TestAdminCookieSession(t *testing.T) {
	s, _ := testServer(t)
	s.superadminID = "42"
	s.discordClientSecret = "cookie-secret"
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	recSet := httptest.NewRecorder()
	s.setSessionCookie(recSet, req, "42")
	for _, c := range recSet.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with cookie, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequireAdminNoWWWAuthenticateWhenBasicOff(t *testing.T) {
	s, _ := testServer(t)
	s.password = ""
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("did not expect WWW-Authenticate: %s", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestExpiredCookieRejected(t *testing.T) {
	s, _ := testServer(t)
	s.superadminID = "42"
	s.discordClientSecret = "cookie-secret"
	raw := s.signSession("42", time.Now().Add(-time.Hour).Unix())
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: raw})
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestDiscordOAuthStartDisabledWithoutSuperadmin(t *testing.T) {
	s, _ := testServer(t)
	s.discordClientID = "cid"
	s.discordClientSecret = "sec"
	s.discordRedirectURL = "http://localhost/auth/discord/callback"
	req := httptest.NewRequest(http.MethodGet, "/auth/discord", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

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
	refreshed := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge > 0 {
			refreshed = true
		}
	}
	if !refreshed {
		t.Fatal("expected same-site response to re-set a persistent admin cookie")
	}
}

func TestLandingRedirectsWhenDiscordCookieValid(t *testing.T) {
	s, _ := testServer(t)
	enableDiscordOAuth(s)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recSet := httptest.NewRecorder()
	s.setSessionCookie(recSet, req, "42")
	for _, c := range recSet.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Fatalf("want /admin, got %s", loc)
	}
}

func TestBasicAuthCookieSurvivesWithoutAuthorization(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 on password login, got %d", rec.Code)
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" && c.MaxAge > 0 {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("password login did not set a persistent admin cookie")
	}

	again := httptest.NewRequest(http.MethodGet, "/admin", nil)
	again.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec2, again)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200 with cookie only, got %d", rec2.Code)
	}

	s.password = ""
	rec3 := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec3, again)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 after Basic is turned off, got %d", rec3.Code)
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

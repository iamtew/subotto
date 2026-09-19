package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func enableDiscordOAuth(s *Server) {
	s.superadminID = "42"
	s.discordClientID = "cid"
	s.discordClientSecret = "sec"
	s.discordRedirectURL = "http://localhost/auth/discord/callback"
}

func TestDiscordOAuthStartRedirects(t *testing.T) {
	s, _ := testServer(t)
	enableDiscordOAuth(s)
	req := httptest.NewRequest(http.MethodGet, "/auth/discord", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "discord.com") || !strings.Contains(loc, "state=") {
		t.Fatalf("want Discord authorize URL, got %s", loc)
	}
	if s.discordOAuthPending.state == "" {
		t.Fatal("expected pending discord oauth state")
	}
}

func TestDiscordCallbackRejectsUnknownUser(t *testing.T) {
	s, _ := testServer(t)
	enableDiscordOAuth(s)
	s.discordExchangeFn = func(_ context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "tok"}, nil
	}
	s.discordMeFn = func(_ context.Context, _ string) (string, error) {
		return "99", nil
	}
	s.discordOAuthPending.state = "st"
	s.discordOAuthPending.until = time.Now().Add(time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=good&state=st", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/?login=forbidden" {
		t.Fatalf("want forbidden, got %s", loc)
	}
	if rec.Result().Cookies() != nil {
		for _, c := range rec.Result().Cookies() {
			if c.Name == sessionCookieName && c.Value != "" && c.MaxAge >= 0 {
				t.Fatalf("did not expect session cookie for unknown user: %+v", c)
			}
		}
	}
}

func TestDiscordCallbackAllowsSuperadmin(t *testing.T) {
	s, _ := testServer(t)
	enableDiscordOAuth(s)
	s.discordExchangeFn = func(_ context.Context, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "tok"}, nil
	}
	s.discordMeFn = func(_ context.Context, _ string) (string, error) {
		return "42", nil
	}
	s.discordOAuthPending.state = "st"
	s.discordOAuthPending.until = time.Now().Add(time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=good&state=st", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Fatalf("want /admin, got %s", loc)
	}
	gotCookie := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			gotCookie = true
		}
	}
	if !gotCookie {
		t.Fatal("expected session cookie")
	}
}

func TestOperatorsPutRequiresSuperadmin(t *testing.T) {
	s, store := testServer(t)
	enableDiscordOAuth(s)
	if err := store.SaveExtraAdminDiscordIDs(context.Background(), []string{"99"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/operators", strings.NewReader(`{"extra_ids":["100"]}`))
	req.Header.Set("Content-Type", "application/json")
	recSet := httptest.NewRecorder()
	s.setSessionCookie(recSet, req, "99")
	for _, c := range recSet.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOperatorsPutByBasicAdmin(t *testing.T) {
	s, _ := testServer(t)
	s.superadminID = "42"
	req := httptest.NewRequest(http.MethodPut, "/api/operators", strings.NewReader(`{"extra_ids":["99"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		ExtraIDs []string `json:"extra_ids"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ExtraIDs) != 1 || out.ExtraIDs[0] != "99" {
		t.Fatalf("unexpected extra_ids: %+v", out.ExtraIDs)
	}
}

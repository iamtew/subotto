package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/youtube"
)

func TestYouTubeAuthStartRequiresAuth(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/youtube/auth", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestYouTubeAuthStartNeedsConfig(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/youtube/auth", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 without oauth config, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestYouTubeAuthStartRedirectsToGoogle(t *testing.T) {
	s, _ := testServer(t)
	s.ytClientID = "cid"
	s.ytClientSecret = "sec"
	s.ytRedirectURL = "https://example.com/oauth/callback"

	req := httptest.NewRequest(http.MethodGet, "/api/youtube/auth", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "accounts.google.com") {
		t.Fatalf("want Google auth URL, got %s", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Fatalf("want state in URL, got %s", loc)
	}
	if s.oauthPending.state == "" {
		t.Fatal("expected pending oauth state")
	}
}

func TestYouTubeCallbackRejectsBadState(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=x&state=nope", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestYouTubeCallbackHappyPath(t *testing.T) {
	s, store := testServer(t)
	s.ytClientID = "cid"
	s.ytClientSecret = "sec"
	s.ytRedirectURL = "https://example.com/oauth/callback"
	s.yt = &youtube.Client{}
	s.exchangeFn = func(_ context.Context, code string) (*oauth2.Token, error) {
		if code != "good" {
			t.Fatalf("unexpected code %q", code)
		}
		return &oauth2.Token{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"}, nil
	}
	s.youtubePing = func(context.Context) (string, error) {
		return "Test Channel", nil
	}
	s.oauthPending.state = "st"
	s.oauthPending.until = time.Now().Add(time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=good&state=st", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin?youtube=ok" {
		t.Fatalf("want /admin?youtube=ok, got %s", loc)
	}
	if s.youtubeName != "Test Channel" {
		t.Fatalf("youtubeName=%q", s.youtubeName)
	}
	raw, err := store.LoadOAuthToken(context.Background(), youtube.TokenKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "rt") {
		t.Fatalf("expected refresh token in db, got %s", raw)
	}
}

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

	"subotto/internal/twitch"
)

func TestTwitchAuthStartRequiresAuth(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/twitch/auth", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestTwitchAuthStartNeedsConfig(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/twitch/auth", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 without oauth config, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTwitchAuthStartRedirects(t *testing.T) {
	s, _ := testServer(t)
	s.twitchClientID = "cid"
	s.twitchClientSecret = "sec"
	s.twitchRedirectURL = "https://example.com/auth/twitch/callback"

	req := httptest.NewRequest(http.MethodGet, "/api/twitch/auth", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "id.twitch.tv") {
		t.Fatalf("want Twitch auth URL, got %s", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Fatalf("want state in URL, got %s", loc)
	}
	if strings.Contains(loc, "chat%3Awrite") || strings.Contains(loc, "chat:write") {
		t.Fatalf("chat:write is not a Twitch scope, got %s", loc)
	}
	if !strings.Contains(loc, "chat%3Aedit") && !strings.Contains(loc, "chat:edit") {
		t.Fatalf("want chat:edit in URL, got %s", loc)
	}
	if s.twitchOAuthPending.state == "" {
		t.Fatal("expected pending oauth state")
	}
}

func TestTwitchCallbackRejectsBadState(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/twitch/callback?code=x&state=nope", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTwitchCallbackHappyPath(t *testing.T) {
	s, store := testServer(t)
	s.twitchClientID = "cid"
	s.twitchClientSecret = "sec"
	s.twitchRedirectURL = "https://example.com/auth/twitch/callback"
	s.twitchExchangeFn = func(_ context.Context, code string) (*oauth2.Token, error) {
		if code != "good" {
			t.Fatalf("unexpected code %q", code)
		}
		return &oauth2.Token{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"}, nil
	}
	s.twitchUserFn = func(_ context.Context, accessToken string) (twitch.User, error) {
		if accessToken != "at" {
			t.Fatalf("token %q", accessToken)
		}
		return twitch.User{Login: "heshbot", DisplayName: "HeshBot"}, nil
	}
	s.twitchOAuthPending.state = "st"
	s.twitchOAuthPending.until = time.Now().Add(time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/auth/twitch/callback?code=good&state=st", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin?twitch=ok" {
		t.Fatalf("want /admin?twitch=ok, got %s", loc)
	}
	raw, err := store.LoadOAuthToken(context.Background(), twitch.TokenKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "rt") {
		t.Fatalf("expected refresh token in db, got %s", raw)
	}
	login, _ := store.TwitchLogin(context.Background())
	if login != "heshbot" {
		t.Fatalf("login %q", login)
	}
}

func TestPostDeleteTwitchChannel(t *testing.T) {
	s, store := testServer(t)
	body := strings.NewReader(`{"channel":"#Cool_Chan"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/twitch/channels", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["channel"] != "cool_chan" {
		t.Fatalf("%v", out)
	}
	list, err := store.TwitchChannels(context.Background())
	if err != nil || len(list) != 1 || list[0] != "cool_chan" {
		t.Fatalf("store %v %v", list, err)
	}

	body = strings.NewReader(`{"channel":"other"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/twitch/channels", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second want 200, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/twitch/channels/nope/messages", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unjoined messages want 404, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/twitch/channels/cool_chan/messages", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("joined messages want 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/twitch/channels/cool_chan", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete want 200, got %d", rec.Code)
	}
	list, err = store.TwitchChannels(context.Background())
	if err != nil || len(list) != 1 || list[0] != "other" {
		t.Fatalf("after delete %v %v", list, err)
	}

	bad := strings.NewReader(`{"channel":"nope!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/twitch/channels", bad)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

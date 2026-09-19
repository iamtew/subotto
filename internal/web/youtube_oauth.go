package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/youtube"
)

const oauthStateTTL = 10 * time.Minute

// handleYouTubeAuthStart sends the Admin browser to Google consent.
// Full navigation (not fetch) so Google owns the tab until it redirects back.
func (s *Server) handleYouTubeAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.ytClientID == "" || s.ytClientSecret == "" || s.ytRedirectURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "YouTube OAuth is not configured (client id/secret/redirect)")
		return
	}
	state, err := randomOAuthState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start YouTube auth")
		return
	}
	s.oauthMu.Lock()
	s.oauthPending.state = state
	s.oauthPending.until = time.Now().Add(oauthStateTTL)
	s.oauthMu.Unlock()

	cfg := youtube.OAuthConfig(s.ytClientID, s.ytClientSecret, s.ytRedirectURL)
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleYouTubeOAuthCallback is public — Google’s redirect has no Basic Auth.
func (s *Server) handleYouTubeOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		_ = s.takeOAuthState(r.URL.Query().Get("state"))
		slog.Warn("youtube oauth denied", "error", errMsg)
		http.Redirect(w, r, "/admin?youtube=denied", http.StatusFound)
		return
	}
	if !s.takeOAuthState(r.URL.Query().Get("state")) {
		http.Error(w, "invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	tok, err := s.exchangeCode(ctx, code)
	if err != nil {
		slog.Error("youtube oauth exchange failed", "err", err)
		http.Redirect(w, r, "/admin?youtube=err", http.StatusFound)
		return
	}
	if err := youtube.SaveToken(ctx, s.store, tok); err != nil {
		slog.Error("youtube oauth save failed", "err", err)
		http.Redirect(w, r, "/admin?youtube=err", http.StatusFound)
		return
	}

	if s.yt == nil {
		s.yt = &youtube.Client{}
	}
	if err := s.yt.Reload(ctx, s.store, s.ytClientID, s.ytClientSecret, s.ytRedirectURL); err != nil {
		slog.Error("youtube client reload failed", "err", err)
		http.Redirect(w, r, "/admin?youtube=err", http.StatusFound)
		return
	}

	title, err := s.pingYouTube(ctx)
	if err != nil {
		slog.Warn("YouTube ping failed after Admin OAuth", "err", err)
		_ = s.store.LogActivity(ctx, "youtube_auth", map[string]any{"ok": false, "error": err.Error()}, false)
		s.youtubeName = ""
		http.Redirect(w, r, "/admin?youtube=err", http.StatusFound)
		return
	}
	s.youtubeName = title
	_ = s.store.LogActivity(ctx, "youtube_auth", map[string]any{"ok": true, "channel": title}, true)
	slog.Info("YouTube authorization OK", "channel", title)
	http.Redirect(w, r, "/admin?youtube=ok", http.StatusFound)
}

func (s *Server) exchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if s.exchangeFn != nil {
		return s.exchangeFn(ctx, code)
	}
	cfg := youtube.OAuthConfig(s.ytClientID, s.ytClientSecret, s.ytRedirectURL)
	return cfg.Exchange(ctx, code)
}

func (s *Server) pingYouTube(ctx context.Context) (string, error) {
	if s.youtubePing != nil {
		return s.youtubePing(ctx)
	}
	return s.yt.Ping(ctx)
}

func (s *Server) takeOAuthState(got string) bool {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	ok := got != "" && s.oauthPending.state == got && time.Now().Before(s.oauthPending.until)
	s.oauthPending.state = ""
	s.oauthPending.until = time.Time{}
	return ok
}

func randomOAuthState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

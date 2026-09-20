package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/twitch"
)

// handleTwitchAuthStart sends the Admin browser to Twitch consent.
func (s *Server) handleTwitchAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.twitchClientID == "" || s.twitchClientSecret == "" || s.twitchRedirectURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "Twitch OAuth is not configured (client id/secret/redirect)")
		return
	}
	state, err := randomOAuthState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start Twitch auth")
		return
	}
	s.oauthMu.Lock()
	s.twitchOAuthPending.state = state
	s.twitchOAuthPending.until = time.Now().Add(oauthStateTTL)
	s.oauthMu.Unlock()

	cfg := twitch.OAuthConfig(s.twitchClientID, s.twitchClientSecret, s.twitchRedirectURL)
	http.Redirect(w, r, twitch.AuthCodeURL(cfg, state), http.StatusFound)
}

// handleTwitchOAuthCallback is public — Twitch’s redirect has no Admin login.
func (s *Server) handleTwitchOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		_ = s.takeTwitchOAuthState(r.URL.Query().Get("state"))
		slog.Warn("twitch oauth denied", "error", errMsg)
		http.Redirect(w, r, "/admin?twitch=denied", http.StatusFound)
		return
	}
	if !s.takeTwitchOAuthState(r.URL.Query().Get("state")) {
		http.Error(w, "invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	tok, err := s.exchangeTwitchCode(ctx, code)
	if err != nil {
		slog.Error("twitch oauth exchange failed", "err", err)
		http.Redirect(w, r, "/admin?twitch=err", http.StatusFound)
		return
	}
	if err := twitch.SaveToken(ctx, s.store, tok); err != nil {
		slog.Error("twitch oauth save failed", "err", err)
		http.Redirect(w, r, "/admin?twitch=err", http.StatusFound)
		return
	}

	user, err := s.fetchTwitchUser(ctx, tok.AccessToken)
	if err != nil {
		slog.Error("twitch helix user failed", "err", err)
		http.Redirect(w, r, "/admin?twitch=err", http.StatusFound)
		return
	}
	if err := s.store.SetTwitchIdentity(ctx, user.Login, user.DisplayName); err != nil {
		slog.Error("twitch identity save failed", "err", err)
		http.Redirect(w, r, "/admin?twitch=err", http.StatusFound)
		return
	}

	if s.twitch != nil {
		if err := s.twitch.ReloadToken(ctx); err != nil {
			slog.Warn("twitch client reload failed", "err", err)
		}
	}

	_ = s.store.LogActivity(ctx, "twitch_auth", map[string]any{"ok": true, "login": user.Login}, true)
	slog.Info("Twitch authorization OK", "login", user.Login)
	http.Redirect(w, r, "/admin?twitch=ok", http.StatusFound)
}

func (s *Server) handlePutTwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	channel, err := twitch.ParseChannel(body.Channel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetTwitchChannel(r.Context(), channel); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.twitch != nil {
		s.twitch.Kick()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "channel": channel})
}

func (s *Server) exchangeTwitchCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if s.twitchExchangeFn != nil {
		return s.twitchExchangeFn(ctx, code)
	}
	cfg := twitch.OAuthConfig(s.twitchClientID, s.twitchClientSecret, s.twitchRedirectURL)
	return cfg.Exchange(ctx, code)
}

func (s *Server) fetchTwitchUser(ctx context.Context, accessToken string) (twitch.User, error) {
	if s.twitchUserFn != nil {
		return s.twitchUserFn(ctx, accessToken)
	}
	return twitch.FetchUser(ctx, s.twitchClientID, accessToken)
}

func (s *Server) takeTwitchOAuthState(got string) bool {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	ok := got != "" && s.twitchOAuthPending.state == got && time.Now().Before(s.twitchOAuthPending.until)
	s.twitchOAuthPending.state = ""
	s.twitchOAuthPending.until = time.Time{}
	return ok
}

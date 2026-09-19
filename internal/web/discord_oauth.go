package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

func discordOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"identify"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	data := struct {
		DiscordOAuth bool
		Basic        bool
		Err          string
	}{
		DiscordOAuth: s.discordOAuthEnabled(),
		Basic:        s.basicEnabled(),
		Err:          landingErr(r.URL.Query().Get("login")),
	}
	path := filepath.Join(s.webroot, "landing.html")
	raw, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "landing page missing", http.StatusInternalServerError)
		return
	}
	tmpl, err := template.New("landing").Parse(string(raw))
	if err != nil {
		http.Error(w, "landing page broken", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Warn("landing render failed", "err", err)
	}
}

func landingErr(code string) string {
	switch code {
	case "denied":
		return "Discord login was cancelled."
	case "forbidden":
		return "That Discord account is not allowed into Admin."
	case "err":
		return "Discord login failed. Try again."
	default:
		return ""
	}
}

func (s *Server) handleDiscordAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.discordOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	state, err := randomOAuthState()
	if err != nil {
		http.Error(w, "could not start Discord login", http.StatusInternalServerError)
		return
	}
	s.oauthMu.Lock()
	s.discordOAuthPending.state = state
	s.discordOAuthPending.until = time.Now().Add(oauthStateTTL)
	s.oauthMu.Unlock()
	cfg := discordOAuthConfig(s.discordClientID, s.discordClientSecret, s.discordRedirectURL)
	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) handleDiscordOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.discordOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		_ = s.takeDiscordOAuthState(r.URL.Query().Get("state"))
		http.Redirect(w, r, "/?login=denied", http.StatusFound)
		return
	}
	if !s.takeDiscordOAuthState(r.URL.Query().Get("state")) {
		http.Error(w, "invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	tok, err := s.exchangeDiscordCode(r.Context(), code)
	if err != nil {
		slog.Error("discord oauth exchange failed", "err", err)
		http.Redirect(w, r, "/?login=err", http.StatusFound)
		return
	}
	userID, err := s.discordMe(r.Context(), tok.AccessToken)
	if err != nil || userID == "" {
		slog.Error("discord oauth identify failed", "err", err)
		http.Redirect(w, r, "/?login=err", http.StatusFound)
		return
	}
	allowed, _ := s.discordUserAllowed(r.Context(), userID)
	if !allowed {
		slog.Info("discord login rejected", "user", userID)
		http.Redirect(w, r, "/?login=forbidden", http.StatusFound)
		return
	}
	s.setSessionCookie(w, r, userID)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) takeDiscordOAuthState(got string) bool {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	ok := got != "" && s.discordOAuthPending.state == got && time.Now().Before(s.discordOAuthPending.until)
	s.discordOAuthPending.state = ""
	s.discordOAuthPending.until = time.Time{}
	return ok
}

func (s *Server) exchangeDiscordCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if s.discordExchangeFn != nil {
		return s.discordExchangeFn(ctx, code)
	}
	cfg := discordOAuthConfig(s.discordClientID, s.discordClientSecret, s.discordRedirectURL)
	return cfg.Exchange(ctx, code)
}

func (s *Server) discordMe(ctx context.Context, accessToken string) (string, error) {
	if s.discordMeFn != nil {
		return s.discordMeFn(ctx, accessToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord identify: HTTP %d", res.StatusCode)
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimSpace(body.ID), nil
}

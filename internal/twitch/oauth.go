// Package twitch chats as a user account over Twitch IRC (Hesh Helper).
//
// Meat Bag: authorize the Twitch account in Admin (Status). Subotto then
// joins one channel you pick — it does not have to be that account’s channel.
package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/db"
)

// TokenKey is the oauth_tokens row for the Twitch user token JSON.
const TokenKey = "twitch"

// IRC scopes. Twitch’s send scope is chat:edit — chat:write is not a real scope.
var Scopes = []string{"chat:read", "chat:edit"}

// Endpoint is Twitch’s OAuth2 URLs.
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://id.twitch.tv/oauth2/authorize",
	TokenURL: "https://id.twitch.tv/oauth2/token",
}

// OAuthConfig builds the Twitch user OAuth2 config.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RedirectURL:  strings.TrimSpace(redirectURL),
		Scopes:       Scopes,
		Endpoint:     Endpoint,
	}
}

// AuthCodeURL is Twitch consent with a forced re-prompt so we get a refresh token.
func AuthCodeURL(cfg *oauth2.Config, state string) string {
	if cfg == nil {
		return ""
	}
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("force_verify", "true"))
}

// HasStoredToken reports whether a Twitch token blob is already in the DB.
func HasStoredToken(ctx context.Context, store *db.DB) (bool, error) {
	raw, err := store.LoadOAuthToken(ctx, TokenKey)
	if err != nil {
		return false, err
	}
	return raw != "", nil
}

// LoadToken reads and unmarshals the stored oauth2.Token from SQLite.
func LoadToken(ctx context.Context, store *db.DB) (*oauth2.Token, error) {
	raw, err := store.LoadOAuthToken(ctx, TokenKey)
	if err != nil {
		return nil, fmt.Errorf("load twitch token: %w", err)
	}
	if raw == "" {
		return nil, ErrNotAuthorized
	}
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return nil, fmt.Errorf("parse twitch token: %w", err)
	}
	return &tok, nil
}

// SaveToken marshals and stores the oauth2.Token in SQLite.
func SaveToken(ctx context.Context, store *db.DB, tok *oauth2.Token) error {
	if tok == nil {
		return errors.New("cannot save nil twitch token")
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal twitch token: %w", err)
	}
	if err := store.SaveOAuthToken(ctx, TokenKey, string(raw)); err != nil {
		return fmt.Errorf("save twitch token: %w", err)
	}
	return nil
}

// ErrNotAuthorized means Meat Bag still needs Admin → Authorize Twitch.
var ErrNotAuthorized = errors.New("twitch not authorized: use Admin → Authorize Twitch")

// User is the Helix identity for the access token.
type User struct {
	Login       string
	DisplayName string
}

type helixUsers struct {
	Data []struct {
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// FetchUser loads login + display name for the current user token.
func FetchUser(ctx context.Context, clientID, accessToken string) (User, error) {
	return fetchUserHTTP(ctx, http.DefaultClient, "https://api.twitch.tv/helix/users", clientID, accessToken)
}

func fetchUserHTTP(ctx context.Context, hc *http.Client, url, clientID, accessToken string) (User, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	clientID = strings.TrimSpace(clientID)
	accessToken = strings.TrimSpace(accessToken)
	if clientID == "" || accessToken == "" {
		return User{}, errors.New("twitch client id and access token are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := hc.Do(req)
	if err != nil {
		return User{}, fmt.Errorf("twitch helix users: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return User{}, fmt.Errorf("twitch helix users HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed helixUsers
	if err := json.Unmarshal(body, &parsed); err != nil {
		return User{}, fmt.Errorf("twitch helix users json: %w", err)
	}
	if len(parsed.Data) == 0 || strings.TrimSpace(parsed.Data[0].Login) == "" {
		return User{}, errors.New("twitch helix users returned no login")
	}
	u := parsed.Data[0]
	display := strings.TrimSpace(u.DisplayName)
	if display == "" {
		display = u.Login
	}
	return User{Login: strings.ToLower(strings.TrimSpace(u.Login)), DisplayName: display}, nil
}

// savingTokenSource persists refreshed tokens back into SQLite.
type savingTokenSource struct {
	src   oauth2.TokenSource
	store *db.DB
	mu    sync.Mutex
	last  string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return tok, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if string(raw) == s.last {
		return tok, nil
	}
	if err := s.store.SaveOAuthToken(context.Background(), TokenKey, string(raw)); err != nil {
		return tok, nil
	}
	s.last = string(raw)
	return tok, nil
}

func newSavingSource(store *db.DB, cfg *oauth2.Config, tok *oauth2.Token) oauth2.TokenSource {
	life := context.Background()
	base := cfg.TokenSource(life, tok)
	return &savingTokenSource{
		src:   oauth2.ReuseTokenSource(tok, base),
		store: store,
	}
}

func waitIdle(ctx context.Context, wake <-chan struct{}, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-wake:
	case <-t.C:
	}
}

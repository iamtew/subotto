// Package youtube talks to the YouTube Data API v3 with OAuth 2.0.
//
// Meat Bag: an API key is NOT enough to add videos to a playlist. You must
// authorize Subotto once with your Google account (the one that owns the
// playlists). After that, a refresh token lives in SQLite and Subotto can
// keep working without you logging in every day.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	ytapi "google.golang.org/api/youtube/v3"

	"subotto/internal/db"
)

// TokenKey is the oauth_tokens row key for the YouTube refresh/access token JSON.
const TokenKey = "youtube"

// Scope lets Subotto manage playlists (insert playlist items, etc.).
// YoutubeForceSsl is the usual write scope for Data API v3.
var Scope = ytapi.YoutubeForceSslScope

// OAuthConfig builds the Google OAuth2 config from Subotto settings.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{Scope},
		Endpoint:     google.Endpoint,
	}
}

// HasStoredToken reports whether a YouTube token blob is already in the DB.
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
		return nil, fmt.Errorf("load youtube token: %w", err)
	}
	if raw == "" {
		return nil, ErrNotAuthorized
	}
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return nil, fmt.Errorf("parse youtube token: %w", err)
	}
	return &tok, nil
}

// SaveToken marshals and stores the oauth2.Token in SQLite.
func SaveToken(ctx context.Context, store *db.DB, tok *oauth2.Token) error {
	if tok == nil {
		return errors.New("cannot save nil youtube token")
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal youtube token: %w", err)
	}
	if err := store.SaveOAuthToken(ctx, TokenKey, string(raw)); err != nil {
		return fmt.Errorf("save youtube token: %w", err)
	}
	return nil
}

// ErrNotAuthorized means Meat Bag still needs to run `just auth-youtube`.
var ErrNotAuthorized = errors.New("youtube not authorized: run `just auth-youtube` once")

// AuthorizeInteractive runs the one-time browser OAuth flow.
//
// Steps for Meat Bag:
//  1. Subotto prints a URL (and tries to open your browser).
//  2. You sign in with the Google account that owns the playlists.
//  3. Google redirects to localhost; Subotto catches the code and saves the token.
func AuthorizeInteractive(ctx context.Context, store *db.DB, clientID, clientSecret, redirectURL string) (*oauth2.Token, error) {
	cfg := OAuthConfig(clientID, clientSecret, redirectURL)

	listenAddr, err := listenAddrFromRedirect(redirectURL)
	if err != nil {
		return nil, err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, "OAuth error: "+errMsg+" "+desc, http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth error from google: %s (%s)", errMsg, desc)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- errors.New("oauth callback missing code")
			return
		}
		fmt.Fprint(w, "<html><body><h1>Subotto</h1><p>YouTube authorization saved. You can close this tab and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s for oauth callback: %w (is another Subotto already running?)", listenAddr, err)
	}
	srv := &http.Server{Handler: mux}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		wg.Wait()
	}()

	// offline + consent forces Google to give us a refresh token (important!).
	authURL := cfg.AuthCodeURL("subotto", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	slog.Info("open this URL to authorize YouTube (Meat Bag, use the Google account that owns your playlists)",
		"url", authURL,
	)
	_ = openBrowser(authURL)

	var code string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case code = <-codeCh:
	case <-time.After(5 * time.Minute):
		return nil, errors.New("timed out waiting for YouTube OAuth callback (5 minutes)")
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange oauth code: %w", err)
	}
	if err := SaveToken(ctx, store, tok); err != nil {
		return nil, err
	}
	slog.Info("YouTube OAuth token saved to the database", "has_refresh_token", tok.RefreshToken != "")
	return tok, nil
}

// listenAddrFromRedirect turns http://localhost:50770/oauth/callback into ":50770".
func listenAddrFromRedirect(redirectURL string) (string, error) {
	u, err := url.Parse(redirectURL)
	if err != nil {
		return "", fmt.Errorf("parse YOUTUBE_REDIRECT_URL: %w", err)
	}
	if u.Path != "" && u.Path != "/oauth/callback" {
		slog.Warn("YOUTUBE_REDIRECT_URL path is not /oauth/callback; callback handler only serves that path",
			"path", u.Path,
		)
	}
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	// Bind on all interfaces of that port so localhost and 127.0.0.1 both work.
	return net.JoinHostPort("", port), nil
}

// openBrowser tries to open the system browser. Failure is OK — Meat Bag can paste the URL.
func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

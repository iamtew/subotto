package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	ytapi "google.golang.org/api/youtube/v3"

	"subotto/internal/db"
)

// Client is a thin wrapper around the official YouTube Data API service.
type Client struct {
	service *ytapi.Service
}

// NewClient builds an authenticated YouTube client using the token in SQLite.
// If no token is stored yet, returns ErrNotAuthorized.
func NewClient(ctx context.Context, store *db.DB, clientID, clientSecret, redirectURL string) (*Client, error) {
	tok, err := LoadToken(ctx, store)
	if err != nil {
		return nil, err
	}

	cfg := OAuthConfig(clientID, clientSecret, redirectURL)
	base := cfg.TokenSource(ctx, tok)
	src := &savingTokenSource{
		src:   oauth2.ReuseTokenSource(tok, base),
		store: store,
	}

	httpClient := oauth2.NewClient(ctx, src)
	svc, err := ytapi.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create youtube service: %w", err)
	}
	return &Client{service: svc}, nil
}

// savingTokenSource persists refreshed tokens back into SQLite.
// Meat Bag: access tokens expire; Google gives a new one using the refresh
// token. We write the updated blob so Subotto keeps working across restarts.
type savingTokenSource struct {
	src   oauth2.TokenSource
	store *db.DB
	mu    sync.Mutex
	last  string // last saved JSON, to avoid needless writes
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return tok, nil // still usable; just skip save
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if string(raw) == s.last {
		return tok, nil
	}
	if err := s.store.SaveOAuthToken(context.Background(), TokenKey, string(raw)); err != nil {
		// Do not fail the API call just because persistence glitched.
		return tok, nil
	}
	s.last = string(raw)
	return tok, nil
}

// AddVideoToPlaylist inserts a video into a playlist (playlistItems.insert).
// Meat Bag: on temporary rate limits (429) or Google hiccups (5xx) we retry a
// couple of times with a short pause. Daily quota exceeded is NOT retried —
// waiting a few seconds will not refill the quota bucket.
func (c *Client) AddVideoToPlaylist(ctx context.Context, playlistID, videoID string) error {
	if c == nil || c.service == nil {
		return errors.New("youtube client is nil")
	}
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" || videoID == "" {
		return errors.New("playlistID and videoID are required")
	}

	item := &ytapi.PlaylistItem{
		Snippet: &ytapi.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &ytapi.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := c.service.PlaylistItems.Insert([]string{"snippet"}, item).Context(ctx).Do()
		if err == nil {
			return nil
		}
		lastErr = wrapAPIError(err, playlistID, videoID)
		if !isRetryableYouTube(err) || attempt == maxAttempts {
			return lastErr
		}
		// 1s, then 2s — enough for a brief rate-limit window.
		wait := time.Duration(attempt) * time.Second
		slog.Warn("youtube temporary error — retrying",
			"attempt", attempt,
			"wait", wait.String(),
			"err", lastErr,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// CreatePlaylist makes a new YouTube playlist owned by the authorized account.
// Meat Bag: use this from the Admin UI when you cannot create an empty playlist
// by hand — Subotto creates it, then stores the returned playlist ID.
// Privacy is "public" so Discord ONLINE notices can link the playlist for everyone.
// This is voluntary association with your own Google account.
func (c *Client) CreatePlaylist(ctx context.Context, title, description string) (playlistID string, err error) {
	if c == nil || c.service == nil {
		return "", errors.New("youtube client is nil")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("playlist title is required")
	}
	if description == "" {
		description = "Created by Subotto"
	}

	pl := &ytapi.Playlist{
		Snippet: &ytapi.PlaylistSnippet{
			Title:       title,
			Description: description,
		},
		Status: &ytapi.PlaylistStatus{
			PrivacyStatus: "public",
		},
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := c.service.Playlists.Insert([]string{"snippet", "status"}, pl).Context(ctx).Do()
		if err == nil {
			if resp == nil || resp.Id == "" {
				return "", errors.New("youtube created playlist but returned no id")
			}
			slog.Info("created youtube playlist", "title", title, "playlist", resp.Id)
			return resp.Id, nil
		}
		lastErr = wrapAPIError(err, "", "")
		if !isRetryableYouTube(err) || attempt == maxAttempts {
			return "", lastErr
		}
		wait := time.Duration(attempt) * time.Second
		slog.Warn("youtube create playlist temporary error — retrying",
			"attempt", attempt,
			"wait", wait.String(),
			"err", lastErr,
		)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

// UpdatePlaylistTitle renames an existing playlist the authorized account owns.
// Meat Bag: your listen epoch can keep the same playlist ID while you fix the title.
func (c *Client) UpdatePlaylistTitle(ctx context.Context, playlistID, title string) error {
	if c == nil || c.service == nil {
		return errors.New("youtube client is nil")
	}
	playlistID = strings.TrimSpace(playlistID)
	title = strings.TrimSpace(title)
	if playlistID == "" || title == "" {
		return errors.New("playlistID and title are required")
	}

	list, err := c.service.Playlists.List([]string{"snippet"}).Id(playlistID).Context(ctx).Do()
	if err != nil {
		return wrapAPIError(err, playlistID, "")
	}
	if list == nil || len(list.Items) == 0 {
		return fmt.Errorf("youtube playlist %q not found or not owned by this account", playlistID)
	}

	pl := list.Items[0]
	pl.Snippet.Title = title

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := c.service.Playlists.Update([]string{"snippet"}, pl).Context(ctx).Do()
		if err == nil {
			slog.Info("renamed youtube playlist", "playlist", playlistID, "title", title)
			return nil
		}
		lastErr = wrapAPIError(err, playlistID, "")
		if !isRetryableYouTube(err) || attempt == maxAttempts {
			return lastErr
		}
		wait := time.Duration(attempt) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// Ping checks that the token works by listing the authorized channel.
// Useful after OAuth so Meat Bag knows authorization succeeded.
func (c *Client) Ping(ctx context.Context) (channelTitle string, err error) {
	resp, err := c.service.Channels.List([]string{"snippet"}).Mine(true).Context(ctx).Do()
	if err != nil {
		return "", wrapAPIError(err, "", "")
	}
	if len(resp.Items) == 0 {
		return "", errors.New("YouTube returned no channels for this account")
	}
	return resp.Items[0].Snippet.Title, nil
}

// wrapAPIError turns Google API errors into plain-English messages.
func wrapAPIError(err error, playlistID, videoID string) error {
	if err == nil {
		return nil
	}

	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		reason := firstReason(gerr)
		switch gerr.Code {
		case 401:
			return fmt.Errorf("youtube auth failed (401): token missing/expired/revoked — run `just auth-youtube` again: %w", err)
		case 403:
			switch reason {
			case "quotaExceeded", "dailyLimitExceeded":
				return fmt.Errorf("youtube quota exceeded (403/%s): wait for the daily reset or request a higher quota in Google Cloud Console: %w", reason, err)
			case "accessNotConfigured":
				return fmt.Errorf("youtube data api is not enabled on your Google Cloud project (403/%s): enable YouTube Data API v3: %w", reason, err)
			case "forbidden", "insufficientPermissions":
				return fmt.Errorf("youtube permission denied (403/%s): re-auth with the account that owns playlist %q: %w", reason, playlistID, err)
			default:
				return fmt.Errorf("youtube forbidden (403/%s) playlist=%q video=%q: %w", reason, playlistID, videoID, err)
			}
		case 404:
			return fmt.Errorf("youtube not found (404): check playlist %q and video %q exist and are accessible: %w", playlistID, videoID, err)
		case 409:
			return fmt.Errorf("youtube conflict (409): video %q may already be on playlist %q: %w", videoID, playlistID, err)
		case 429:
			return fmt.Errorf("youtube rate limited (429): slow down and retry later: %w", err)
		default:
			return fmt.Errorf("youtube api error (%d/%s) playlist=%q video=%q: %w", gerr.Code, reason, playlistID, videoID, err)
		}
	}
	return fmt.Errorf("youtube request failed: %w", err)
}

// isRetryableYouTube is true for transient Google errors worth a short wait.
// Quota / auth / not-found are permanent for this request — do not loop.
func isRetryableYouTube(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	if gerr.Code == 429 {
		return true
	}
	return gerr.Code >= 500 && gerr.Code <= 599
}

func firstReason(gerr *googleapi.Error) string {
	for _, e := range gerr.Errors {
		if e.Reason != "" {
			return e.Reason
		}
	}
	return "unknown"
}

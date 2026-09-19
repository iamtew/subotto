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
// Reload swaps the inner service in place so Discord + scheduler keep the same pointer.
type Client struct {
	mu      sync.Mutex
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
	// oauth2 TokenSource / NewClient die when this ctx is canceled. Admin
	// Reload uses r.Context(); after the callback returns that ctx is dead
	// and the next playlist add fails refreshing the access token.
	life := context.Background()
	base := cfg.TokenSource(life, tok)
	src := &savingTokenSource{
		src:   oauth2.ReuseTokenSource(tok, base),
		store: store,
	}

	httpClient := oauth2.NewClient(life, src)
	svc, err := ytapi.NewService(life, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create youtube service: %w", err)
	}
	return &Client{service: svc}, nil
}

// Reload rebuilds the API service from the token currently in SQLite.
// Meat Bag: Admin OAuth writes a new refresh token, then this picks it up
// without restarting Subotto. Empty Client (boot with no token yet) is OK.
func (c *Client) Reload(ctx context.Context, store *db.DB, clientID, clientSecret, redirectURL string) error {
	if c == nil {
		return errors.New("youtube client is nil")
	}
	fresh, err := NewClient(ctx, store, clientID, clientSecret, redirectURL)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.service = fresh.service
	c.mu.Unlock()
	return nil
}

// svc snapshots the current Google service pointer.
func (c *Client) svc() (*ytapi.Service, error) {
	if c == nil {
		return nil, errors.New("youtube client is nil")
	}
	c.mu.Lock()
	svc := c.service
	c.mu.Unlock()
	if svc == nil {
		return nil, errors.New("youtube client is nil")
	}
	return svc, nil
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
	svc, err := c.svc()
	if err != nil {
		return err
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
		_, err := svc.PlaylistItems.Insert([]string{"snippet"}, item).Context(ctx).Do()
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

// RemoveVideoFromPlaylist deletes the playlist item for videoID (playlistItems.list + delete).
func (c *Client) RemoveVideoFromPlaylist(ctx context.Context, playlistID, videoID string) error {
	svc, err := c.svc()
	if err != nil {
		return err
	}
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" || videoID == "" {
		return errors.New("playlistID and videoID are required")
	}

	itemID, err := findPlaylistItemID(ctx, svc, playlistID, videoID)
	if err != nil {
		return wrapAPIError(err, playlistID, videoID)
	}
	if itemID == "" {
		return nil // already gone
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := svc.PlaylistItems.Delete(itemID).Context(ctx).Do()
		if err == nil {
			return nil
		}
		lastErr = wrapAPIError(err, playlistID, videoID)
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

func findPlaylistItemID(ctx context.Context, svc *ytapi.Service, playlistID, videoID string) (string, error) {
	token := ""
	for {
		call := svc.PlaylistItems.List([]string{"snippet"}).PlaylistId(playlistID).MaxResults(50).Context(ctx)
		if token != "" {
			call = call.PageToken(token)
		}
		resp, err := call.Do()
		if err != nil {
			return "", err
		}
		for _, it := range resp.Items {
			if it == nil || it.Snippet == nil || it.Snippet.ResourceId == nil {
				continue
			}
			if it.Snippet.ResourceId.VideoId == videoID {
				return it.Id, nil
			}
		}
		if resp.NextPageToken == "" {
			return "", nil
		}
		token = resp.NextPageToken
	}
}

// CreatePlaylist makes a new YouTube playlist owned by the authorized account.
// Meat Bag: use this from the Admin UI when you cannot create an empty playlist
// by hand — Subotto creates it, then stores the returned playlist ID.
// Privacy is "public" so Discord ONLINE notices can link the playlist for everyone.
// This is voluntary association with your own Google account.
func (c *Client) CreatePlaylist(ctx context.Context, title, description string) (playlistID string, err error) {
	svc, err := c.svc()
	if err != nil {
		return "", err
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
		resp, err := svc.Playlists.Insert([]string{"snippet", "status"}, pl).Context(ctx).Do()
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
	svc, err := c.svc()
	if err != nil {
		return err
	}
	playlistID = strings.TrimSpace(playlistID)
	title = strings.TrimSpace(title)
	if playlistID == "" || title == "" {
		return errors.New("playlistID and title are required")
	}

	list, err := svc.Playlists.List([]string{"snippet"}).Id(playlistID).Context(ctx).Do()
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
		_, err := svc.Playlists.Update([]string{"snippet"}, pl).Context(ctx).Do()
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
	svc, err := c.svc()
	if err != nil {
		return "", err
	}
	resp, err := svc.Channels.List([]string{"snippet"}).Mine(true).Context(ctx).Do()
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
			return fmt.Errorf("youtube auth failed (401): token missing/expired/revoked — re-auth in Admin (Status → Authorize YouTube): %w", err)
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

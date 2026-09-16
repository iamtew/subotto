package youtube

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/db"
)

func TestNewClientSurvivesCanceledSetupContext(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "yt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tok := &oauth2.Token{
		AccessToken:  "expired-at",
		RefreshToken: "rt",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}
	if err := SaveToken(context.Background(), store, tok); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := NewClient(ctx, store, "cid", "sec", "http://localhost/oauth/callback")
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	err = client.AddVideoToPlaylist(context.Background(), "PLtest", "dQw4w9WgXcQ")
	if err == nil {
		t.Fatal("expected YouTube/token error with fake credentials")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("construction ctx leaked into token refresh: %v", err)
	}
}

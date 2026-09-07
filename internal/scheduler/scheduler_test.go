package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/youtube"
)

func TestRunNoOpWhenIntervalZero(t *testing.T) {
	s := New(Options{ResyncIntervalHours: 0})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// good — returned immediately without waiting on ctx
	case <-time.After(500 * time.Millisecond):
		t.Fatal("disabled scheduler should return immediately")
	}

	info := s.Info()
	if info.Enabled {
		t.Fatal("expected Enabled false")
	}
	if info.IntervalHours != 0 {
		t.Fatalf("interval=%d", info.IntervalHours)
	}
}

func TestRunOnceSkipsDisabledMappings(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if _, err := store.UpsertMapping(ctx, "chan-on", "", "PLon", "on", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertMapping(ctx, "chan-off", "", "PLoff", "off", false); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var called []string
	s := New(Options{
		Store:               store,
		DiscordToken:        "fake-token",
		ResyncIntervalHours: 1,
		ResyncLimit:         50,
	})
	s.resync = func(
		_ context.Context,
		_ string,
		_ *db.DB,
		_ *youtube.Client,
		channelID string,
		limit int,
	) (*discord.ResyncSummary, error) {
		mu.Lock()
		called = append(called, channelID)
		mu.Unlock()
		if limit != 50 {
			t.Errorf("want limit 50, got %d", limit)
		}
		return &discord.ResyncSummary{}, nil
	}

	s.runOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(called) != 1 || called[0] != "chan-on" {
		t.Fatalf("expected only enabled channel, got %v", called)
	}

	info := s.Info()
	if !info.Enabled || info.IntervalHours != 1 {
		t.Fatalf("bad info: %+v", info)
	}
	if info.LastRunAt == "" {
		t.Fatal("expected last_run_at set")
	}
	if info.LastError != "" {
		t.Fatalf("unexpected last error: %s", info.LastError)
	}
}

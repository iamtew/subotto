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

func TestRunIdleUntilCancelWhenIntervalZero(t *testing.T) {
	s := New(Options{ResyncIntervalHours: 0})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("idle scheduler should wait, not return immediately")
	case <-time.After(150 * time.Millisecond):
	}

	info := s.Info()
	if info.Enabled {
		t.Fatal("expected Enabled false")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected stop after cancel")
	}
}

func TestReloadWakesIdleLoop(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	s := New(Options{Store: store, ResyncIntervalHours: 0})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	s.Reload()
	select {
	case <-done:
		t.Fatal("reload should not stop the loop")
	case <-time.After(100 * time.Millisecond):
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
	if _, err := store.SaveResyncScheduler(ctx, db.ResyncScheduler{
		IntervalHours: 1,
		Limit:         50,
		Scope:         db.ResyncScopeAll,
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var called []string
	s := New(Options{
		Store:               store,
		DiscordToken:        "fake-token",
		ResyncIntervalHours: 1,
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
		called = append(called, "content:"+channelID)
		mu.Unlock()
		if limit != 50 {
			t.Errorf("want limit 50, got %d", limit)
		}
		return &discord.ResyncSummary{}, nil
	}

	s.runOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(called) != 1 || called[0] != "content:chan-on" {
		t.Fatalf("expected only enabled content channel, got %v", called)
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

func TestRunOnceSelectedAndPictures(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if _, err := store.UpsertMapping(ctx, "c1", "", "PL1", "one", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertMapping(ctx, "c2", "", "PL2", "two", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: "c1",
		Name:             "pics",
		Enabled:          true,
		IntervalSeconds:  8,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveResyncScheduler(ctx, db.ResyncScheduler{
		IntervalHours: 1,
		Limit:         10,
		Scope:         db.ResyncScopeSelected,
		Targets: []db.ResyncTarget{
			{Kind: db.ResyncKindContent, ChannelID: "c1"},
			{Kind: db.ResyncKindPicture, ChannelID: "c1"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var called []string
	s := New(Options{Store: store, DiscordToken: "fake"})
	s.resync = func(_ context.Context, _ string, _ *db.DB, _ *youtube.Client, channelID string, _ int) (*discord.ResyncSummary, error) {
		mu.Lock()
		called = append(called, "content:"+channelID)
		mu.Unlock()
		return &discord.ResyncSummary{}, nil
	}
	s.picture = func(_ context.Context, _ string, _ *db.DB, channelID string, _ int) (*discord.PictureResyncSummary, error) {
		mu.Lock()
		called = append(called, "picture:"+channelID)
		mu.Unlock()
		return &discord.PictureResyncSummary{}, nil
	}

	s.runOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	want := map[string]bool{"content:c1": true, "picture:c1": true}
	if len(called) != 2 {
		t.Fatalf("got %v", called)
	}
	for _, c := range called {
		if !want[c] {
			t.Fatalf("unexpected %s in %v", c, called)
		}
	}
}

func TestRunOnceAllIncludesPictures(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if _, err := store.UpsertMapping(ctx, "c1", "", "PL1", "one", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: "p1",
		Name:             "pics",
		Enabled:          true,
		IntervalSeconds:  8,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveResyncScheduler(ctx, db.ResyncScheduler{
		IntervalHours: 1,
		Limit:         10,
		Scope:         db.ResyncScopeAll,
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var called []string
	s := New(Options{Store: store})
	s.resync = func(_ context.Context, _ string, _ *db.DB, _ *youtube.Client, channelID string, _ int) (*discord.ResyncSummary, error) {
		mu.Lock()
		called = append(called, "content:"+channelID)
		mu.Unlock()
		return &discord.ResyncSummary{}, nil
	}
	s.picture = func(_ context.Context, _ string, _ *db.DB, channelID string, _ int) (*discord.PictureResyncSummary, error) {
		mu.Lock()
		called = append(called, "picture:"+channelID)
		mu.Unlock()
		return &discord.PictureResyncSummary{}, nil
	}

	s.runOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(called) != 2 {
		t.Fatalf("want content+picture, got %v", called)
	}
}

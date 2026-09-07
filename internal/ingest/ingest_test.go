package ingest

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"subotto/internal/db"
)

// fakeAdder pretends to be YouTube so tests do not need real API keys.
type fakeAdder struct {
	// failVideo, if set, makes AddVideoToPlaylist return an error for that id.
	failVideo string
	calls     []string
}

func (f *fakeAdder) AddVideoToPlaylist(_ context.Context, _, videoID string) error {
	f.calls = append(f.calls, videoID)
	if f.failVideo != "" && videoID == f.failVideo {
		return errors.New("fake youtube boom")
	}
	return nil
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestProcessContentAddAndDuplicate(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)
	mapping, err := store.UpsertMapping(ctx, "c1", "g1", "pl1", "test", true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	yt := &fakeAdder{}
	content := "check this https://www.youtube.com/watch?v=dQw4w9WgXcQ please"

	r1 := ProcessContent(ctx, store, yt, mapping, "c1", "msg-1", content)
	if r1.Added != 1 || r1.Skipped != 0 || r1.Failed != 0 {
		t.Fatalf("first pass: want added=1, got %+v", r1)
	}
	if len(yt.calls) != 1 || yt.calls[0] != "dQw4w9WgXcQ" {
		t.Fatalf("unexpected youtube calls: %v", yt.calls)
	}

	r2 := ProcessContent(ctx, store, yt, mapping, "c1", "msg-2", content)
	if r2.Added != 0 || r2.Skipped != 1 || r2.Failed != 0 {
		t.Fatalf("second pass: want skipped=1, got %+v", r2)
	}
	if len(yt.calls) != 1 {
		t.Fatalf("duplicate must not call YouTube again, calls=%v", yt.calls)
	}
}

func TestProcessContentAddFail(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)
	mapping, err := store.UpsertMapping(ctx, "c1", "g1", "pl1", "test", true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	yt := &fakeAdder{failVideo: "dQw4w9WgXcQ"}
	content := "https://youtu.be/dQw4w9WgXcQ"

	r := ProcessContent(ctx, store, yt, mapping, "c1", "msg-1", content)
	if r.Failed != 1 || r.Added != 0 {
		t.Fatalf("want failed=1, got %+v", r)
	}

	// Failed adds must NOT be marked processed — a retry should try again.
	already, err := store.WasVideoProcessed(ctx, "dQw4w9WgXcQ", "pl1")
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}
	if already {
		t.Fatal("failed add should not mark video as processed")
	}
}

func TestProcessContentNoLinks(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)
	mapping := &db.ChannelMapping{YouTubePlaylistID: "pl1", Name: "x"}
	yt := &fakeAdder{}

	r := ProcessContent(ctx, store, yt, mapping, "c1", "msg-1", "hello meat bag")
	if r.Added != 0 || r.Skipped != 0 || r.Failed != 0 || len(yt.calls) != 0 {
		t.Fatalf("expected empty result, got %+v calls=%v", r, yt.calls)
	}
}

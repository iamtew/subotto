package pictures

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"subotto/internal/db"
)

func TestIsImageContentType(t *testing.T) {
	if !IsImageContentType("image/png") || !IsImageContentType("image/jpeg; charset=binary") {
		t.Fatal("expected image types")
	}
	if IsImageContentType("application/pdf") {
		t.Fatal("pdf is not an image")
	}
}

func TestProcessAttachments(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	pl, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: "c1",
		Name:             "test_show",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("listener: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	}))
	t.Cleanup(srv.Close)

	res := ProcessAttachments(ctx, store, pl, "m1", "u1", "Alice", []Attachment{{
		ID:          "a1",
		URL:         srv.URL + "/pic.png",
		Filename:    "pic.png",
		ContentType: "image/png",
	}}, map[string]int{"👍": 1})
	if res.Saved != 1 || res.Failed != 0 {
		t.Fatalf("first save: %+v", res)
	}

	abs := filepath.Join(store.PicturesDir(), pl.Slug, "m1_a1.png")
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("file missing: %v", err)
	}

	res2 := ProcessAttachments(ctx, store, pl, "m1", "u1", "Alice", []Attachment{{
		ID:          "a1",
		URL:         srv.URL + "/pic.png",
		Filename:    "pic.png",
		ContentType: "image/png",
	}}, nil)
	if res2.Skipped != 1 {
		t.Fatalf("want skip dupe: %+v", res2)
	}
}

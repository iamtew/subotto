package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBroadcastCRUD(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	tmpl, err := store.CreateEpisodeTemplate(ctx, "Sesh Sofa", "LIVE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tid := tmpl.ID

	b, err := store.CreateBroadcast(ctx, "Cold Open", "cold-open", &tid, []BroadcastMessage{
		{Body: "Hello {{show}} EP{{episode}}", ChannelIDs: []string{"111", "111", "222"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Slug != "cold-open" || b.TemplateShow != "Sesh Sofa" || len(b.Messages) != 1 {
		t.Fatalf("create: %+v", b)
	}
	if len(b.Messages[0].ChannelIDs) != 2 {
		t.Fatalf("dedupe channels: %+v", b.Messages[0].ChannelIDs)
	}
	if b.Messages[0].ID == "" {
		t.Fatal("expected message id")
	}

	got, err := store.GetBroadcastBySlug(ctx, "COLD-OPEN")
	if err != nil || got == nil || got.ID != b.ID {
		t.Fatalf("by slug: %+v err=%v", got, err)
	}

	standalone, err := store.CreateBroadcast(ctx, "Promo", "promo", nil, []BroadcastMessage{
		{Body: "Watch now", ChannelIDs: []string{"333"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.EpisodeTemplateID != nil {
		t.Fatalf("standalone should be nil template: %+v", standalone)
	}

	list, err := store.ListBroadcasts(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}

	if _, err := store.CreateBroadcast(ctx, "Dup", "promo", nil, []BroadcastMessage{
		{Body: "x", ChannelIDs: []string{"1"}},
	}); err == nil {
		t.Fatal("expected duplicate slug error")
	}

	if _, err := store.CreateBroadcast(ctx, "Bad", "bad", nil, nil); err == nil {
		t.Fatal("expected empty messages error")
	}

	if err := store.DeleteBroadcast(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	gone, err := store.GetBroadcastByID(ctx, b.ID)
	if err != nil || gone != nil {
		t.Fatalf("deleted: %+v err=%v", gone, err)
	}
}

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":     "hello_world",
		"Show #42!":       "show_42",
		"  already_slug ": "already_slug",
		"latest":          "latest",
		"":                "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Fatalf("Slugify(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPictureListenerCRUD(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	p, err := store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pic-1",
		GuildID:          "guild-1",
		Name:             "Friday Pics",
		Enabled:          true,
		CreditCorner:     "tl",
		IntervalSeconds:  5,
		Shuffle:          true,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if p.Slug != "friday_pics" || p.CreditCorner != "tl" || !p.Shuffle {
		t.Fatalf("unexpected upsert: %+v", p)
	}

	dir := filepath.Join(store.PicturesDir(), p.Slug)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("expected pictures dir %s: %v", dir, err)
	}

	// Same slug → in-place update.
	p2, err := store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pic-1",
		GuildID:          "guild-1",
		Name:             "Friday Pics",
		Slug:             "friday_pics",
		Enabled:          true,
		CreditCorner:     "br",
		IntervalSeconds:  12,
		ShowCredit:       false,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("update in place: %v", err)
	}
	if p2.ID != p.ID || p2.CreditCorner != "br" || p2.IntervalSeconds != 12 || p2.ShowCredit {
		t.Fatalf("in-place update failed: %+v", p2)
	}

	// New slug on same channel → close epoch, open new.
	p3, err := store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pic-1",
		GuildID:          "guild-1",
		Name:             "Saturday Pics",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("new epoch: %v", err)
	}
	if p3.ID == p.ID || p3.Slug != "saturday_pics" {
		t.Fatalf("expected new epoch: old=%d new=%+v", p.ID, p3)
	}

	list, err := store.ListPictureListeners(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 active, got %d", len(list))
	}

	// Slug collision across channels.
	_, err = store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pic-2",
		GuildID:          "guild-1",
		Name:             "Saturday Pics",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err == nil {
		t.Fatal("expected slug collision error")
	}

	latest, err := store.GetLatestPictureListener(ctx)
	if err != nil || latest == nil || latest.Slug != "saturday_pics" {
		t.Fatalf("latest: %+v err=%v", latest, err)
	}

	bySlug, err := store.GetPictureListenerBySlug(ctx, "saturday_pics")
	if err != nil || bySlug == nil {
		t.Fatalf("by slug: %v", err)
	}

	if err := store.DeletePictureListener(ctx, "chan-pic-1"); err != nil {
		t.Fatalf("cease: %v", err)
	}
	gone, err := store.GetPictureListenerByChannel(ctx, "chan-pic-1")
	if err != nil || gone != nil {
		t.Fatalf("expected ceased: %+v err=%v", gone, err)
	}

	// Reserved slug.
	_, err = store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-x",
		Name:             "latest",
		Enabled:          true,
	})
	if err == nil {
		t.Fatal("expected reserved slug error")
	}
}

func TestCollectedPictures(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	pl, err := store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-c",
		Name:             "Collect Me",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	pic, err := store.InsertCollectedPicture(ctx, CollectedPicture{
		ListenerID:          pl.ID,
		DiscordMessageID:    "msg-1",
		DiscordAttachmentID: "att-1",
		AuthorID:            "user-1",
		AuthorDisplayName:   "MeatBag",
		StoredPath:          pl.Slug + "/msg-1_att-1.jpg",
		ContentType:         "image/jpeg",
		Reactions:           map[string]int{"🔥": 2},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if pic.ID == 0 || pic.Reactions["🔥"] != 2 {
		t.Fatalf("bad insert: %+v", pic)
	}

	ok, err := store.HasCollectedAttachment(ctx, pl.ID, "att-1")
	if err != nil || !ok {
		t.Fatalf("has attachment: ok=%v err=%v", ok, err)
	}

	prev, found, err := store.CollectedAttachmentOnChannel(ctx, "chan-c", "att-1")
	if err != nil || !found || prev.ListenerID != pl.ID || prev.MessageID != "msg-1" {
		t.Fatalf("channel lookup: %+v found=%v err=%v", prev, found, err)
	}
	_, found, err = store.CollectedAttachmentOnChannel(ctx, "chan-c", "missing")
	if err != nil || found {
		t.Fatalf("missing attachment should be absent: found=%v err=%v", found, err)
	}

	if err := store.UpdatePictureReactions(ctx, "chan-c", "msg-1", map[string]int{"🔥": 3, "❤️": 1}); err != nil {
		t.Fatalf("update reactions: %v", err)
	}
	list, err := store.ListCollectedPicturesForListener(ctx, pl.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if list[0].Reactions["🔥"] != 3 || list[0].Reactions["❤️"] != 1 {
		t.Fatalf("reactions not updated: %+v", list[0].Reactions)
	}

	abs, err := store.AbsolutePicturePath(list[0].StoredPath)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("want abs path, got %s", abs)
	}
	if _, err := store.AbsolutePicturePath("../etc/passwd"); err == nil {
		t.Fatal("expected traversal reject")
	}
}

func TestContentAndPictureSameChannel(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	_, err := store.UpsertMapping(ctx, "shared", "g", "pl-1", "content", true)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	_, err = store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "shared",
		GuildID:          "g",
		Name:             "pics",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("picture on same channel: %v", err)
	}

	c, err := store.GetEnabledMappingByChannel(ctx, "shared")
	if err != nil || c == nil {
		t.Fatalf("content still live: %v", err)
	}
	p, err := store.GetEnabledPictureListenerByChannel(ctx, "shared")
	if err != nil || p == nil {
		t.Fatalf("picture still live: %v", err)
	}
}

func TestPictureResyncNotBefore(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	nb, err := store.PictureResyncNotBefore(ctx, "chan-pr")
	if err != nil || !nb.IsZero() {
		t.Fatalf("first epoch should have zero floor: %v %v", nb, err)
	}

	_, err = store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pr",
		Name:             "epoch_one",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	if err := store.DeletePictureListener(ctx, "chan-pr"); err != nil {
		t.Fatalf("cease: %v", err)
	}
	_, err = store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "chan-pr",
		Name:             "epoch_two",
		Enabled:          true,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		t.Fatalf("start 2: %v", err)
	}

	nb2, err := store.PictureResyncNotBefore(ctx, "chan-pr")
	if err != nil || nb2.IsZero() {
		t.Fatalf("expected epoch floor after prior cease: %v err=%v", nb2, err)
	}
}

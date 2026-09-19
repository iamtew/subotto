package discord

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
)

type fakeRemover struct {
	calls []string
}

func (f *fakeRemover) RemoveVideoFromPlaylist(_ context.Context, playlistID, videoID string) error {
	f.calls = append(f.calls, playlistID+":"+videoID)
	return nil
}

func TestSkipVideosOnMessage(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "skip.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.MarkVideoProcessed(ctx, "vid1", "pl1", "ch1", "msg1"); err != nil {
		t.Fatal(err)
	}
	yt := &fakeRemover{}
	n, err := skipVideosOnMessage(ctx, store, yt, "ch1", "msg1")
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(yt.calls) != 1 || yt.calls[0] != "pl1:vid1" {
		t.Fatalf("calls %v", yt.calls)
	}
	rows, err := store.ListProcessedVideosByMessage(ctx, "ch1", "msg1")
	if err != nil || len(rows) != 1 || !rows[0].Skipped {
		t.Fatalf("rows %+v err=%v", rows, err)
	}
	n, err = skipVideosOnMessage(ctx, store, yt, "ch1", "msg1")
	if err != nil || n != 0 {
		t.Fatalf("second skip n=%d err=%v", n, err)
	}
}

func TestHidePicturesByMessage(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "hide.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pl, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: "ch1",
		GuildID:          "g1",
		Name:             "pics",
		Enabled:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertCollectedPicture(ctx, db.CollectedPicture{
		ListenerID:          pl.ID,
		DiscordMessageID:    "msg1",
		DiscordAttachmentID: "a1",
		StoredPath:          "pics/a1.jpg",
		ContentType:         "image/jpeg",
	}); err != nil {
		t.Fatal(err)
	}
	n, err := store.SetCollectedPicturesIgnoredByMessage(ctx, "ch1", "msg1", true)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	pics, err := store.ListCollectedPicturesByMessage(ctx, "ch1", "msg1")
	if err != nil || len(pics) != 1 || !pics[0].Ignored {
		t.Fatalf("pics %+v err=%v", pics, err)
	}
}

func TestIsSuperadminSkipReact(t *testing.T) {
	b := &Bot{superadminID: "42"}
	if !b.isSuperadminSkipReact(reactAdd("42", "❌")) {
		t.Fatal("superadmin ❌ should command")
	}
	if b.isSuperadminSkipReact(reactAdd("99", "❌")) {
		t.Fatal("other user")
	}
	if b.isSuperadminSkipReact(reactAdd("42", "💾")) {
		t.Fatal("wrong emoji")
	}
}

func reactAdd(user, emoji string) *discordgo.MessageReactionAdd {
	return &discordgo.MessageReactionAdd{
		MessageReaction: &discordgo.MessageReaction{
			UserID: user,
			Emoji:  discordgo.Emoji{Name: emoji},
		},
	}
}

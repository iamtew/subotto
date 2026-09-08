package discord

import (
	"slices"
	"testing"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/ingest"
	"subotto/internal/pictures"
)

func TestStatusChromeEmojis(t *testing.T) {
	if got := statusChromeEmojis(0, 0, 0, 0, 0, 0); got != nil {
		t.Fatalf("idle message should not stamp: %v", got)
	}
	if !slices.Equal(statusChromeEmojis(0, 1, 0, 0, 0, 0), chromeSaved) {
		t.Fatal("fresh save should be 💾")
	}
	if !slices.Equal(statusChromeEmojis(0, 0, 1, 0, 1, 1), chromeSaved) {
		t.Fatal("resync of the original post should be 💾, not DUPE")
	}
	if !slices.Equal(statusChromeEmojis(1, 0, 0, 0, 0, 0), chromeFailed) {
		t.Fatal("failed should be ❌")
	}
	if !slices.Equal(statusChromeEmojis(0, 0, 0, 1, 0, 1), chromeOld) {
		t.Fatal("previous epoch should be OLD")
	}
	if !slices.Equal(statusChromeEmojis(0, 0, 0, 0, 1, 1), chromeDupe) {
		t.Fatal("same listener, other message should be DUPE")
	}
}

func TestContentAndPictureChromeSharePolicy(t *testing.T) {
	saved := contentStatusChrome(ingest.Result{Added: 1})
	pic := pictureStatusChrome(pictures.Result{Saved: 1})
	if !slices.Equal(saved, pic) || !slices.Equal(saved, chromeSaved) {
		t.Fatalf("content and picture saved chrome must match: %v vs %v", saved, pic)
	}
	if contentStatusChrome(ingest.Result{}) != nil {
		t.Fatal("no links → no chrome")
	}
	if pictureStatusChrome(pictures.Result{}) != nil {
		t.Fatal("no images → no chrome")
	}
}

func TestMissingStatusEmojisSkipsPresent(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
		},
	}
	if got := missingStatusEmojis(msg, chromeSaved); len(got) != 0 {
		t.Fatalf("already stamped 💾: %v", got)
	}
	if got := missingStatusEmojis(nil, chromeSaved); !slices.Equal(got, chromeSaved) {
		t.Fatalf("nil message should need full chrome: %v", got)
	}
}

func TestMissingStatusEmojisPartialDupe(t *testing.T) {
	// Discord often stores recycle without the FE0F variation selector.
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "♻"}},
		},
	}
	got := missingStatusEmojis(msg, chromeDupe)
	want := []string{"🇩", "🇺", "🇵", "🇪"}
	if !slices.Equal(got, want) {
		t.Fatalf("should skip recycle, still stamp letters: got %v want %v", got, want)
	}
}

func TestMissingStatusEmojisIgnoresHumanReacts(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: false, Emoji: &discordgo.Emoji{Name: "💾"}},
		},
	}
	if got := missingStatusEmojis(msg, chromeSaved); !slices.Equal(got, chromeSaved) {
		t.Fatalf("human 💾 is not Subotto chrome: %v", got)
	}
}

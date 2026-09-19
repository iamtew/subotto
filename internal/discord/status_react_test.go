package discord

import (
	"slices"
	"testing"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
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

func TestPictureIgnoreChrome(t *testing.T) {
	saved := []db.CollectedPicture{{ID: 1}}
	if !slices.Equal(PictureIgnoreChrome(saved), chromeSaved) {
		t.Fatalf("one saved: %v", PictureIgnoreChrome(saved))
	}
	ignored := []db.CollectedPicture{{ID: 1, Ignored: true}}
	if !slices.Equal(PictureIgnoreChrome(ignored), chromeFailed) {
		t.Fatalf("one ignored: %v", PictureIgnoreChrome(ignored))
	}
	got := PictureIgnoreChrome([]db.CollectedPicture{
		{ID: 1},
		{ID: 2, Ignored: true},
	})
	want := []string{"💾", "❌", "2️⃣"}
	if !slices.Equal(got, want) {
		t.Fatalf("two pics ignore second: got %v want %v", got, want)
	}
	if PictureIgnoreChrome(nil) != nil {
		t.Fatal("empty should be nil")
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

func TestExtraManagedChromeSwapsSavedAndFailed(t *testing.T) {
	failed := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "❌"}},
		},
	}
	if got := extraManagedChrome(failed, chromeSaved); !slices.Equal(got, []string{"❌"}) {
		t.Fatalf("💾 should drop previous ❌: %v", got)
	}
	saved := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
		},
	}
	if got := extraManagedChrome(saved, chromeFailed); !slices.Equal(got, []string{"💾"}) {
		t.Fatalf("❌ should drop previous 💾: %v", got)
	}
}

func TestExtraManagedChromeDropsStaleDupe(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
			{Me: true, Emoji: &discordgo.Emoji{Name: "♻️"}},
			{Me: true, Emoji: &discordgo.Emoji{Name: "🇩"}},
		},
	}
	got := extraManagedChrome(msg, chromeFailed)
	want := []string{"💾", "♻️", "🇩"}
	if !slices.Equal(got, want) {
		t.Fatalf("status chrome is exclusive: got %v want %v", got, want)
	}
}

func TestExtraManagedChromeSkipDropsFloppy(t *testing.T) {
	saved := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
		},
	}
	if got := extraManagedChrome(saved, chromeSkip); !slices.Equal(got, []string{"💾"}) {
		t.Fatalf("SKIP should drop 💾: %v", got)
	}
}

func TestMessageHasSubottoChrome(t *testing.T) {
	if messageHasSubottoChrome(nil) {
		t.Fatal("nil")
	}
	human := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: false, Emoji: &discordgo.Emoji{Name: "❌"}},
		},
	}
	if messageHasSubottoChrome(human) {
		t.Fatal("human ❌ is not Subotto chrome")
	}
	bot := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
		},
	}
	if !messageHasSubottoChrome(bot) {
		t.Fatal("bot 💾 is chrome")
	}
}

func TestExtraManagedChromeKeepsWantedIgnoreSet(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
			{Me: true, Emoji: &discordgo.Emoji{Name: "❌"}},
			{Me: true, Emoji: &discordgo.Emoji{Name: "2️⃣"}},
			{Me: true, Emoji: &discordgo.Emoji{Name: "🇩"}},
		},
	}
	wanted := []string{"💾", "❌", "2️⃣"}
	got := extraManagedChrome(msg, wanted)
	if !slices.Equal(got, []string{"🇩"}) {
		t.Fatalf("keep ignore chrome, drop leftover DUPE: %v", got)
	}
}

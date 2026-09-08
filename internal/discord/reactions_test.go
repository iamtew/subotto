package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestReactionsFromMessageExcludesBot(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Count: 1, Me: true, Emoji: &discordgo.Emoji{Name: "💾"}},
			{Count: 3, Me: true, Emoji: &discordgo.Emoji{Name: "🔥"}},
			{Count: 2, Me: false, Emoji: &discordgo.Emoji{Name: "❤️"}},
			{Count: 1, Me: false, Emoji: &discordgo.Emoji{Name: "blob", ID: "123"}},
			{Count: 1, Me: true, Emoji: &discordgo.Emoji{Name: "spin", ID: "99", Animated: true}},
		},
	}

	got := reactionsFromMessage(msg)

	if _, ok := got["💾"]; ok {
		t.Fatalf("bot-only 💾 should be omitted: %+v", got)
	}
	if got["🔥"] != 2 {
		t.Fatalf("🔥 want 2 (3 minus bot), got %d", got["🔥"])
	}
	if got["❤️"] != 2 {
		t.Fatalf("❤️ want 2, got %d", got["❤️"])
	}
	if got["blob:123"] != 1 {
		t.Fatalf("custom emoji want 1, got %+v", got)
	}
	if _, ok := got["a:spin:99"]; ok {
		t.Fatalf("bot-only animated custom should be omitted: %+v", got)
	}
}

func TestReactionsFromMessageNil(t *testing.T) {
	if len(reactionsFromMessage(nil)) != 0 {
		t.Fatal("nil message should yield empty map")
	}
}

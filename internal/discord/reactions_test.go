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
	byEmoji := map[string]int{}
	for _, r := range got {
		byEmoji[r.Emoji] = r.Count
	}

	if _, ok := byEmoji["💾"]; ok {
		t.Fatalf("bot-only 💾 should be omitted: %+v", got)
	}
	if byEmoji["🔥"] != 2 {
		t.Fatalf("🔥 want 2 (3 minus bot), got %d", byEmoji["🔥"])
	}
	if byEmoji["❤️"] != 2 {
		t.Fatalf("❤️ want 2, got %d", byEmoji["❤️"])
	}
	if byEmoji["blob:123"] != 1 {
		t.Fatalf("custom emoji want 1, got %+v", got)
	}
	if _, ok := byEmoji["a:spin:99"]; ok {
		t.Fatalf("bot-only animated custom should be omitted: %+v", got)
	}
}

func TestReactionsFromMessagePreservesDiscordOrder(t *testing.T) {
	msg := &discordgo.Message{
		Reactions: []*discordgo.MessageReactions{
			{Count: 1, Me: false, Emoji: &discordgo.Emoji{Name: "❤️"}},
			{Count: 4, Me: false, Emoji: &discordgo.Emoji{Name: "🔥"}},
			{Count: 2, Me: false, Emoji: &discordgo.Emoji{Name: "blob", ID: "1"}},
		},
	}
	got := reactionsFromMessage(msg)
	if len(got) != 3 {
		t.Fatalf("want 3 reactions, got %+v", got)
	}
	want := []string{"❤️", "🔥", "blob:1"}
	for i, key := range want {
		if got[i].Emoji != key {
			t.Fatalf("order[%d]: want %s, got %s (%+v)", i, key, got[i].Emoji, got)
		}
	}
}

func TestReactionsFromMessageNil(t *testing.T) {
	if len(reactionsFromMessage(nil)) != 0 {
		t.Fatal("nil message should yield empty list")
	}
}

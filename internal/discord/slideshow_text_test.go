package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestFormatSlideshowMessageMentionsAndEmoji(t *testing.T) {
	m := &discordgo.Message{
		Content: "hey <@42> and <@!42> in <#99> ping <@&7> <:blob:55> <a:spin:9> leftover",
		Mentions: []*discordgo.User{
			{ID: "42", Username: "alice", GlobalName: "Alice G"},
		},
		MentionRoles: []string{"7"},
		MentionChannels: []*discordgo.Channel{
			{ID: "99", Name: "pics"},
		},
	}
	got := formatSlideshowMessage(nil, m)
	want := "hey @Alice G and @Alice G in #pics ping @7 <:blob:55> <a:spin:9> leftover"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatSlideshowMessageEmpty(t *testing.T) {
	if formatSlideshowMessage(nil, nil) != "" {
		t.Fatal("nil message")
	}
	if formatSlideshowMessage(nil, &discordgo.Message{Content: "  "}) != "" {
		t.Fatal("whitespace-only should trim empty")
	}
}

func TestDisplayNameFromUser(t *testing.T) {
	if displayNameFromUser(&discordgo.User{ID: "1", Username: "u", GlobalName: "G"}) != "G" {
		t.Fatal("prefer global name")
	}
	if displayNameFromUser(&discordgo.User{ID: "1", Username: "u"}) != "u" {
		t.Fatal("username fallback")
	}
}

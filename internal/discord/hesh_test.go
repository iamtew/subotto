package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHeshContextTurns(t *testing.T) {
	botID := "bot1"
	msgs := []*discordgo.Message{
		{ID: "cur", Content: "now", Author: &discordgo.User{ID: "u1", Username: "alice"}},
		{ID: "3", Content: "do it", Author: &discordgo.User{ID: botID, Username: "subotto"}},
		{ID: "2", Content: "  ", Author: &discordgo.User{ID: "u2", Username: "bob"}},
		{ID: "1", Content: "<@" + botID + "> kickflip?", Author: &discordgo.User{ID: "u1", Username: "alice"}},
	}
	got := heshContextTurns(msgs, botID, "cur")
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Role != "user" || got[0].Content != "alice: kickflip?" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "do it" {
		t.Fatalf("second %+v", got[1])
	}
}

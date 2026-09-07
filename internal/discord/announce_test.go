package discord

import (
	"strings"
	"testing"

	"subotto/internal/db"
)

func TestFormatListenMessage(t *testing.T) {
	listen := &db.ChannelMapping{
		Name:              "Week 12",
		YouTubePlaylistID: "PLabc",
		DiscordChannelID:  "123",
	}
	out := FormatListenMessage("{{name}} / {{playlist_id}} / {{channel_id}}", listen)
	if out != "Week 12 / PLabc / 123" {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(db.DefaultListenStartMessage, "listener") {
		t.Fatal("default start copy should say listener")
	}
	if !strings.Contains(strings.ToLower(db.DefaultListenStartMessage), "listener") {
		t.Fatal("default start copy should say listener")
	}
}

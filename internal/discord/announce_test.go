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
	if !strings.Contains(db.DefaultListenStartMessage, "{{playlist_id}}") {
		t.Fatal("default ONLINE notice should link the playlist")
	}
	if !strings.Contains(db.DefaultListenStartMessage, "collecting content") {
		t.Fatal("default ONLINE notice should mention collecting content")
	}
	if !strings.Contains(db.DefaultListenStopMessage, "stopped") {
		t.Fatal("default OFFLINE notice should say collection stopped")
	}
	formatted := FormatListenMessage(db.DefaultListenStartMessage, listen)
	if !strings.Contains(formatted, "playlist?list=PLabc") || !strings.Contains(formatted, "Week 12") {
		t.Fatalf("placeholders not filled: %q", formatted)
	}
}

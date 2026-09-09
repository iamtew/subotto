package discord

import (
	"context"
	"strings"
	"testing"
	"unicode/utf16"

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

func TestFormatPictureListenMessage(t *testing.T) {
	listen := &db.PictureListener{
		Name:             "Fan art night",
		Slug:             "fan-art-night",
		DiscordChannelID: "456",
	}
	out := FormatPictureListenMessage("{{name}} / {{slug}} / {{channel_id}}", listen)
	if out != "Fan art night / fan-art-night / 456" {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(db.DefaultPictureListenStartMessage, "{{slug}}") {
		t.Fatal("default ONLINE picture notice should mention slideshow slug")
	}
	if !strings.Contains(db.DefaultPictureListenStartMessage, "collecting pictures") {
		t.Fatal("default ONLINE picture notice should mention collecting pictures")
	}
	if !strings.Contains(db.DefaultPictureListenStopMessage, "stopped") {
		t.Fatal("default OFFLINE picture notice should say collection stopped")
	}
	formatted := FormatPictureListenMessage(db.DefaultPictureListenStartMessage, listen)
	if !strings.Contains(formatted, "/slideshow/fan-art-night") || !strings.Contains(formatted, "Fan art night") {
		t.Fatalf("placeholders not filled: %q", formatted)
	}
}

func TestSanitizeAnnounceField(t *testing.T) {
	listen := &db.ChannelMapping{
		Name:              "hi @everyone ](https://evil)",
		YouTubePlaylistID: "PL",
		DiscordChannelID:  "1",
	}
	out := FormatListenMessage("{{name}}", listen)
	if strings.Contains(out, "@everyone") {
		t.Fatalf("should neutralize @everyone: %q", out)
	}
	if strings.Contains(out, "](https://evil)") {
		t.Fatalf("should break markdown link injection: %q", out)
	}
}

func TestAnnounceRejectsOversize(t *testing.T) {
	// Build a string that is over Discord's UTF-16 cap.
	runes := make([]rune, DiscordMaxMessageLength+1)
	for i := range runes {
		runes[i] = 'a'
	}
	msg := string(runes)
	if len(utf16.Encode([]rune(msg))) <= DiscordMaxMessageLength {
		t.Fatal("test setup: expected oversize message")
	}
	err := Announce(context.Background(), "fake-token", "channel", msg)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected oversize error, got %v", err)
	}
}

func TestAnnounceEmptyIsNoop(t *testing.T) {
	if err := Announce(context.Background(), "fake-token", "channel", "   "); err != nil {
		t.Fatalf("empty notice should be silent success, got %v", err)
	}
}

func TestDiscordUTF16LenEmoji(t *testing.T) {
	// 💚 is outside the BMP → 2 UTF-16 code units.
	if DiscordUTF16Len("💚") != 2 {
		t.Fatalf("got %d", DiscordUTF16Len("💚"))
	}
}

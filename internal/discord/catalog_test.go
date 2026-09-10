package discord

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestClampChatHistory(t *testing.T) {
	if got := clampChatHistory(0); got != defaultChatHistory {
		t.Fatalf("default: got %d", got)
	}
	if got := clampChatHistory(3); got != 3 {
		t.Fatalf("keep 3: got %d", got)
	}
	if got := clampChatHistory(99); got != maxChatHistory {
		t.Fatalf("cap: got %d", got)
	}
}

func TestToChatMessage(t *testing.T) {
	ts := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	m := &discordgo.Message{
		ID:        "m1",
		Content:   " hello ",
		Timestamp: ts,
		Author:    &discordgo.User{ID: "bot-1", Username: "subotto", Bot: true},
		Attachments: []*discordgo.MessageAttachment{
			{Filename: "shot.png", URL: "https://cdn.example/shot.png", ContentType: "image/png"},
		},
	}
	got := toChatMessage(m, "bot-1")
	if got.Author != "subotto" || !got.Bot || !got.Self {
		t.Fatalf("author flags: %+v", got)
	}
	if got.Content != "hello" {
		t.Fatalf("content: %q", got.Content)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "shot.png" {
		t.Fatalf("attachments: %+v", got.Attachments)
	}
	if got.Timestamp != "2026-09-11T12:00:00Z" {
		t.Fatalf("ts: %q", got.Timestamp)
	}
}

func TestNormalizeChatUpload(t *testing.T) {
	if _, _, err := normalizeChatUpload("  ", nil); err == nil {
		t.Fatal("empty should fail")
	}
	content, files, err := normalizeChatUpload("  hi  ", []ChatFile{{Name: "sub/dir/note.txt", Data: []byte("x")}})
	if err != nil {
		t.Fatal(err)
	}
	if content != "hi" || len(files) != 1 || files[0].Name != "note.txt" {
		t.Fatalf("got %q %+v", content, files)
	}
	big := make([]byte, MaxChatFileBytes+1)
	if _, _, err := normalizeChatUpload("", []ChatFile{{Name: "big.bin", Data: big}}); err == nil {
		t.Fatal("oversize should fail")
	}
	if _, _, err := normalizeChatUpload("", []ChatFile{{Name: "ok.bin", Data: []byte("ok")}}); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeChatFilename(t *testing.T) {
	if got := sanitizeChatFilename("sub/dir/photo.jpg"); got != "photo.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeChatFilename(".."); got != "" {
		t.Fatalf("dotdot: %q", got)
	}
}

package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenAnnounceMessagesDefaultsAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	start, stop, err := store.ListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start == "" || stop == "" {
		t.Fatal("expected default start/stop messages")
	}
	if err := store.SetSettings(ctx, map[string]string{
		SettingListenStartMessage: "ONLINE {{name}}",
		SettingListenStopMessage:  "OFFLINE {{channel_id}}",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.ListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "ONLINE {{name}}" || stop != "OFFLINE {{channel_id}}" {
		t.Fatalf("got start=%q stop=%q", start, stop)
	}
}

func TestPictureListenAnnounceMessagesDefaultsAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "picture-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	start, stop, err := store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != DefaultPictureListenStartMessage || stop != DefaultPictureListenStopMessage {
		t.Fatalf("expected picture defaults, got start=%q stop=%q", start, stop)
	}
	if err := store.SetSettings(ctx, map[string]string{
		SettingPictureListenStartMessage: "PICS ONLINE {{slug}}",
		SettingPictureListenStopMessage:  "PICS OFFLINE {{name}}",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "PICS ONLINE {{slug}}" || stop != "PICS OFFLINE {{name}}" {
		t.Fatalf("got start=%q stop=%q", start, stop)
	}
}

func TestAnnounceSilenceVsDefault(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "silence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	// Missing keys → defaults.
	start, stop, err := store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start == "" || stop == "" {
		t.Fatal("missing keys should use defaults, not silence")
	}

	// Saved empty → silence.
	if err := store.SetSettings(ctx, map[string]string{
		SettingPictureListenStartMessage: "",
		SettingPictureListenStopMessage:  "",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "" || stop != "" {
		t.Fatalf("saved empty should silence, got start=%q stop=%q", start, stop)
	}
}

func TestValidateAnnounceTemplate(t *testing.T) {
	if err := ValidateAnnounceTemplate("ONLINE notice", "ok"); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", MaxAnnounceTemplateLength+1)
	if err := ValidateAnnounceTemplate("ONLINE notice", huge); err == nil {
		t.Fatal("expected oversize error")
	}
	if NormalizeAnnounceTemplate("  \n\t  ") != "" {
		t.Fatal("whitespace-only should normalize to empty")
	}
}

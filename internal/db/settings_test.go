package db

import (
	"context"
	"path/filepath"
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
	if err := store.SetSetting(ctx, SettingListenStartMessage, "ONLINE {{name}}"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, SettingListenStopMessage, "OFFLINE {{channel_id}}"); err != nil {
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

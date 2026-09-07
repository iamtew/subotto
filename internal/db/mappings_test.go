package db

import (
	"context"
	"path/filepath"
	"testing"
)

// openTestDB creates a fresh SQLite file under t.TempDir().
// Meat Bag: each test gets its own DB so tests do not stomp on each other.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMappingCRUD(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	m, err := store.UpsertMapping(ctx, "chan-1", "guild-1", "pl-1", "alpha", true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if m.DiscordChannelID != "chan-1" || !m.Enabled {
		t.Fatalf("unexpected upsert result: %+v", m)
	}

	_, err = store.UpsertMapping(ctx, "chan-2", "guild-1", "pl-2", "beta", true)
	if err != nil {
		t.Fatalf("upsert second: %v", err)
	}

	list, err := store.ListMappings(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 mappings, got %d", len(list))
	}
	// Newest first (chan-2 inserted second → higher id).
	if list[0].DiscordChannelID != "chan-2" {
		t.Fatalf("want newest first (chan-2), got %s", list[0].DiscordChannelID)
	}

	disabled, err := store.SetMappingEnabled(ctx, "chan-1", false)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("expected mapping to be disabled")
	}

	enabledOnly, err := store.GetEnabledMappingByChannel(ctx, "chan-1")
	if err != nil {
		t.Fatalf("get enabled: %v", err)
	}
	if enabledOnly != nil {
		t.Fatal("disabled mapping must not be returned by GetEnabledMappingByChannel")
	}

	any, err := store.GetMappingByChannel(ctx, "chan-1")
	if err != nil || any == nil || any.Enabled {
		t.Fatalf("GetMappingByChannel should still return disabled row: %+v err=%v", any, err)
	}

	reenabled, err := store.SetMappingEnabled(ctx, "chan-1", true)
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if !reenabled.Enabled {
		t.Fatal("expected mapping to be enabled again")
	}

	if err := store.DeleteMapping(ctx, "chan-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, err := store.GetMappingByChannel(ctx, "chan-1")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if gone != nil {
		t.Fatal("active mapping should be gone after soft-delete")
	}

	if err := store.DeleteMapping(ctx, "chan-missing"); err == nil {
		t.Fatal("delete missing channel should error")
	}
	if _, err := store.SetMappingEnabled(ctx, "chan-missing", true); err == nil {
		t.Fatal("enable missing channel should error")
	}
}

func TestMappingEpochReplaceAndLookback(t *testing.T) {
	ctx := context.Background()
	store := openTestDB(t)

	m1, err := store.UpsertMapping(ctx, "chan-x", "g", "pl-old", "fortnight-1", true)
	if err != nil {
		t.Fatalf("upsert1: %v", err)
	}

	// First epoch: no previous boundary.
	nb, err := store.ResyncNotBefore(ctx, "chan-x")
	if err != nil {
		t.Fatalf("notBefore1: %v", err)
	}
	if !nb.IsZero() {
		t.Fatalf("first mapping should have no lookback floor, got %v", nb)
	}

	m2, err := store.UpsertMapping(ctx, "chan-x", "g", "pl-new", "fortnight-2", true)
	if err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	if m2.ID == m1.ID {
		t.Fatal("new playlist should open a new epoch row")
	}
	if m2.YouTubePlaylistID != "pl-new" {
		t.Fatalf("unexpected playlist: %+v", m2)
	}

	// Old epoch closed; only one active.
	active, err := store.ListMappings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].YouTubePlaylistID != "pl-new" {
		t.Fatalf("want one active new mapping, got %+v", active)
	}

	nb2, err := store.ResyncNotBefore(ctx, "chan-x")
	if err != nil {
		t.Fatalf("notBefore2: %v", err)
	}
	if nb2.IsZero() {
		t.Fatal("second epoch must have a lookback floor from previous active_until")
	}
}

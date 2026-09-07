package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Global listener announce templates (one set for every guild/channel —
// a private ops desk under Meat Bag control). Placeholders: {{name}}, {{playlist_id}}, {{channel_id}}.
const (
	SettingListenStartMessage = "listen_start_message"
	SettingListenStopMessage  = "listen_stop_message"
	// Legacy keys from the short-lived "air" naming — still read for upgrades.
	SettingAirStartMessage = "air_start_message"
	SettingAirStopMessage  = "air_stop_message"
)

// DefaultListenStartMessage is posted when a listener goes ONLINE.
const DefaultListenStartMessage = `🎧 **{{name}}** — listener ONLINE.
SIGINT collection active. Drop YouTube links; Subotto files them to the playlist.
_(Eyes on. Ears open. Your channel, your watch.)_`

// DefaultListenStopMessage is posted when a listener goes OFFLINE.
const DefaultListenStopMessage = `⏹ **{{name}}** — listener OFFLINE.
Collection window closed. This channel is no longer under watch.
_(The wire went quiet — until you open it again.)_`

// Deprecated aliases so older call sites compile during the rename.
const (
	DefaultAirStartMessage = DefaultListenStartMessage
	DefaultAirStopMessage  = DefaultListenStopMessage
)

func (d *DB) ensureSettingsTable() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
)`)
	return err
}

// GetSetting reads a settings value, or "" if missing.
func (d *DB) GetSetting(ctx context.Context, key string) (string, error) {
	if err := d.ensureSettingsTable(); err != nil {
		return "", err
	}
	var value string
	err := d.sql.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting writes a settings value.
func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	if err := d.ensureSettingsTable(); err != nil {
		return err
	}
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')
	`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// ListenAnnounceMessages returns start/stop templates (defaults if unset).
// Prefers listen_* keys; falls back to legacy air_* if Meat Bag already saved those.
func (d *DB) ListenAnnounceMessages(ctx context.Context) (start, stop string, err error) {
	start, err = d.GetSetting(ctx, SettingListenStartMessage)
	if err != nil {
		return "", "", err
	}
	if start == "" {
		start, err = d.GetSetting(ctx, SettingAirStartMessage)
		if err != nil {
			return "", "", err
		}
	}
	stop, err = d.GetSetting(ctx, SettingListenStopMessage)
	if err != nil {
		return "", "", err
	}
	if stop == "" {
		stop, err = d.GetSetting(ctx, SettingAirStopMessage)
		if err != nil {
			return "", "", err
		}
	}
	if start == "" {
		start = DefaultListenStartMessage
	}
	if stop == "" {
		stop = DefaultListenStopMessage
	}
	return start, stop, nil
}

// AirAnnounceMessages is a legacy alias for ListenAnnounceMessages.
func (d *DB) AirAnnounceMessages(ctx context.Context) (start, stop string, err error) {
	return d.ListenAnnounceMessages(ctx)
}

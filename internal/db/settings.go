package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Global listener ONLINE/OFFLINE notices (one set for every guild/channel —
// a private ops desk under Meat Bag control).
//
// Content placeholders: {{name}}, {{playlist_id}}, {{channel_id}}.
// Picture placeholders: {{name}}, {{slug}}, {{channel_id}}.
//
// Silence: a saved empty template (key present, value "") posts nothing.
// Missing key → built-in default. Clear the Admin textarea and SAVE to silence.
const (
	SettingListenStartMessage = "listen_start_message"
	SettingListenStopMessage  = "listen_stop_message"
	// Legacy keys from the short-lived "air" naming — still read for upgrades.
	SettingAirStartMessage = "air_start_message"
	SettingAirStopMessage  = "air_stop_message"

	SettingPictureListenStartMessage = "picture_listen_start_message"
	SettingPictureListenStopMessage  = "picture_listen_stop_message"
)

// MaxAnnounceTemplateLength matches Discord's message cap (UTF-16 code units).
// Templates are checked on save; formatted notices are checked again on send.
const MaxAnnounceTemplateLength = 2000

// DefaultListenStartMessage is posted when a content listener goes ONLINE.
const DefaultListenStartMessage = `## Now collecting content for ***[{{name}}](<https://www.youtube.com/playlist?list={{playlist_id}}>)***`

// DefaultListenStopMessage is posted when a content listener goes OFFLINE.
const DefaultListenStopMessage = `## Content collection has been stopped!
Thank you for your participation to ***[{{name}}](<https://www.youtube.com/playlist?list={{playlist_id}}>)*** 💚`

// DefaultPictureListenStartMessage is posted when a picture listener goes ONLINE.
// Path-only slideshow link — Meat Bag can prepend their public host in Admin.
const DefaultPictureListenStartMessage = `## Now collecting pictures for ***{{name}}***
Slideshow: ` + "`/slideshow/{{slug}}`"

// DefaultPictureListenStopMessage is posted when a picture listener goes OFFLINE.
const DefaultPictureListenStopMessage = `## Picture collection has been stopped!
Thank you for your participation to ***{{name}}*** 💚`

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
	value, _, err := d.getSettingPresent(ctx, key)
	return value, err
}

// getSettingPresent distinguishes missing keys from saved empty values.
func (d *DB) getSettingPresent(ctx context.Context, key string) (value string, present bool, err error) {
	if err := d.ensureSettingsTable(); err != nil {
		return "", false, err
	}
	err = d.sql.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, true, nil
}

// SetSetting writes a settings value.
func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	return d.SetSettings(ctx, map[string]string{key: value})
}

// SetSettings writes several settings in one transaction (both notice templates together).
func (d *DB) SetSettings(ctx context.Context, pairs map[string]string) error {
	if len(pairs) == 0 {
		return nil
	}
	if err := d.ensureSettingsTable(); err != nil {
		return err
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settings tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for key, value := range pairs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')
		`, key, value); err != nil {
			return fmt.Errorf("set setting %q: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings: %w", err)
	}
	return nil
}

// NormalizeAnnounceTemplate trims whitespace; whitespace-only becomes "" (silence).
func NormalizeAnnounceTemplate(s string) string {
	return strings.TrimSpace(s)
}

// ValidateAnnounceTemplate checks Discord's UTF-16 length cap.
func ValidateAnnounceTemplate(label, s string) error {
	n := len(utf16.Encode([]rune(s)))
	if n > MaxAnnounceTemplateLength {
		return fmt.Errorf("%s exceeds Discord %d-character limit (%d)", label, MaxAnnounceTemplateLength, n)
	}
	return nil
}

func (d *DB) loadAnnounceTemplate(ctx context.Context, primaryKey, legacyKey, fallback string) (string, error) {
	v, present, err := d.getSettingPresent(ctx, primaryKey)
	if err != nil {
		return "", err
	}
	if present {
		return v, nil // may be "" → silence
	}
	if legacyKey != "" {
		v, present, err = d.getSettingPresent(ctx, legacyKey)
		if err != nil {
			return "", err
		}
		if present {
			return v, nil
		}
	}
	return fallback, nil
}

// ListenAnnounceMessages returns start/stop templates.
// Missing keys → defaults. Saved empty string → silence (no Discord post).
func (d *DB) ListenAnnounceMessages(ctx context.Context) (start, stop string, err error) {
	start, err = d.loadAnnounceTemplate(ctx, SettingListenStartMessage, SettingAirStartMessage, DefaultListenStartMessage)
	if err != nil {
		return "", "", err
	}
	stop, err = d.loadAnnounceTemplate(ctx, SettingListenStopMessage, SettingAirStopMessage, DefaultListenStopMessage)
	if err != nil {
		return "", "", err
	}
	return start, stop, nil
}

// AirAnnounceMessages is a legacy alias for ListenAnnounceMessages.
func (d *DB) AirAnnounceMessages(ctx context.Context) (start, stop string, err error) {
	return d.ListenAnnounceMessages(ctx)
}

// PictureListenAnnounceMessages returns picture start/stop templates.
// Missing keys → defaults. Saved empty string → silence.
func (d *DB) PictureListenAnnounceMessages(ctx context.Context) (start, stop string, err error) {
	start, err = d.loadAnnounceTemplate(ctx, SettingPictureListenStartMessage, "", DefaultPictureListenStartMessage)
	if err != nil {
		return "", "", err
	}
	stop, err = d.loadAnnounceTemplate(ctx, SettingPictureListenStopMessage, "", DefaultPictureListenStopMessage)
	if err != nil {
		return "", "", err
	}
	return start, stop, nil
}

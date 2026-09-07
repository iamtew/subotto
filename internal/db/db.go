// Package db talks to Subotto's SQLite database.
//
// Meat Bag: SQLite is one file on disk (see DATABASE_PATH). No separate
// database server to install. We use the pure-Go driver modernc.org/sqlite
// so Windows DEV and Linux PROD builds do not need CGO / a C compiler.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver with database/sql
)

// DB wraps *sql.DB with Subotto-specific helpers.
type DB struct {
	sql *sql.DB
}

// Open creates the parent directory if needed, opens SQLite, and runs migrations.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	// _pragma=foreign_keys(1) turns on foreign keys for this connection.
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// One writer at a time is plenty for a single-VPS bot.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return d, nil
}

// Close shuts down the database connection.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// migrate creates tables if they do not exist yet.
// Safe to run every startup — CREATE TABLE IF NOT EXISTS is a no-op when present.
func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS channel_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	discord_channel_id TEXT NOT NULL,
	guild_id TEXT NOT NULL DEFAULT '',
	youtube_playlist_id TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_mappings_channel
	ON channel_mappings (discord_channel_id);

CREATE TABLE IF NOT EXISTS processed_videos (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	video_id TEXT NOT NULL,
	playlist_id TEXT NOT NULL,
	discord_message_id TEXT NOT NULL DEFAULT '',
	added_at TEXT NOT NULL DEFAULT (datetime('now')),
	UNIQUE (video_id, playlist_id)
);

CREATE TABLE IF NOT EXISTS activity_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp TEXT NOT NULL DEFAULT (datetime('now')),
	event_type TEXT NOT NULL,
	details TEXT NOT NULL DEFAULT '{}',
	success INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_activity_log_timestamp
	ON activity_log (timestamp DESC);

CREATE TABLE IF NOT EXISTS oauth_tokens (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`
	_, err := d.sql.Exec(schema)
	return err
}

// ---------- Types Meat Bag will see in later phases ----------

// ChannelMapping is one Discord channel → YouTube playlist link.
type ChannelMapping struct {
	ID                 int64
	DiscordChannelID   string
	GuildID            string
	YouTubePlaylistID  string
	Name               string
	Enabled            bool
	CreatedAt          time.Time
}

// ActivityEntry is one line in the activity / audit log.
type ActivityEntry struct {
	ID        int64
	Timestamp time.Time
	EventType string
	Details   string // JSON text
	Success   bool
}

// ---------- Phase 1 helpers (enough to prove the DB works) ----------

// LogActivity appends a row to activity_log.
// details can be any JSON-friendly value (map, struct, string); we marshal it.
func (d *DB) LogActivity(ctx context.Context, eventType string, details any, success bool) error {
	payload := "{}"
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("marshal activity details: %w", err)
		}
		payload = string(b)
	}

	successInt := 0
	if success {
		successInt = 1
	}

	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO activity_log (event_type, details, success) VALUES (?, ?, ?)`,
		eventType, payload, successInt,
	)
	if err != nil {
		return fmt.Errorf("insert activity_log: %w", err)
	}
	return nil
}

// CountMappings returns how many channel ↔ playlist rows exist.
func (d *DB) CountMappings(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_mappings`).Scan(&n)
	return n, err
}

// CountActivity returns how many activity_log rows exist.
func (d *DB) CountActivity(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_log`).Scan(&n)
	return n, err
}

// CountEnabledMappings returns how many mappings are currently enabled.
func (d *DB) CountEnabledMappings(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_mappings WHERE enabled = 1`).Scan(&n)
	return n, err
}

// ListActivity returns the newest activity_log rows (newest first).
// Meat Bag: the Admin UI uses this for the activity table.
func (d *DB) ListActivity(ctx context.Context, limit int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, timestamp, event_type, details, success
		FROM activity_log
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	defer rows.Close()

	var out []ActivityEntry
	for rows.Next() {
		var (
			e       ActivityEntry
			ts      string
			success int
		)
		if err := rows.Scan(&e.ID, &ts, &e.EventType, &e.Details, &success); err != nil {
			return nil, err
		}
		e.Timestamp = parseSQLiteTime(ts)
		e.Success = success == 1
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ActivityEntry{}
	}
	return out, nil
}

// WasVideoProcessed returns true if this video was already added to this playlist.
func (d *DB) WasVideoProcessed(ctx context.Context, videoID, playlistID string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM processed_videos WHERE video_id = ? AND playlist_id = ?`,
		videoID, playlistID,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// MarkVideoProcessed records a successful add (used for dedup later).
func (d *DB) MarkVideoProcessed(ctx context.Context, videoID, playlistID, discordMessageID string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT OR IGNORE INTO processed_videos (video_id, playlist_id, discord_message_id)
		 VALUES (?, ?, ?)`,
		videoID, playlistID, discordMessageID,
	)
	return err
}

// SaveOAuthToken stores a token blob under a key (e.g. "youtube").
func (d *DB) SaveOAuthToken(ctx context.Context, key, value string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO oauth_tokens (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`,
		key, value,
	)
	return err
}

// LoadOAuthToken returns the token blob for a key, or "" if missing.
func (d *DB) LoadOAuthToken(ctx context.Context, key string) (string, error) {
	var value string
	err := d.sql.QueryRowContext(ctx,
		`SELECT value FROM oauth_tokens WHERE key = ?`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

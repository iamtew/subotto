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
	sql         *sql.DB
	path        string // absolute-ish path to the SQLite file
	picturesDir string // on-disk gallery root: <data dir>/pictures
}

// Open creates the parent directory if needed, opens SQLite, and runs migrations.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	// Picture listener files live next to the DB: data/pictures/{slug}/…
	picturesDir := filepath.Join(dir, "pictures")
	if err := os.MkdirAll(picturesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create pictures directory %q: %w", picturesDir, err)
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

	d := &DB{sql: sqlDB, path: path, picturesDir: picturesDir}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return d, nil
}

// PicturesDir is the host folder where picture listeners store image files.
func (d *DB) PicturesDir() string {
	if d == nil {
		return ""
	}
	return d.picturesDir
}

// Close shuts down the database connection.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// migrate creates tables if they do not exist yet, then applies additive upgrades.
// Safe to run every startup.
func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS channel_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	discord_channel_id TEXT NOT NULL,
	guild_id TEXT NOT NULL DEFAULT '',
	youtube_playlist_id TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	active_from TEXT NOT NULL DEFAULT (datetime('now')),
	active_until TEXT
);

CREATE TABLE IF NOT EXISTS processed_videos (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	video_id TEXT NOT NULL,
	playlist_id TEXT NOT NULL,
	discord_channel_id TEXT NOT NULL DEFAULT '',
	discord_message_id TEXT NOT NULL DEFAULT '',
	added_at TEXT NOT NULL DEFAULT (datetime('now'))
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

CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS picture_listeners (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	discord_channel_id TEXT NOT NULL,
	guild_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	slug TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	credit_corner TEXT NOT NULL DEFAULT 'br',
	interval_seconds INTEGER NOT NULL DEFAULT 8,
	shuffle INTEGER NOT NULL DEFAULT 0,
	show_credit INTEGER NOT NULL DEFAULT 1,
	show_reactions INTEGER NOT NULL DEFAULT 1,
	reaction_multiplier INTEGER NOT NULL DEFAULT 1,
	credit_scale REAL NOT NULL DEFAULT 1.5,
	reaction_scale REAL NOT NULL DEFAULT 1.5,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	active_from TEXT NOT NULL DEFAULT (datetime('now')),
	active_until TEXT
);

CREATE TABLE IF NOT EXISTS collected_pictures (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	listener_id INTEGER NOT NULL,
	discord_message_id TEXT NOT NULL,
	discord_attachment_id TEXT NOT NULL,
	author_id TEXT NOT NULL DEFAULT '',
	author_display_name TEXT NOT NULL DEFAULT '',
	stored_path TEXT NOT NULL,
	content_type TEXT NOT NULL DEFAULT '',
	collected_at TEXT NOT NULL DEFAULT (datetime('now')),
	reactions_json TEXT NOT NULL DEFAULT '{}'
);
`
	if _, err := d.sql.Exec(schema); err != nil {
		return err
	}
	return d.migrateUpgrades()
}

// migrateUpgrades brings older Subotto DBs forward (Meat Bag may already have data/).
func (d *DB) migrateUpgrades() error {
	cols, err := d.tableColumns("channel_mappings")
	if err != nil {
		return err
	}
	if !cols["active_from"] {
		// SQLite ALTER TABLE cannot use non-constant defaults like datetime('now').
		// Add with a constant, then copy created_at into active_from.
		if _, err := d.sql.Exec(`ALTER TABLE channel_mappings ADD COLUMN active_from TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add active_from: %w", err)
		}
		if _, err := d.sql.Exec(`UPDATE channel_mappings SET active_from = created_at WHERE active_from = '' OR active_from IS NULL`); err != nil {
			return fmt.Errorf("backfill active_from: %w", err)
		}
	}
	if !cols["active_until"] {
		if _, err := d.sql.Exec(`ALTER TABLE channel_mappings ADD COLUMN active_until TEXT`); err != nil {
			return fmt.Errorf("add active_until: %w", err)
		}
	}

	// Old DBs had UNIQUE(discord_channel_id). Epochs need many rows per channel,
	// with at most one *active* (active_until IS NULL).
	if _, err := d.sql.Exec(`DROP INDEX IF EXISTS idx_channel_mappings_channel`); err != nil {
		return fmt.Errorf("drop old channel unique index: %w", err)
	}
	if _, err := d.sql.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_mappings_active_channel
		ON channel_mappings (discord_channel_id) WHERE active_until IS NULL
	`); err != nil {
		return fmt.Errorf("create active-channel unique index: %w", err)
	}

	if err := d.migrateProcessedVideosChannelScope(); err != nil {
		return err
	}
	return d.migratePictureListenerIndexes()
}

// migratePictureListenerIndexes ensures unique constraints for live picture listeners.
func (d *DB) migratePictureListenerIndexes() error {
	if _, err := d.sql.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_picture_listeners_active_channel
		ON picture_listeners (discord_channel_id) WHERE active_until IS NULL
	`); err != nil {
		return fmt.Errorf("create picture active-channel unique index: %w", err)
	}
	if _, err := d.sql.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_picture_listeners_active_slug
		ON picture_listeners (slug) WHERE active_until IS NULL
	`); err != nil {
		return fmt.Errorf("create picture active-slug unique index: %w", err)
	}
	if _, err := d.sql.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_collected_pictures_listener_attachment
		ON collected_pictures (listener_id, discord_attachment_id)
	`); err != nil {
		return fmt.Errorf("create collected_pictures unique index: %w", err)
	}
	if _, err := d.sql.Exec(`
		CREATE INDEX IF NOT EXISTS idx_collected_pictures_listener
		ON collected_pictures (listener_id, id)
	`); err != nil {
		return fmt.Errorf("create collected_pictures listener index: %w", err)
	}
	return d.migratePictureListenerColumns()
}

// migratePictureListenerColumns adds Phase 8+ slideshow columns on older DBs.
func (d *DB) migratePictureListenerColumns() error {
	cols, err := d.tableColumns("picture_listeners")
	if err != nil {
		return err
	}
	if !cols["reaction_multiplier"] {
		if _, err := d.sql.Exec(`
			ALTER TABLE picture_listeners
			ADD COLUMN reaction_multiplier INTEGER NOT NULL DEFAULT 1
		`); err != nil {
			return fmt.Errorf("add reaction_multiplier: %w", err)
		}
	}
	if !cols["credit_scale"] {
		if _, err := d.sql.Exec(`
			ALTER TABLE picture_listeners
			ADD COLUMN credit_scale REAL NOT NULL DEFAULT 1.5
		`); err != nil {
			return fmt.Errorf("add credit_scale: %w", err)
		}
	}
	if !cols["reaction_scale"] {
		if _, err := d.sql.Exec(`
			ALTER TABLE picture_listeners
			ADD COLUMN reaction_scale REAL NOT NULL DEFAULT 1.5
		`); err != nil {
			return fmt.Errorf("add reaction_scale: %w", err)
		}
	}
	return nil
}

func (d *DB) migrateProcessedVideosChannelScope() error {
	cols, err := d.tableColumns("processed_videos")
	if err != nil {
		return err
	}
	if cols["discord_channel_id"] {
		// Ensure channel-scoped unique index exists (fresh DBs created without UNIQUE in CREATE).
		_, err := d.sql.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_processed_videos_channel
			ON processed_videos (video_id, discord_channel_id)
		`)
		return err
	}

	// Rebuild: old UNIQUE(video_id, playlist_id) → channel-scoped dedup.
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`ALTER TABLE processed_videos RENAME TO processed_videos_legacy`); err != nil {
		return fmt.Errorf("rename processed_videos: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE TABLE processed_videos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			video_id TEXT NOT NULL,
			playlist_id TEXT NOT NULL,
			discord_channel_id TEXT NOT NULL DEFAULT '',
			discord_message_id TEXT NOT NULL DEFAULT '',
			added_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create processed_videos: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO processed_videos (video_id, playlist_id, discord_channel_id, discord_message_id, added_at)
		SELECT video_id, playlist_id, '', discord_message_id, added_at FROM processed_videos_legacy
	`); err != nil {
		return fmt.Errorf("copy processed_videos: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE processed_videos_legacy`); err != nil {
		return fmt.Errorf("drop processed_videos_legacy: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_processed_videos_channel
		ON processed_videos (video_id, discord_channel_id)
	`); err != nil {
		return fmt.Errorf("index processed_videos: %w", err)
	}
	return tx.Commit()
}

func (d *DB) tableColumns(table string) (map[string]bool, error) {
	rows, err := d.sql.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// ---------- Types Meat Bag will see in later phases ----------

// ChannelMapping is one Discord channel → YouTube playlist *content listener* epoch.
// Meat Bag: you flip epochs in the Admin UI whenever you want (fortnight,
// three weeks, whatever). Only one content listener can be live per channel —
// a picture listener may also be live on the same channel at the same time.
type ChannelMapping struct {
	ID                int64
	DiscordChannelID  string
	GuildID           string
	YouTubePlaylistID string
	Name              string
	Enabled           bool
	CreatedAt         time.Time
	ActiveFrom        time.Time
	ActiveUntil       *time.Time // nil = currently active epoch
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

// CountMappings returns how many *active* channel ↔ playlist rows exist.
func (d *DB) CountMappings(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM channel_mappings WHERE active_until IS NULL`,
	).Scan(&n)
	return n, err
}

// CountActivity returns how many activity_log rows exist.
func (d *DB) CountActivity(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_log`).Scan(&n)
	return n, err
}

// CountEnabledMappings returns how many *active* mappings are currently enabled.
func (d *DB) CountEnabledMappings(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM channel_mappings
		WHERE active_until IS NULL AND enabled = 1
	`).Scan(&n)
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

// WasVideoProcessedOnChannel returns true if this video was already saved for
// this Discord channel (any playlist epoch). That way a biweekly playlist swap
// does not re-add the same links on resync.
func (d *DB) WasVideoProcessedOnChannel(ctx context.Context, videoID, channelID string) (bool, error) {
	_, ok, err := d.ProcessedPlaylistOnChannel(ctx, videoID, channelID)
	return ok, err
}

// ProcessedOnChannel is the first filing of a video on one Discord channel.
type ProcessedOnChannel struct {
	PlaylistID string
	MessageID  string // Discord message that first saved it (may be empty on very old rows)
}

// LookupProcessedOnChannel returns the first filing of this video on this channel.
// ok=false means never processed here.
func (d *DB) LookupProcessedOnChannel(ctx context.Context, videoID, channelID string) (ProcessedOnChannel, bool, error) {
	var rec ProcessedOnChannel
	err := d.sql.QueryRowContext(ctx, `
		SELECT playlist_id, discord_message_id FROM processed_videos
		WHERE video_id = ? AND discord_channel_id = ?
	`, videoID, channelID).Scan(&rec.PlaylistID, &rec.MessageID)
	if err == sql.ErrNoRows {
		return ProcessedOnChannel{}, false, nil
	}
	if err != nil {
		return ProcessedOnChannel{}, false, err
	}
	return rec, true, nil
}

// ProcessedPlaylistOnChannel returns the playlist_id recorded when this video
// was first marked on the channel. ok=false means never processed here.
// Meat Bag: comparing that id to the current listener's playlist tells us
// whether a skip is a same-listener DUPE or a previous-listener OLD.
func (d *DB) ProcessedPlaylistOnChannel(ctx context.Context, videoID, channelID string) (playlistID string, ok bool, err error) {
	rec, ok, err := d.LookupProcessedOnChannel(ctx, videoID, channelID)
	return rec.PlaylistID, ok, err
}

// MarkVideoProcessed records a successful add for channel-scoped dedup.
func (d *DB) MarkVideoProcessed(ctx context.Context, videoID, playlistID, channelID, discordMessageID string) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT OR IGNORE INTO processed_videos (video_id, playlist_id, discord_channel_id, discord_message_id)
		VALUES (?, ?, ?, ?)
	`, videoID, playlistID, channelID, discordMessageID)
	return err
}

// WasVideoProcessed is kept for older call sites/tests — prefers channel scope
// when channelID is non-empty; otherwise falls back to playlist-only.
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

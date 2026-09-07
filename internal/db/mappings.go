package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GetEnabledMappingByChannel returns the enabled mapping for a Discord channel,
// or nil if none exists / it is disabled.
func (d *DB) GetEnabledMappingByChannel(ctx context.Context, channelID string) (*ChannelMapping, error) {
	m, err := d.GetMappingByChannel(ctx, channelID)
	if err != nil || m == nil || !m.Enabled {
		return nil, err
	}
	return m, nil
}

// GetMappingByChannel returns a mapping whether enabled or not (nil if missing).
func (d *DB) GetMappingByChannel(ctx context.Context, channelID string) (*ChannelMapping, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, discord_channel_id, guild_id, youtube_playlist_id, name, enabled, created_at
		FROM channel_mappings
		WHERE discord_channel_id = ?
	`, channelID)

	m, err := scanMapping(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// UpsertMapping creates or updates a channel → playlist mapping.
// Meat Bag: use the Admin UI, or `just add-mapping`, for this.
func (d *DB) UpsertMapping(ctx context.Context, channelID, guildID, playlistID, name string, enabled bool) (*ChannelMapping, error) {
	channelID = strings.TrimSpace(channelID)
	playlistID = strings.TrimSpace(playlistID)
	if channelID == "" || playlistID == "" {
		return nil, fmt.Errorf("discord channel id and youtube playlist id are required")
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO channel_mappings (discord_channel_id, guild_id, youtube_playlist_id, name, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(discord_channel_id) DO UPDATE SET
			guild_id = excluded.guild_id,
			youtube_playlist_id = excluded.youtube_playlist_id,
			name = excluded.name,
			enabled = excluded.enabled
	`, channelID, guildID, playlistID, name, enabledInt)
	if err != nil {
		return nil, fmt.Errorf("upsert channel mapping: %w", err)
	}

	return d.GetMappingByChannel(ctx, channelID)
}

// ListMappings returns every channel ↔ playlist row, newest first.
// Meat Bag: this includes disabled mappings so you can see what is paused.
func (d *DB) ListMappings(ctx context.Context) ([]ChannelMapping, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, discord_channel_id, guild_id, youtube_playlist_id, name, enabled, created_at
		FROM channel_mappings
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list channel mappings: %w", err)
	}
	defer rows.Close()

	var out []ChannelMapping
	for rows.Next() {
		m, err := scanMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ChannelMapping{}
	}
	return out, nil
}

// SetMappingEnabled flips the enabled flag for a channel mapping.
// Returns an error if that channel has no mapping yet.
func (d *DB) SetMappingEnabled(ctx context.Context, channelID string, enabled bool) (*ChannelMapping, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("discord channel id is required")
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	res, err := d.sql.ExecContext(ctx, `
		UPDATE channel_mappings SET enabled = ? WHERE discord_channel_id = ?
	`, enabledInt, channelID)
	if err != nil {
		return nil, fmt.Errorf("set mapping enabled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("no mapping found for channel %s", channelID)
	}
	return d.GetMappingByChannel(ctx, channelID)
}

// DeleteMapping removes a channel ↔ playlist row.
// Meat Bag: processed_videos are left alone on purpose — if you remapping
// the same playlist later, Subotto will not re-add old videos.
func (d *DB) DeleteMapping(ctx context.Context, channelID string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return fmt.Errorf("discord channel id is required")
	}

	res, err := d.sql.ExecContext(ctx, `
		DELETE FROM channel_mappings WHERE discord_channel_id = ?
	`, channelID)
	if err != nil {
		return fmt.Errorf("delete channel mapping: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no mapping found for channel %s", channelID)
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanMapping(row scannable) (*ChannelMapping, error) {
	var (
		m         ChannelMapping
		enabled   int
		createdAt string
	)
	err := row.Scan(
		&m.ID,
		&m.DiscordChannelID,
		&m.GuildID,
		&m.YouTubePlaylistID,
		&m.Name,
		&enabled,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled == 1
	m.CreatedAt = parseSQLiteTime(createdAt)
	return &m, nil
}

func parseSQLiteTime(s string) time.Time {
	// SQLite datetime('now') is usually "2006-01-02 15:04:05"
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

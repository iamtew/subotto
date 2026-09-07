package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const mappingSelectCols = `
	id, discord_channel_id, guild_id, youtube_playlist_id, name, enabled,
	created_at, active_from, active_until
`

// GetEnabledMappingByChannel returns the active+enabled mapping for a channel,
// or nil if none / disabled / closed.
func (d *DB) GetEnabledMappingByChannel(ctx context.Context, channelID string) (*ChannelMapping, error) {
	m, err := d.GetMappingByChannel(ctx, channelID)
	if err != nil || m == nil || !m.Enabled {
		return nil, err
	}
	return m, nil
}

// GetMappingByChannel returns the *active* mapping (active_until IS NULL),
// whether enabled or not. Closed epochs are ignored.
func (d *DB) GetMappingByChannel(ctx context.Context, channelID string) (*ChannelMapping, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+mappingSelectCols+`
		FROM channel_mappings
		WHERE discord_channel_id = ? AND active_until IS NULL
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

// UpsertMapping creates or updates the active mapping for a channel.
//
// Meat Bag biweekly flow:
//   - Same playlist ID → update name/guild/enabled in place (same epoch).
//   - New playlist ID (Admin "create playlist") → close the old epoch and open a new one.
func (d *DB) UpsertMapping(ctx context.Context, channelID, guildID, playlistID, name string, enabled bool) (*ChannelMapping, error) {
	channelID = strings.TrimSpace(channelID)
	playlistID = strings.TrimSpace(playlistID)
	if channelID == "" || playlistID == "" {
		return nil, fmt.Errorf("discord channel id and youtube playlist id are required")
	}

	existing, err := d.GetMappingByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	if existing != nil && existing.YouTubePlaylistID == playlistID {
		_, err := d.sql.ExecContext(ctx, `
			UPDATE channel_mappings
			SET guild_id = ?, name = ?, enabled = ?
			WHERE id = ? AND active_until IS NULL
		`, guildID, name, enabledInt, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("update channel mapping: %w", err)
		}
		return d.GetMappingByChannel(ctx, channelID)
	}

	if existing != nil {
		// Close previous epoch so resync lookback can stop at this boundary.
		if _, err := d.sql.ExecContext(ctx, `
			UPDATE channel_mappings
			SET active_until = ?, enabled = 0
			WHERE id = ? AND active_until IS NULL
		`, now, existing.ID); err != nil {
			return nil, fmt.Errorf("close previous mapping epoch: %w", err)
		}
	}

	_, err = d.sql.ExecContext(ctx, `
		INSERT INTO channel_mappings (
			discord_channel_id, guild_id, youtube_playlist_id, name, enabled,
			created_at, active_from, active_until
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
	`, channelID, guildID, playlistID, name, enabledInt, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert channel mapping: %w", err)
	}

	return d.GetMappingByChannel(ctx, channelID)
}

// ListMappings returns every *active* channel ↔ playlist row, newest first.
// Closed epochs stay in the DB for lookback but are hidden from the Admin list.
func (d *DB) ListMappings(ctx context.Context) ([]ChannelMapping, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+mappingSelectCols+`
		FROM channel_mappings
		WHERE active_until IS NULL
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

// SetMappingEnabled flips the enabled flag for the active mapping.
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
		UPDATE channel_mappings SET enabled = ?
		WHERE discord_channel_id = ? AND active_until IS NULL
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

// DeleteMapping closes the active mapping epoch (soft delete).
// Meat Bag: processed_videos stay forever so a later epoch on the same channel
// will not re-add the same videos on resync.
func (d *DB) DeleteMapping(ctx context.Context, channelID string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return fmt.Errorf("discord channel id is required")
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.ExecContext(ctx, `
		UPDATE channel_mappings
		SET active_until = ?, enabled = 0
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, now, channelID)
	if err != nil {
		return fmt.Errorf("close channel mapping: %w", err)
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

// ResyncNotBefore returns the earliest Discord message time a resync should
// consider for this channel's *current* mapping.
//
// If a previous epoch exists, we do not look further back than when that epoch
// ended (same instant the new one started). First mapping on a channel: zero
// time → resync may walk by message limit only.
func (d *DB) ResyncNotBefore(ctx context.Context, channelID string) (time.Time, error) {
	channelID = strings.TrimSpace(channelID)
	var until sql.NullString
	err := d.sql.QueryRowContext(ctx, `
		SELECT active_until FROM channel_mappings
		WHERE discord_channel_id = ? AND active_until IS NOT NULL
		ORDER BY active_until DESC, id DESC
		LIMIT 1
	`, channelID).Scan(&until)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if !until.Valid || strings.TrimSpace(until.String) == "" {
		return time.Time{}, nil
	}
	return parseSQLiteTime(until.String), nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanMapping(row scannable) (*ChannelMapping, error) {
	var (
		m          ChannelMapping
		enabled    int
		createdAt  string
		activeFrom string
		activeUntil sql.NullString
	)
	err := row.Scan(
		&m.ID,
		&m.DiscordChannelID,
		&m.GuildID,
		&m.YouTubePlaylistID,
		&m.Name,
		&enabled,
		&createdAt,
		&activeFrom,
		&activeUntil,
	)
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled == 1
	m.CreatedAt = parseSQLiteTime(createdAt)
	m.ActiveFrom = parseSQLiteTime(activeFrom)
	if activeUntil.Valid && strings.TrimSpace(activeUntil.String) != "" {
		t := parseSQLiteTime(activeUntil.String)
		m.ActiveUntil = &t
	}
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
			return t.UTC()
		}
	}
	return time.Time{}
}

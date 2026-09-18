package db

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Newest Hesh Helper rows to keep in SQLite. Older rows are deleted after each insert.
const aiRequestLogKeep = 200

const aiLogClipRunes = 2000

const (
	AILevelInfo     = "info"
	AILevelWarning  = "warning"
	AILevelError    = "error"
	AILevelCritical = "critical"
)

// AIRequestEntry is one OpenRouter attempt (or a failed attempt to call it).
type AIRequestEntry struct {
	ID          int64
	Timestamp   time.Time
	Level       string
	Trigger     string
	Author      string
	ChannelID   string
	UserMessage string
	Reply       string
	Error       string
	HTTPStatus  int
}

// ClassifyAILevel maps an OpenRouter / Discord outcome to a filter bucket.
// httpStatus comes from *ai.APIError when the HTTP call ran; 0 otherwise.
func ClassifyAILevel(httpStatus int, apiErr, discordSendErr error) string {
	if apiErr == nil {
		if discordSendErr != nil {
			return AILevelWarning
		}
		return AILevelInfo
	}
	switch httpStatus {
	case 401, 402, 403:
		return AILevelCritical
	case 429:
		return AILevelWarning
	}
	msg := strings.ToLower(apiErr.Error())
	if strings.Contains(msg, "openrouter_api_key is not set") {
		return AILevelCritical
	}
	if strings.Contains(msg, "empty reply") {
		return AILevelWarning
	}
	return AILevelError
}

func aiLevelRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case AILevelCritical:
		return 3
	case AILevelError:
		return 2
	case AILevelWarning:
		return 1
	default:
		return 0
	}
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// LogAIRequest appends one row and trims the table to the newest 200.
func (d *DB) LogAIRequest(ctx context.Context, e AIRequestEntry) error {
	if d == nil {
		return nil
	}
	level := strings.TrimSpace(e.Level)
	if level == "" {
		level = AILevelInfo
	}
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO ai_request_log (level, trigger, author, channel_id, user_message, reply, error, http_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		level,
		strings.TrimSpace(e.Trigger),
		strings.TrimSpace(e.Author),
		strings.TrimSpace(e.ChannelID),
		clipRunes(e.UserMessage, aiLogClipRunes),
		clipRunes(e.Reply, aiLogClipRunes),
		clipRunes(e.Error, aiLogClipRunes),
		e.HTTPStatus,
	)
	if err != nil {
		return fmt.Errorf("insert ai_request_log: %w", err)
	}
	// Nested subquery so SQLite allows DELETE on the same table.
	_, err = d.sql.ExecContext(ctx, `
		DELETE FROM ai_request_log WHERE id < (
			SELECT MIN(id) FROM (
				SELECT id FROM ai_request_log ORDER BY id DESC LIMIT ?
			)
		)
	`, aiRequestLogKeep)
	if err != nil {
		return fmt.Errorf("trim ai_request_log: %w", err)
	}
	return nil
}

// ListAIRequests returns newest rows at or above minLevel (all/info/warning/error/critical).
func (d *DB) ListAIRequests(ctx context.Context, minLevel string, limit int) ([]AIRequestEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	minRank := aiLevelRank(minLevel)
	if strings.EqualFold(strings.TrimSpace(minLevel), "all") {
		minRank = 0
	}

	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, timestamp, level, trigger, author, channel_id, user_message, reply, error, http_status
		FROM ai_request_log
		WHERE CASE lower(level)
			WHEN 'critical' THEN 3
			WHEN 'error' THEN 2
			WHEN 'warning' THEN 1
			ELSE 0
		END >= ?
		ORDER BY id DESC
		LIMIT ?
	`, minRank, limit)
	if err != nil {
		return nil, fmt.Errorf("list ai_request_log: %w", err)
	}
	defer rows.Close()

	var out []AIRequestEntry
	for rows.Next() {
		var (
			e  AIRequestEntry
			ts string
		)
		if err := rows.Scan(
			&e.ID, &ts, &e.Level, &e.Trigger, &e.Author, &e.ChannelID,
			&e.UserMessage, &e.Reply, &e.Error, &e.HTTPStatus,
		); err != nil {
			return nil, err
		}
		e.Timestamp = parseSQLiteTime(ts)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []AIRequestEntry{}
	}
	return out, nil
}

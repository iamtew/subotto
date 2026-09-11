package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
)

// Broadcast is a named multi-channel Discord send (standalone or show-template-bound).
// Meat Bag: fire by slug; episode-bound bodies resolve {{placeholders}} from the live episode.
type Broadcast struct {
	ID                int64
	Slug              string
	Name              string
	EpisodeTemplateID *int64 // nil = standalone
	Messages          []BroadcastMessage
	UpdatedAt         time.Time

	// TemplateShow is filled on list/get when EpisodeTemplateID is set (join, not stored).
	TemplateShow string
	TemplateSlug string
}

// BroadcastMessage is one text payload mapped to one or more Discord channel IDs.
type BroadcastMessage struct {
	ID         string   `json:"id"`
	Body       string   `json:"body"`
	ChannelIDs []string `json:"channel_ids"`
}

const broadcastSelectCols = `
	b.id, b.slug, b.name, b.episode_template_id, b.messages_json, b.updated_at,
	COALESCE(t.show_name, ''), COALESCE(t.show_slug, '')
`

// ListBroadcasts returns all broadcasts, newest first.
func (d *DB) ListBroadcasts(ctx context.Context) ([]Broadcast, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+broadcastSelectCols+`
		FROM broadcasts b
		LEFT JOIN episode_templates t ON t.id = b.episode_template_id
		ORDER BY b.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list broadcasts: %w", err)
	}
	defer rows.Close()
	var out []Broadcast
	for rows.Next() {
		b, err := scanBroadcast(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Broadcast{}
	}
	return out, nil
}

// GetBroadcastByID returns one broadcast or nil.
func (d *DB) GetBroadcastByID(ctx context.Context, id int64) (*Broadcast, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+broadcastSelectCols+`
		FROM broadcasts b
		LEFT JOIN episode_templates t ON t.id = b.episode_template_id
		WHERE b.id = ?
	`, id)
	b, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GetBroadcastBySlug returns one broadcast or nil (slug match is case-insensitive).
func (d *DB) GetBroadcastBySlug(ctx context.Context, slug string) (*Broadcast, error) {
	slug = NormalizeBroadcastSlug(slug)
	if slug == "" {
		return nil, nil
	}
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+broadcastSelectCols+`
		FROM broadcasts b
		LEFT JOIN episode_templates t ON t.id = b.episode_template_id
		WHERE lower(b.slug) = lower(?)
	`, slug)
	b, err := scanBroadcast(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// CreateBroadcast inserts a new broadcast.
func (d *DB) CreateBroadcast(ctx context.Context, name, slug string, episodeTemplateID *int64, messages []BroadcastMessage) (*Broadcast, error) {
	return d.saveBroadcast(ctx, 0, name, slug, episodeTemplateID, messages)
}

// UpdateBroadcast replaces an existing broadcast.
func (d *DB) UpdateBroadcast(ctx context.Context, id int64, name, slug string, episodeTemplateID *int64, messages []BroadcastMessage) (*Broadcast, error) {
	if id < 1 {
		return nil, fmt.Errorf("broadcast %d not found", id)
	}
	existing, err := d.GetBroadcastByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("broadcast %d not found", id)
	}
	return d.saveBroadcast(ctx, id, name, slug, episodeTemplateID, messages)
}

func (d *DB) saveBroadcast(ctx context.Context, id int64, name, slug string, episodeTemplateID *int64, messages []BroadcastMessage) (*Broadcast, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	slug = NormalizeBroadcastSlug(slug)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	if episodeTemplateID != nil && *episodeTemplateID > 0 {
		tmpl, err := d.GetEpisodeTemplateByID(ctx, *episodeTemplateID)
		if err != nil {
			return nil, err
		}
		if tmpl == nil {
			return nil, fmt.Errorf("episode template %d not found", *episodeTemplateID)
		}
	} else {
		episodeTemplateID = nil
	}
	msgs, err := normalizeBroadcastMessages(messages)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(msgs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if id == 0 {
		res, err := d.sql.ExecContext(ctx, `
			INSERT INTO broadcasts (slug, name, episode_template_id, messages_json, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, slug, name, nullInt64(episodeTemplateID), string(raw), now)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, fmt.Errorf("slug %q already in use", slug)
			}
			return nil, fmt.Errorf("insert broadcast: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		return d.GetBroadcastByID(ctx, newID)
	}
	_, err = d.sql.ExecContext(ctx, `
		UPDATE broadcasts
		SET slug = ?, name = ?, episode_template_id = ?, messages_json = ?, updated_at = ?
		WHERE id = ?
	`, slug, name, nullInt64(episodeTemplateID), string(raw), now, id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("slug %q already in use", slug)
		}
		return nil, fmt.Errorf("update broadcast: %w", err)
	}
	return d.GetBroadcastByID(ctx, id)
}

// DeleteBroadcast removes a broadcast row.
func (d *DB) DeleteBroadcast(ctx context.Context, id int64) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM broadcasts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete broadcast: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("broadcast %d not found", id)
	}
	return nil
}

// NormalizeBroadcastSlug turns operator input into a URL-safe key (hyphens).
func NormalizeBroadcastSlug(s string) string {
	return strings.ReplaceAll(Slugify(s), "_", "-")
}

// BroadcastUTF16Len matches Discord's character count (UTF-16 code units).
func BroadcastUTF16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

const broadcastMaxMessageLen = 2000

func normalizeBroadcastMessages(in []BroadcastMessage) ([]BroadcastMessage, error) {
	if in == nil {
		in = []BroadcastMessage{}
	}
	if len(in) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}
	out := make([]BroadcastMessage, 0, len(in))
	for i, m := range in {
		body := strings.TrimSpace(m.Body)
		if body == "" {
			return nil, fmt.Errorf("message %d: body is required", i)
		}
		if BroadcastUTF16Len(body) > broadcastMaxMessageLen {
			return nil, fmt.Errorf("message %d: body exceeds Discord %d-character limit", i, broadcastMaxMessageLen)
		}
		chans := uniqueNonEmpty(m.ChannelIDs)
		if len(chans) == 0 {
			return nil, fmt.Errorf("message %d: at least one channel is required", i)
		}
		id := strings.TrimSpace(m.ID)
		if id == "" {
			id = newBroadcastMessageID()
		}
		out = append(out, BroadcastMessage{ID: id, Body: body, ChannelIDs: chans})
	}
	return out, nil
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func newBroadcastMessageID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func nullInt64(p *int64) any {
	if p == nil || *p < 1 {
		return nil
	}
	return *p
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint")
}

type broadcastScanner interface {
	Scan(dest ...any) error
}

func scanBroadcast(row broadcastScanner) (*Broadcast, error) {
	var (
		b          Broadcast
		tmplID     sql.NullInt64
		msgsRaw    string
		updatedRaw string
		tmplShow   string
		tmplSlug   string
	)
	if err := row.Scan(
		&b.ID, &b.Slug, &b.Name, &tmplID, &msgsRaw, &updatedRaw, &tmplShow, &tmplSlug,
	); err != nil {
		return nil, err
	}
	if tmplID.Valid && tmplID.Int64 > 0 {
		id := tmplID.Int64
		b.EpisodeTemplateID = &id
		b.TemplateShow = tmplShow
		b.TemplateSlug = tmplSlug
	}
	msgs, err := parseBroadcastMessagesJSON(msgsRaw)
	if err != nil {
		return nil, err
	}
	b.Messages = msgs
	b.UpdatedAt = parseSQLiteTime(updatedRaw)
	return &b, nil
}

func parseBroadcastMessagesJSON(raw string) ([]BroadcastMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []BroadcastMessage{}, nil
	}
	var msgs []BroadcastMessage
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, fmt.Errorf("parse broadcast messages_json: %w", err)
	}
	if msgs == nil {
		msgs = []BroadcastMessage{}
	}
	return msgs, nil
}

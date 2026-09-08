package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Credit corners for the OBS slideshow overlay.
const (
	CreditCornerTL = "tl"
	CreditCornerTR = "tr"
	CreditCornerBL = "bl"
	CreditCornerBR = "br"
)

const pictureSelectCols = `
	id, discord_channel_id, guild_id, name, slug, enabled,
	credit_corner, interval_seconds, shuffle, show_credit, show_reactions,
	reaction_multiplier, credit_scale, reaction_scale,
	created_at, active_from, active_until
`

// PictureListener is one Discord channel → on-disk gallery *picture listener* epoch.
// Meat Bag: start/cease like a content listener, but images land under data/pictures/{slug}/.
type PictureListener struct {
	ID               int64
	DiscordChannelID string
	GuildID          string
	Name             string
	Slug             string
	Enabled          bool
	CreditCorner     string // tl, tr, bl, br
	IntervalSeconds  int
	Shuffle          bool
	ShowCredit       bool
	ShowReactions      bool
	ReactionMultiplier int // 1–25; copies of each reaction = count * multiplier
	CreditScale        float64 // author card + font size multiplier (0.5–5)
	ReactionScale      float64 // floating emoji size multiplier (0.5–5)
	CreatedAt          time.Time
	ActiveFrom         time.Time
	ActiveUntil        *time.Time // nil = currently active epoch
}

// CollectedPicture is one saved Discord image attachment.
type CollectedPicture struct {
	ID                   int64
	ListenerID           int64
	DiscordMessageID     string
	DiscordAttachmentID  string
	AuthorID             string
	AuthorDisplayName    string
	StoredPath           string // relative to PicturesDir(), e.g. "my-show/123_456.jpg"
	ContentType          string
	CollectedAt          time.Time
	Reactions            map[string]int // emoji → count
}

// PictureListenerInput is the create/update payload from Admin / CLI.
type PictureListenerInput struct {
	DiscordChannelID string
	GuildID          string
	Name             string
	Slug             string // optional; derived from Name when empty
	Enabled          bool
	CreditCorner     string
	IntervalSeconds  int
	Shuffle          bool
	ShowCredit       bool
	ShowReactions      bool
	ReactionMultiplier int
	CreditScale        float64
	ReactionScale      float64
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

// Slugify turns a listener name into a URL-safe slug for /slideshow/{slug}.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == '_' || r == '-':
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('_')
				prevDash = true
			}
		default:
			// drop
		}
	}
	out := strings.Trim(b.String(), "_-")
	out = slugSanitizer.ReplaceAllString(out, "")
	if len(out) > 64 {
		out = out[:64]
		out = strings.Trim(out, "_-")
	}
	return out
}

// NormalizeCreditCorner returns a valid corner code or "br".
func NormalizeCreditCorner(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case CreditCornerTL, CreditCornerTR, CreditCornerBL, CreditCornerBR:
		return strings.ToLower(strings.TrimSpace(c))
	default:
		return CreditCornerBR
	}
}

// NormalizeReactionMultiplier clamps to 1–25 (copies of each reaction on the overlay).
func NormalizeReactionMultiplier(n int) int {
	if n < 1 {
		return 1
	}
	if n > 25 {
		return 25
	}
	return n
}

// NormalizeOverlayScale clamps credit/reaction size multipliers to 0.5–5 in 0.25 steps.
// Empty/zero input becomes the default 1.5.
func NormalizeOverlayScale(v float64) float64 {
	if v <= 0 {
		return 1.5
	}
	// Snap to nearest 0.25
	snapped := float64(int(v*4+0.5)) / 4
	if snapped < 0.5 {
		return 0.5
	}
	if snapped > 5 {
		return 5
	}
	return snapped
}

// GetEnabledPictureListenerByChannel returns the active+enabled picture listener, or nil.
func (d *DB) GetEnabledPictureListenerByChannel(ctx context.Context, channelID string) (*PictureListener, error) {
	p, err := d.GetPictureListenerByChannel(ctx, channelID)
	if err != nil || p == nil || !p.Enabled {
		return nil, err
	}
	return p, nil
}

// GetPictureListenerByChannel returns the *active* picture listener for a channel.
func (d *DB) GetPictureListenerByChannel(ctx context.Context, channelID string) (*PictureListener, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, channelID)
	p, err := scanPictureListener(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetPictureListenerBySlug returns the *active* picture listener for a slideshow slug.
func (d *DB) GetPictureListenerBySlug(ctx context.Context, slug string) (*PictureListener, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil
	}
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE slug = ? AND active_until IS NULL
	`, slug)
	p, err := scanPictureListener(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetLatestPictureListener returns the newest active picture listener (for /slideshow/latest).
func (d *DB) GetLatestPictureListener(ctx context.Context) (*PictureListener, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE active_until IS NULL
		ORDER BY active_from DESC, id DESC
		LIMIT 1
	`)
	p, err := scanPictureListener(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpsertPictureListener starts or updates the active picture listener on a channel.
//
// Meat Bag:
//   - Same channel + same slug → update settings in place.
//   - New slug (or first start) → close any previous epoch on that channel and open a new one.
func (d *DB) UpsertPictureListener(ctx context.Context, in PictureListenerInput) (*PictureListener, error) {
	channelID := strings.TrimSpace(in.DiscordChannelID)
	if channelID == "" {
		return nil, fmt.Errorf("discord channel id is required")
	}

	name := strings.TrimSpace(in.Name)
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = Slugify(name)
	} else {
		slug = Slugify(slug)
	}
	if slug == "" {
		return nil, fmt.Errorf("name or slug is required (need a URL-safe slideshow name)")
	}
	if slug == "latest" {
		return nil, fmt.Errorf(`slug "latest" is reserved for /slideshow/latest`)
	}

	corner := NormalizeCreditCorner(in.CreditCorner)
	interval := in.IntervalSeconds
	if interval < 1 {
		interval = 8
	}
	if interval > 600 {
		interval = 600
	}

	enabledInt := boolToInt(in.Enabled)
	shuffleInt := boolToInt(in.Shuffle)
	showCreditInt := boolToInt(in.ShowCredit)
	showReactionsInt := boolToInt(in.ShowReactions)
	mult := NormalizeReactionMultiplier(in.ReactionMultiplier)
	creditScale := NormalizeOverlayScale(in.CreditScale)
	reactionScale := NormalizeOverlayScale(in.ReactionScale)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	existing, err := d.GetPictureListenerByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}

	// Same epoch: update in place when slug matches.
	if existing != nil && existing.Slug == slug {
		_, err := d.sql.ExecContext(ctx, `
			UPDATE picture_listeners
			SET guild_id = ?, name = ?, enabled = ?,
			    credit_corner = ?, interval_seconds = ?, shuffle = ?,
			    show_credit = ?, show_reactions = ?, reaction_multiplier = ?,
			    credit_scale = ?, reaction_scale = ?
			WHERE id = ? AND active_until IS NULL
		`, in.GuildID, name, enabledInt, corner, interval, shuffleInt,
			showCreditInt, showReactionsInt, mult, creditScale, reactionScale, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("update picture listener: %w", err)
		}
		return d.GetPictureListenerByChannel(ctx, channelID)
	}

	// Slug must not be taken by another *live* listener.
	other, err := d.GetPictureListenerBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if other != nil && (existing == nil || other.ID != existing.ID) {
		return nil, fmt.Errorf("slideshow slug %q is already in use by another live picture listener", slug)
	}

	if existing != nil {
		if _, err := d.sql.ExecContext(ctx, `
			UPDATE picture_listeners
			SET active_until = ?, enabled = 0
			WHERE id = ? AND active_until IS NULL
		`, now, existing.ID); err != nil {
			return nil, fmt.Errorf("close previous picture listener epoch: %w", err)
		}
	}

	_, err = d.sql.ExecContext(ctx, `
		INSERT INTO picture_listeners (
			discord_channel_id, guild_id, name, slug, enabled,
			credit_corner, interval_seconds, shuffle, show_credit, show_reactions,
			reaction_multiplier, credit_scale, reaction_scale,
			created_at, active_from, active_until
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, channelID, in.GuildID, name, slug, enabledInt,
		corner, interval, shuffleInt, showCreditInt, showReactionsInt, mult,
		creditScale, reactionScale, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert picture listener: %w", err)
	}

	if err := d.EnsurePictureListenerDir(slug); err != nil {
		return nil, err
	}

	return d.GetPictureListenerByChannel(ctx, channelID)
}

// ListPictureListeners returns every *active* picture listener, newest first.
func (d *DB) ListPictureListeners(ctx context.Context) ([]PictureListener, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE active_until IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list picture listeners: %w", err)
	}
	defer rows.Close()

	var out []PictureListener
	for rows.Next() {
		p, err := scanPictureListener(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []PictureListener{}
	}
	return out, nil
}

// SetPictureListenerEnabled flips enabled for the active picture listener on a channel.
func (d *DB) SetPictureListenerEnabled(ctx context.Context, channelID string, enabled bool) (*PictureListener, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("discord channel id is required")
	}
	res, err := d.sql.ExecContext(ctx, `
		UPDATE picture_listeners SET enabled = ?
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, boolToInt(enabled), channelID)
	if err != nil {
		return nil, fmt.Errorf("set picture listener enabled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("no picture listener found for channel %s", channelID)
	}
	return d.GetPictureListenerByChannel(ctx, channelID)
}

// UpdatePictureListenerSettings patches overlay settings for the active listener on a channel.
func (d *DB) UpdatePictureListenerSettings(ctx context.Context, channelID string, in PictureListenerInput) (*PictureListener, error) {
	channelID = strings.TrimSpace(channelID)
	existing, err := d.GetPictureListenerByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("no picture listener found for channel %s", channelID)
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = existing.Name
	}
	corner := NormalizeCreditCorner(in.CreditCorner)
	if strings.TrimSpace(in.CreditCorner) == "" {
		corner = existing.CreditCorner
	}
	interval := in.IntervalSeconds
	if interval < 1 {
		interval = existing.IntervalSeconds
	}
	if interval > 600 {
		interval = 600
	}
	mult := in.ReactionMultiplier
	if mult < 1 {
		mult = existing.ReactionMultiplier
	}
	mult = NormalizeReactionMultiplier(mult)

	creditScale := in.CreditScale
	if creditScale <= 0 {
		creditScale = existing.CreditScale
	}
	creditScale = NormalizeOverlayScale(creditScale)

	reactionScale := in.ReactionScale
	if reactionScale <= 0 {
		reactionScale = existing.ReactionScale
	}
	reactionScale = NormalizeOverlayScale(reactionScale)

	_, err = d.sql.ExecContext(ctx, `
		UPDATE picture_listeners
		SET name = ?, enabled = ?,
		    credit_corner = ?, interval_seconds = ?, shuffle = ?,
		    show_credit = ?, show_reactions = ?, reaction_multiplier = ?,
		    credit_scale = ?, reaction_scale = ?
		WHERE id = ? AND active_until IS NULL
	`, name, boolToInt(in.Enabled), corner, interval, boolToInt(in.Shuffle),
		boolToInt(in.ShowCredit), boolToInt(in.ShowReactions), mult,
		creditScale, reactionScale, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("update picture listener settings: %w", err)
	}
	return d.GetPictureListenerByChannel(ctx, channelID)
}

// DeletePictureListener closes the active picture listener epoch (soft cease).
// Collected pictures and files stay on disk (same spirit as processed_videos).
func (d *DB) DeletePictureListener(ctx context.Context, channelID string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return fmt.Errorf("discord channel id is required")
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.ExecContext(ctx, `
		UPDATE picture_listeners
		SET active_until = ?, enabled = 0
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, now, channelID)
	if err != nil {
		return fmt.Errorf("close picture listener: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no picture listener found for channel %s", channelID)
	}
	return nil
}

// PictureResyncNotBefore returns the earliest Discord message time a picture
// resync should consider for this channel's *current* picture listener.
//
// If a previous picture-listener epoch exists, we do not look further back than
// when that epoch ended. First picture listener on a channel: zero time →
// resync may walk by message limit only.
func (d *DB) PictureResyncNotBefore(ctx context.Context, channelID string) (time.Time, error) {
	channelID = strings.TrimSpace(channelID)
	var until sql.NullString
	err := d.sql.QueryRowContext(ctx, `
		SELECT active_until FROM picture_listeners
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

// CountPictureListeners returns how many *active* picture listeners exist.
func (d *DB) CountPictureListeners(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM picture_listeners WHERE active_until IS NULL`,
	).Scan(&n)
	return n, err
}

// CountEnabledPictureListeners returns how many active+enabled picture listeners exist.
func (d *DB) CountEnabledPictureListeners(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM picture_listeners
		WHERE active_until IS NULL AND enabled = 1
	`).Scan(&n)
	return n, err
}

// CountCollectedPictures returns how many images a listener has saved.
func (d *DB) CountCollectedPictures(ctx context.Context, listenerID int64) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collected_pictures WHERE listener_id = ?`, listenerID,
	).Scan(&n)
	return n, err
}

// EnsurePictureListenerDir creates data/pictures/{slug}/ if missing.
func (d *DB) EnsurePictureListenerDir(slug string) error {
	slug = Slugify(slug)
	if slug == "" {
		return fmt.Errorf("empty slug")
	}
	dir := filepath.Join(d.picturesDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create picture dir %q: %w", dir, err)
	}
	return nil
}

// AbsolutePicturePath joins PicturesDir with a relative stored_path safely.
// Returns "" if the path would escape the pictures root.
func (d *DB) AbsolutePicturePath(storedRel string) (string, error) {
	storedRel = filepath.ToSlash(strings.TrimSpace(storedRel))
	if storedRel == "" || strings.Contains(storedRel, "..") {
		return "", fmt.Errorf("invalid picture path")
	}
	absRoot, err := filepath.Abs(d.picturesDir)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(absRoot, filepath.FromSlash(storedRel))
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("picture path escapes pictures root")
	}
	return abs, nil
}

// HasCollectedAttachment reports whether this attachment was already saved for the listener.
func (d *DB) HasCollectedAttachment(ctx context.Context, listenerID int64, attachmentID string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM collected_pictures
		WHERE listener_id = ? AND discord_attachment_id = ?
	`, listenerID, attachmentID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CollectedAttachmentOnChannel looks up an attachment already filed under any
// picture listener on this Discord channel (current or previous epoch).
// Returns the listener_id that owns the row, or ok=false if never collected.
// Used to distinguish same-listener DUPE vs previous-listener OLD skips.
func (d *DB) CollectedAttachmentOnChannel(ctx context.Context, channelID, attachmentID string) (listenerID int64, ok bool, err error) {
	err = d.sql.QueryRowContext(ctx, `
		SELECT c.listener_id
		FROM collected_pictures c
		JOIN picture_listeners p ON p.id = c.listener_id
		WHERE p.discord_channel_id = ? AND c.discord_attachment_id = ?
		ORDER BY c.id ASC
		LIMIT 1
	`, channelID, attachmentID).Scan(&listenerID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return listenerID, true, nil
}

// InsertCollectedPicture records a newly saved image. reactions may be nil.
func (d *DB) InsertCollectedPicture(ctx context.Context, p CollectedPicture) (*CollectedPicture, error) {
	reactions := "{}"
	if p.Reactions != nil {
		b, err := json.Marshal(p.Reactions)
		if err != nil {
			return nil, fmt.Errorf("marshal reactions: %w", err)
		}
		reactions = string(b)
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO collected_pictures (
			listener_id, discord_message_id, discord_attachment_id,
			author_id, author_display_name, stored_path, content_type,
			collected_at, reactions_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ListenerID, p.DiscordMessageID, p.DiscordAttachmentID,
		p.AuthorID, p.AuthorDisplayName, p.StoredPath, p.ContentType,
		now, reactions)
	if err != nil {
		return nil, fmt.Errorf("insert collected picture: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetCollectedPicture(ctx, id)
}

// GetCollectedPicture loads one row by id.
func (d *DB) GetCollectedPicture(ctx context.Context, id int64) (*CollectedPicture, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, listener_id, discord_message_id, discord_attachment_id,
		       author_id, author_display_name, stored_path, content_type,
		       collected_at, reactions_json
		FROM collected_pictures WHERE id = ?
	`, id)
	return scanCollectedPicture(row)
}

// ListCollectedPicturesForListener returns images for a slideshow feed (oldest first).
func (d *DB) ListCollectedPicturesForListener(ctx context.Context, listenerID int64) ([]CollectedPicture, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, listener_id, discord_message_id, discord_attachment_id,
		       author_id, author_display_name, stored_path, content_type,
		       collected_at, reactions_json
		FROM collected_pictures
		WHERE listener_id = ?
		ORDER BY id ASC
	`, listenerID)
	if err != nil {
		return nil, fmt.Errorf("list collected pictures: %w", err)
	}
	defer rows.Close()

	var out []CollectedPicture
	for rows.Next() {
		p, err := scanCollectedPicture(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []CollectedPicture{}
	}
	return out, nil
}

// ListCollectedPicturesByMessage returns all saved attachments for a Discord message
// across any listener (used when reactions change).
func (d *DB) ListCollectedPicturesByMessage(ctx context.Context, channelID, messageID string) ([]CollectedPicture, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT c.id, c.listener_id, c.discord_message_id, c.discord_attachment_id,
		       c.author_id, c.author_display_name, c.stored_path, c.content_type,
		       c.collected_at, c.reactions_json
		FROM collected_pictures c
		INNER JOIN picture_listeners p ON p.id = c.listener_id
		WHERE p.discord_channel_id = ? AND c.discord_message_id = ?
		  AND p.active_until IS NULL
	`, channelID, messageID)
	if err != nil {
		return nil, fmt.Errorf("list pictures by message: %w", err)
	}
	defer rows.Close()

	var out []CollectedPicture
	for rows.Next() {
		p, err := scanCollectedPicture(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []CollectedPicture{}
	}
	return out, nil
}

// UpdatePictureReactions sets reactions_json for every collected row of a message
// under the active picture listener on that channel.
func (d *DB) UpdatePictureReactions(ctx context.Context, channelID, messageID string, reactions map[string]int) error {
	payload := "{}"
	if reactions != nil {
		b, err := json.Marshal(reactions)
		if err != nil {
			return fmt.Errorf("marshal reactions: %w", err)
		}
		payload = string(b)
	}
	_, err := d.sql.ExecContext(ctx, `
		UPDATE collected_pictures
		SET reactions_json = ?
		WHERE discord_message_id = ?
		  AND listener_id IN (
			SELECT id FROM picture_listeners
			WHERE discord_channel_id = ? AND active_until IS NULL
		  )
	`, payload, messageID, channelID)
	return err
}

func scanPictureListener(row scannable) (*PictureListener, error) {
	var (
		p           PictureListener
		enabled     int
		shuffle     int
		showCredit  int
		showReact   int
		createdAt   string
		activeFrom  string
		activeUntil sql.NullString
	)
	err := row.Scan(
		&p.ID,
		&p.DiscordChannelID,
		&p.GuildID,
		&p.Name,
		&p.Slug,
		&enabled,
		&p.CreditCorner,
		&p.IntervalSeconds,
		&shuffle,
		&showCredit,
		&showReact,
		&p.ReactionMultiplier,
		&p.CreditScale,
		&p.ReactionScale,
		&createdAt,
		&activeFrom,
		&activeUntil,
	)
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	p.Shuffle = shuffle == 1
	p.ShowCredit = showCredit == 1
	p.ShowReactions = showReact == 1
	p.ReactionMultiplier = NormalizeReactionMultiplier(p.ReactionMultiplier)
	p.CreditScale = NormalizeOverlayScale(p.CreditScale)
	p.ReactionScale = NormalizeOverlayScale(p.ReactionScale)
	p.CreatedAt = parseSQLiteTime(createdAt)
	p.ActiveFrom = parseSQLiteTime(activeFrom)
	if activeUntil.Valid && strings.TrimSpace(activeUntil.String) != "" {
		t := parseSQLiteTime(activeUntil.String)
		p.ActiveUntil = &t
	}
	return &p, nil
}

func scanCollectedPicture(row scannable) (*CollectedPicture, error) {
	var (
		p            CollectedPicture
		collectedAt  string
		reactionsRaw string
	)
	err := row.Scan(
		&p.ID,
		&p.ListenerID,
		&p.DiscordMessageID,
		&p.DiscordAttachmentID,
		&p.AuthorID,
		&p.AuthorDisplayName,
		&p.StoredPath,
		&p.ContentType,
		&collectedAt,
		&reactionsRaw,
	)
	if err != nil {
		return nil, err
	}
	p.CollectedAt = parseSQLiteTime(collectedAt)
	p.Reactions = map[string]int{}
	if strings.TrimSpace(reactionsRaw) != "" && reactionsRaw != "{}" {
		_ = json.Unmarshal([]byte(reactionsRaw), &p.Reactions)
	}
	return &p, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultEpisodeNameFullTemplate builds Twitch-style full titles from episode fields.
const DefaultEpisodeNameFullTemplate = "{{episode_short}} {{name}} | {{show}} | {{twitch_suffix}}"

// Episode is one live (or ceased) show episode that groups listeners.
// Meat Bag: show + episode number are immutable after create; name fields can change.
type Episode struct {
	ID               int64
	Show             string
	ShowSlug         string
	Episode          int
	Name             string
	TwitchSuffix     string
	NameFullTemplate string
	Listeners        []EpisodeListenerStub // stub snapshot for rename re-resolve
	CreatedAt        time.Time
	ActiveFrom       time.Time
	ActiveUntil      *time.Time // nil = live
}

// EpisodeListenerStub is one pre-configured listener inside an episode template.
type EpisodeListenerStub struct {
	Kind             string `json:"kind"` // content | picture
	GuildID          string `json:"guild_id"`
	DiscordChannelID string `json:"discord_channel_id"`
	Name             string `json:"name"`                      // may contain {{placeholders}}
	PlaylistTitle    string `json:"playlist_title,omitempty"` // content; may contain placeholders
	Slug             string `json:"slug,omitempty"`           // picture; may contain placeholders
}

// EpisodeTemplate is a reusable recipe: show defaults + listener stubs.
type EpisodeTemplate struct {
	ID               int64
	Show             string
	ShowSlug         string
	TwitchSuffix     string
	NameFullTemplate string
	Listeners        []EpisodeListenerStub
	UpdatedAt        time.Time
}

// ShowSlug turns a show display name into a URL key for /api/get/episode/{show}.
func ShowSlug(show string) string {
	return strings.ReplaceAll(Slugify(show), "_", "-")
}

// EpisodeShort is EP20-style.
func EpisodeShort(n int) string {
	return fmt.Sprintf("EP%d", n)
}

// EpisodeLong is "Episode 20"-style.
func EpisodeLong(n int) string {
	return fmt.Sprintf("Episode %d", n)
}

// EpisodeFieldMap builds placeholder values for template resolve (without name_full yet).
func EpisodeFieldMap(show string, episode int, name, twitchSuffix string) map[string]string {
	return map[string]string{
		"show":           show,
		"episode":        strconv.Itoa(episode),
		"episode_short":  EpisodeShort(episode),
		"episode_long":   EpisodeLong(episode),
		"name":           name,
		"twitch_suffix":  twitchSuffix,
	}
}

// ResolveTemplate replaces {{key}} placeholders. Unknown keys stay as-is.
func ResolveTemplate(tmpl string, fields map[string]string) string {
	if tmpl == "" || len(fields) == 0 {
		return tmpl
	}
	out := tmpl
	for k, v := range fields {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// ResolveEpisodeNameFull fills name_full_template from episode fields.
func ResolveEpisodeNameFull(nameFullTemplate, show string, episode int, name, twitchSuffix string) string {
	tmpl := strings.TrimSpace(nameFullTemplate)
	if tmpl == "" {
		tmpl = DefaultEpisodeNameFullTemplate
	}
	return ResolveTemplate(tmpl, EpisodeFieldMap(show, episode, name, twitchSuffix))
}

// Computed helpers on Episode.
func (e Episode) Short() string { return EpisodeShort(e.Episode) }
func (e Episode) Long() string  { return EpisodeLong(e.Episode) }
func (e Episode) NameFull() string {
	return ResolveEpisodeNameFull(e.NameFullTemplate, e.Show, e.Episode, e.Name, e.TwitchSuffix)
}

const episodeSelectCols = `
	id, show_name, show_slug, episode_num, name, twitch_suffix, name_full_template,
	listeners_json, created_at, active_from, active_until
`

// CreateEpisode opens a new live episode. Fails if show_slug already has a live one.
// stubs are snapshotted so a later name change can re-resolve listener labels.
func (d *DB) CreateEpisode(ctx context.Context, show string, episode int, name, twitchSuffix, nameFullTemplate string, stubs []EpisodeListenerStub) (*Episode, error) {
	show = strings.TrimSpace(show)
	name = strings.TrimSpace(name)
	twitchSuffix = strings.TrimSpace(twitchSuffix)
	if show == "" {
		return nil, fmt.Errorf("show is required")
	}
	if episode < 1 {
		return nil, fmt.Errorf("episode must be >= 1")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	slug := ShowSlug(show)
	if slug == "" {
		return nil, fmt.Errorf("show does not produce a usable show_slug")
	}
	tmpl := strings.TrimSpace(nameFullTemplate)
	if tmpl == "" {
		tmpl = DefaultEpisodeNameFullTemplate
	}
	if stubs == nil {
		stubs = []EpisodeListenerStub{}
	}
	stubsJSON, err := json.Marshal(stubs)
	if err != nil {
		return nil, err
	}

	existing, err := d.GetLiveEpisodeByShowSlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("show %q already has a live episode (EP%d)", show, existing.Episode)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.ExecContext(ctx, `
		INSERT INTO episodes (
			show_name, show_slug, episode_num, name, twitch_suffix, name_full_template,
			listeners_json, created_at, active_from, active_until
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, show, slug, episode, name, twitchSuffix, tmpl, string(stubsJSON), now, now)
	if err != nil {
		return nil, fmt.Errorf("insert episode: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetEpisodeByID(ctx, id)
}

// GetEpisodeByID returns any episode row (live or ceased).
func (d *DB) GetEpisodeByID(ctx context.Context, id int64) (*Episode, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+episodeSelectCols+` FROM episodes WHERE id = ?
	`, id)
	e, err := scanEpisode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetLiveEpisodeByShowSlug returns the live episode for a show slug (case-insensitive).
func (d *DB) GetLiveEpisodeByShowSlug(ctx context.Context, slug string) (*Episode, error) {
	slug = ShowSlug(slug)
	if slug == "" {
		return nil, nil
	}
	row := d.sql.QueryRowContext(ctx, `
		SELECT `+episodeSelectCols+`
		FROM episodes
		WHERE show_slug = ? AND active_until IS NULL
	`, slug)
	e, err := scanEpisode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ListLiveEpisodes returns every live episode, newest first.
func (d *DB) ListLiveEpisodes(ctx context.Context) ([]Episode, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+episodeSelectCols+`
		FROM episodes
		WHERE active_until IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Episode{}
	}
	return out, nil
}

// UpdateEpisodeMutable patches name / twitch_suffix / name_full_template only.
func (d *DB) UpdateEpisodeMutable(ctx context.Context, id int64, name, twitchSuffix, nameFullTemplate *string) (*Episode, error) {
	e, err := d.GetEpisodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("episode %d not found", id)
	}
	if e.ActiveUntil != nil {
		return nil, fmt.Errorf("episode %d is ceased", id)
	}

	newName := e.Name
	newSuffix := e.TwitchSuffix
	newTmpl := e.NameFullTemplate
	if name != nil {
		newName = strings.TrimSpace(*name)
		if newName == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
	}
	if twitchSuffix != nil {
		newSuffix = strings.TrimSpace(*twitchSuffix)
	}
	if nameFullTemplate != nil {
		newTmpl = strings.TrimSpace(*nameFullTemplate)
		if newTmpl == "" {
			newTmpl = DefaultEpisodeNameFullTemplate
		}
	}

	_, err = d.sql.ExecContext(ctx, `
		UPDATE episodes
		SET name = ?, twitch_suffix = ?, name_full_template = ?
		WHERE id = ? AND active_until IS NULL
	`, newName, newSuffix, newTmpl, id)
	if err != nil {
		return nil, fmt.Errorf("update episode: %w", err)
	}
	return d.GetEpisodeByID(ctx, id)
}

// CeaseEpisode soft-closes the episode (does not touch listeners — caller ceases those).
func (d *DB) CeaseEpisode(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.ExecContext(ctx, `
		UPDATE episodes SET active_until = ? WHERE id = ? AND active_until IS NULL
	`, now, id)
	if err != nil {
		return fmt.Errorf("cease episode: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no live episode with id %d", id)
	}
	return nil
}

// SetMappingEpisodeID links the live content listener on a channel to an episode.
func (d *DB) SetMappingEpisodeID(ctx context.Context, channelID string, episodeID int64) error {
	channelID = strings.TrimSpace(channelID)
	res, err := d.sql.ExecContext(ctx, `
		UPDATE channel_mappings SET episode_id = ?
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, episodeID, channelID)
	if err != nil {
		return fmt.Errorf("set mapping episode_id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no live content listener on channel %s", channelID)
	}
	return nil
}

// SetPictureListenerEpisodeID links the live picture listener on a channel to an episode.
func (d *DB) SetPictureListenerEpisodeID(ctx context.Context, channelID string, episodeID int64) error {
	channelID = strings.TrimSpace(channelID)
	res, err := d.sql.ExecContext(ctx, `
		UPDATE picture_listeners SET episode_id = ?
		WHERE discord_channel_id = ? AND active_until IS NULL
	`, episodeID, channelID)
	if err != nil {
		return fmt.Errorf("set picture episode_id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no live picture listener on channel %s", channelID)
	}
	return nil
}

// ClearEpisodeListenerLinks detaches all live listeners from an episode (episode_id → NULL).
// Used when absorb fails mid-way so orphans stay absorbable.
func (d *DB) ClearEpisodeListenerLinks(ctx context.Context, episodeID int64) error {
	if _, err := d.sql.ExecContext(ctx, `
		UPDATE channel_mappings SET episode_id = NULL
		WHERE episode_id = ? AND active_until IS NULL
	`, episodeID); err != nil {
		return fmt.Errorf("clear mapping episode links: %w", err)
	}
	if _, err := d.sql.ExecContext(ctx, `
		UPDATE picture_listeners SET episode_id = NULL
		WHERE episode_id = ? AND active_until IS NULL
	`, episodeID); err != nil {
		return fmt.Errorf("clear picture episode links: %w", err)
	}
	return nil
}

// ListUnlinkedMappings returns live content listeners not tied to any episode.
// Meat Bag: absorb these into a Show Episode after deploying / restarting.
func (d *DB) ListUnlinkedMappings(ctx context.Context) ([]ChannelMapping, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+mappingSelectCols+`
		FROM channel_mappings
		WHERE active_until IS NULL AND episode_id IS NULL
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list unlinked mappings: %w", err)
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

// ListUnlinkedPictureListeners returns live picture listeners not tied to any episode.
func (d *DB) ListUnlinkedPictureListeners(ctx context.Context) ([]PictureListener, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE active_until IS NULL AND episode_id IS NULL
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list unlinked picture listeners: %w", err)
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

// ListMappingsByEpisode returns live content listeners linked to an episode.
func (d *DB) ListMappingsByEpisode(ctx context.Context, episodeID int64) ([]ChannelMapping, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+mappingSelectCols+`
		FROM channel_mappings
		WHERE episode_id = ? AND active_until IS NULL
		ORDER BY id ASC
	`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list mappings by episode: %w", err)
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

// ListPictureListenersByEpisode returns live picture listeners linked to an episode.
func (d *DB) ListPictureListenersByEpisode(ctx context.Context, episodeID int64) ([]PictureListener, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+pictureSelectCols+`
		FROM picture_listeners
		WHERE episode_id = ? AND active_until IS NULL
		ORDER BY id ASC
	`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list picture listeners by episode: %w", err)
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

// ---------- Templates ----------

// ListEpisodeTemplates returns all templates, newest first.
func (d *DB) ListEpisodeTemplates(ctx context.Context) ([]EpisodeTemplate, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, show_name, show_slug, twitch_suffix, name_full_template, listeners_json, updated_at
		FROM episode_templates
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list episode templates: %w", err)
	}
	defer rows.Close()
	var out []EpisodeTemplate
	for rows.Next() {
		t, err := scanEpisodeTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []EpisodeTemplate{}
	}
	return out, nil
}

// GetEpisodeTemplateByID returns one template or nil.
func (d *DB) GetEpisodeTemplateByID(ctx context.Context, id int64) (*EpisodeTemplate, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, show_name, show_slug, twitch_suffix, name_full_template, listeners_json, updated_at
		FROM episode_templates WHERE id = ?
	`, id)
	t, err := scanEpisodeTemplate(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateEpisodeTemplate inserts a new template.
func (d *DB) CreateEpisodeTemplate(ctx context.Context, show, twitchSuffix, nameFullTemplate string, listeners []EpisodeListenerStub) (*EpisodeTemplate, error) {
	return d.saveEpisodeTemplate(ctx, 0, show, twitchSuffix, nameFullTemplate, listeners)
}

// UpdateEpisodeTemplate replaces an existing template.
func (d *DB) UpdateEpisodeTemplate(ctx context.Context, id int64, show, twitchSuffix, nameFullTemplate string, listeners []EpisodeListenerStub) (*EpisodeTemplate, error) {
	if id < 1 {
		return nil, fmt.Errorf("episode template %d not found", id)
	}
	existing, err := d.GetEpisodeTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("episode template %d not found", id)
	}
	return d.saveEpisodeTemplate(ctx, id, show, twitchSuffix, nameFullTemplate, listeners)
}

func (d *DB) saveEpisodeTemplate(ctx context.Context, id int64, show, twitchSuffix, nameFullTemplate string, listeners []EpisodeListenerStub) (*EpisodeTemplate, error) {
	show = strings.TrimSpace(show)
	if show == "" {
		return nil, fmt.Errorf("show is required")
	}
	slug := ShowSlug(show)
	if slug == "" {
		return nil, fmt.Errorf("show does not produce a usable show_slug")
	}
	tmpl := strings.TrimSpace(nameFullTemplate)
	if tmpl == "" {
		tmpl = DefaultEpisodeNameFullTemplate
	}
	if listeners == nil {
		listeners = []EpisodeListenerStub{}
	}
	if err := validateListenerStubs(listeners); err != nil {
		return nil, err
	}
	b, err := json.Marshal(listeners)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if id == 0 {
		res, err := d.sql.ExecContext(ctx, `
			INSERT INTO episode_templates (
				show_name, show_slug, twitch_suffix, name_full_template, listeners_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, show, slug, strings.TrimSpace(twitchSuffix), tmpl, string(b), now)
		if err != nil {
			return nil, fmt.Errorf("insert episode template: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		return d.GetEpisodeTemplateByID(ctx, newID)
	}
	_, err = d.sql.ExecContext(ctx, `
		UPDATE episode_templates
		SET show_name = ?, show_slug = ?, twitch_suffix = ?, name_full_template = ?,
		    listeners_json = ?, updated_at = ?
		WHERE id = ?
	`, show, slug, strings.TrimSpace(twitchSuffix), tmpl, string(b), now, id)
	if err != nil {
		return nil, fmt.Errorf("update episode template: %w", err)
	}
	return d.GetEpisodeTemplateByID(ctx, id)
}

// DeleteEpisodeTemplate removes a template row.
func (d *DB) DeleteEpisodeTemplate(ctx context.Context, id int64) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM episode_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete episode template: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("episode template %d not found", id)
	}
	return nil
}

func validateListenerStubs(stubs []EpisodeListenerStub) error {
	for i, s := range stubs {
		kind := strings.ToLower(strings.TrimSpace(s.Kind))
		if kind != "content" && kind != "picture" {
			return fmt.Errorf("listener stub %d: kind must be content or picture", i)
		}
		if strings.TrimSpace(s.DiscordChannelID) == "" {
			return fmt.Errorf("listener stub %d: discord_channel_id is required", i)
		}
		if kind == "content" && strings.TrimSpace(s.PlaylistTitle) == "" && strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("listener stub %d: content needs name or playlist_title", i)
		}
		if kind == "picture" && strings.TrimSpace(s.Name) == "" && strings.TrimSpace(s.Slug) == "" {
			return fmt.Errorf("listener stub %d: picture needs name or slug", i)
		}
	}
	return nil
}

func scanEpisode(row scannable) (*Episode, error) {
	var (
		e           Episode
		stubsRaw    string
		createdAt   string
		activeFrom  string
		activeUntil sql.NullString
	)
	err := row.Scan(
		&e.ID,
		&e.Show,
		&e.ShowSlug,
		&e.Episode,
		&e.Name,
		&e.TwitchSuffix,
		&e.NameFullTemplate,
		&stubsRaw,
		&createdAt,
		&activeFrom,
		&activeUntil,
	)
	if err != nil {
		return nil, err
	}
	e.Listeners = []EpisodeListenerStub{}
	if strings.TrimSpace(stubsRaw) != "" {
		dec := json.NewDecoder(strings.NewReader(stubsRaw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&e.Listeners); err != nil {
			return nil, fmt.Errorf("parse episode listeners_json: %w", err)
		}
	}
	e.CreatedAt = parseSQLiteTime(createdAt)
	e.ActiveFrom = parseSQLiteTime(activeFrom)
	if activeUntil.Valid && strings.TrimSpace(activeUntil.String) != "" {
		t := parseSQLiteTime(activeUntil.String)
		e.ActiveUntil = &t
	}
	return &e, nil
}

func scanEpisodeTemplate(row scannable) (*EpisodeTemplate, error) {
	var (
		t       EpisodeTemplate
		rawJSON string
		updated string
	)
	err := row.Scan(
		&t.ID,
		&t.Show,
		&t.ShowSlug,
		&t.TwitchSuffix,
		&t.NameFullTemplate,
		&rawJSON,
		&updated,
	)
	if err != nil {
		return nil, err
	}
	t.UpdatedAt = parseSQLiteTime(updated)
	t.Listeners = []EpisodeListenerStub{}
	if strings.TrimSpace(rawJSON) != "" {
		dec := json.NewDecoder(strings.NewReader(rawJSON))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t.Listeners); err != nil {
			return nil, fmt.Errorf("parse listeners_json: %w", err)
		}
	}
	return &t, nil
}

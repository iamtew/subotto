package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"subotto/internal/ai"
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

	// Background resync ticker (Admin Scheduler tab). JSON blob.
	SettingResyncScheduler = "resync_scheduler"

	// Hesh Helper system prompt (Admin AI tab). Missing/empty → default.
	SettingAISystemPrompt = "ai_system_prompt"
	// Hesh Helper Discord replies. Missing → enabled.
	SettingAIEnabled = "ai_enabled"
	// Hesh Helper Twitch replies. Missing → follow Discord ai_enabled.
	SettingAITwitchEnabled = "ai_twitch_enabled"
	// Hesh Helper OpenRouter sampling knobs (Admin AI sliders). JSON blob.
	SettingAISampling = "ai_sampling"
	// Hesh Helper active model + catalog (Admin AI tab). JSON blob.
	SettingAIModels = "ai_models"
	// Discord channel memory for Hesh Helper. Missing → off.
	SettingAIMemoryEnabled = "ai_memory_enabled"
	// How many recent channel messages to send (1–12). Missing → 8.
	SettingAIMemoryWindow = "ai_memory_window"
	// Extra Discord user IDs allowed into Admin (JSON string array). Superadmin is env-only.
	SettingAdminDiscordIDs = "admin_discord_ids"

	// Twitch IRC join targets (JSON string array of logins, no #). Empty = do not join.
	SettingTwitchChannels = "twitch_channels"
	// Legacy single join target — still read when twitch_channels is missing/empty.
	SettingTwitchChannel = "twitch_channel"
	// ponytail: bump if an operator actually joins more rooms than this.
	MaxTwitchChannels = 20
	// Authorized Twitch account login / display name (filled after OAuth).
	SettingTwitchLogin   = "twitch_login"
	SettingTwitchDisplay = "twitch_display"

	DefaultAIMemoryWindow = 8
	MinAIMemoryWindow     = 1
	MaxAIMemoryWindow     = 12

	ResyncScopeAll      = "all"
	ResyncScopeSelected = "selected"
	ResyncKindContent   = "content"
	ResyncKindPicture   = "picture"

	DefaultResyncLimit = 100
	MaxResyncLimit     = 500
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

// DefaultAISystemPrompt is used until Meat Bag saves their own copy in Admin.
const DefaultAISystemPrompt = `You are Hesh Helper, Subotto's skateboarding-aware Discord sidekick. Keep replies short (1–3 sentences), witty, and friendly. Talk skating, the show, and casual chat. Do not dump long lists unless asked.`

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

// ResyncTarget is one listener the scheduled resync may scan.
type ResyncTarget struct {
	Kind      string `json:"kind"`
	ChannelID string `json:"channel_id"`
}

// ResyncScheduler is Admin-owned ticker config (interval, amount, who).
type ResyncScheduler struct {
	IntervalHours int            `json:"interval_hours"`
	Limit         int            `json:"limit"`
	Scope         string         `json:"scope"`
	Targets       []ResyncTarget `json:"targets"`
}

// DefaultResyncScheduler is used when no app_settings row exists yet.
func DefaultResyncScheduler(envHours int) ResyncScheduler {
	if envHours < 0 {
		envHours = 0
	}
	return ResyncScheduler{
		IntervalHours: envHours,
		Limit:         DefaultResyncLimit,
		Scope:         ResyncScopeAll,
		Targets:       []ResyncTarget{},
	}
}

// Wants reports whether this config includes an enabled listener of kind/channel.
func (c ResyncScheduler) Wants(kind, channelID string) bool {
	if c.Scope != ResyncScopeSelected {
		return true
	}
	for _, t := range c.Targets {
		if t.Kind == kind && t.ChannelID == channelID {
			return true
		}
	}
	return false
}

// NormalizeResyncScheduler fills defaults and drops empty targets.
func NormalizeResyncScheduler(in ResyncScheduler) (ResyncScheduler, error) {
	if in.IntervalHours < 0 {
		return ResyncScheduler{}, fmt.Errorf("interval_hours must be >= 0")
	}
	if in.Limit <= 0 {
		in.Limit = DefaultResyncLimit
	}
	if in.Limit > MaxResyncLimit {
		return ResyncScheduler{}, fmt.Errorf("limit must be 1–%d", MaxResyncLimit)
	}
	scope := strings.TrimSpace(strings.ToLower(in.Scope))
	if scope == "" {
		scope = ResyncScopeAll
	}
	if scope != ResyncScopeAll && scope != ResyncScopeSelected {
		return ResyncScheduler{}, fmt.Errorf("scope must be %q or %q", ResyncScopeAll, ResyncScopeSelected)
	}
	out := ResyncScheduler{
		IntervalHours: in.IntervalHours,
		Limit:         in.Limit,
		Scope:         scope,
		Targets:       make([]ResyncTarget, 0, len(in.Targets)),
	}
	for _, t := range in.Targets {
		kind := strings.TrimSpace(strings.ToLower(t.Kind))
		ch := strings.TrimSpace(t.ChannelID)
		if kind == "" && ch == "" {
			continue
		}
		if kind != ResyncKindContent && kind != ResyncKindPicture {
			return ResyncScheduler{}, fmt.Errorf("target kind must be %q or %q", ResyncKindContent, ResyncKindPicture)
		}
		if ch == "" {
			return ResyncScheduler{}, fmt.Errorf("target channel_id is required")
		}
		out.Targets = append(out.Targets, ResyncTarget{Kind: kind, ChannelID: ch})
	}
	return out, nil
}

// LoadResyncScheduler reads Admin config, or envHours/all/100 when unset.
func (d *DB) LoadResyncScheduler(ctx context.Context, envHours int) (ResyncScheduler, error) {
	raw, present, err := d.getSettingPresent(ctx, SettingResyncScheduler)
	if err != nil {
		return ResyncScheduler{}, err
	}
	if !present || strings.TrimSpace(raw) == "" {
		return DefaultResyncScheduler(envHours), nil
	}
	var in ResyncScheduler
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return ResyncScheduler{}, fmt.Errorf("resync scheduler settings: %w", err)
	}
	return NormalizeResyncScheduler(in)
}

// SaveResyncScheduler writes the Admin ticker config.
func (d *DB) SaveResyncScheduler(ctx context.Context, in ResyncScheduler) (ResyncScheduler, error) {
	out, err := NormalizeResyncScheduler(in)
	if err != nil {
		return ResyncScheduler{}, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ResyncScheduler{}, err
	}
	if err := d.SetSetting(ctx, SettingResyncScheduler, string(b)); err != nil {
		return ResyncScheduler{}, err
	}
	return out, nil
}

// AISystemPrompt returns the saved Hesh Helper prompt, or the built-in default
// when the key is missing or the saved value is empty.
func (d *DB) AISystemPrompt(ctx context.Context) (string, error) {
	v, present, err := d.getSettingPresent(ctx, SettingAISystemPrompt)
	if err != nil {
		return "", err
	}
	if !present || strings.TrimSpace(v) == "" {
		return DefaultAISystemPrompt, nil
	}
	return strings.TrimSpace(v), nil
}

// AIEnabled is whether Discord addressed chat is on. Missing key → true.
func (d *DB) AIEnabled(ctx context.Context) (bool, error) {
	return d.aiFlag(ctx, SettingAIEnabled, true)
}

func (d *DB) SetAIEnabled(ctx context.Context, on bool) error {
	return d.setAIFlag(ctx, SettingAIEnabled, on)
}

// AITwitchEnabled is whether Twitch addressed chat is on.
// Missing key follows Discord so an old global off still silences Twitch.
func (d *DB) AITwitchEnabled(ctx context.Context) (bool, error) {
	v, present, err := d.getSettingPresent(ctx, SettingAITwitchEnabled)
	if err != nil {
		return false, err
	}
	if !present {
		return d.AIEnabled(ctx)
	}
	return parseAIFlag(v, true), nil
}

func (d *DB) SetAITwitchEnabled(ctx context.Context, on bool) error {
	return d.setAIFlag(ctx, SettingAITwitchEnabled, on)
}

func (d *DB) aiFlag(ctx context.Context, key string, missingDefault bool) (bool, error) {
	v, present, err := d.getSettingPresent(ctx, key)
	if err != nil {
		return false, err
	}
	if !present {
		return missingDefault, nil
	}
	return parseAIFlag(v, missingDefault), nil
}

func (d *DB) setAIFlag(ctx context.Context, key string, on bool) error {
	val := "0"
	if on {
		val = "1"
	}
	return d.SetSetting(ctx, key, val)
}

func parseAIFlag(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	default:
		return fallback
	}
}

// AIMemoryEnabled is whether Discord replies include recent channel messages.
// Missing key → false (opt-in; extra tokens).
func (d *DB) AIMemoryEnabled(ctx context.Context) (bool, error) {
	v, present, err := d.getSettingPresent(ctx, SettingAIMemoryEnabled)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func (d *DB) SetAIMemoryEnabled(ctx context.Context, on bool) error {
	val := "0"
	if on {
		val = "1"
	}
	return d.SetSetting(ctx, SettingAIMemoryEnabled, val)
}

// NormalizeAIMemoryWindow returns n if it is 1–12.
func NormalizeAIMemoryWindow(n int) (int, error) {
	if n < MinAIMemoryWindow || n > MaxAIMemoryWindow {
		return 0, fmt.Errorf("memory window must be %d–%d", MinAIMemoryWindow, MaxAIMemoryWindow)
	}
	return n, nil
}

// AIMemoryWindow is how many prior channel messages to include. Missing/junk → 8.
func (d *DB) AIMemoryWindow(ctx context.Context) (int, error) {
	v, present, err := d.getSettingPresent(ctx, SettingAIMemoryWindow)
	if err != nil {
		return 0, err
	}
	if !present || strings.TrimSpace(v) == "" {
		return DefaultAIMemoryWindow, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return DefaultAIMemoryWindow, nil
	}
	out, err := NormalizeAIMemoryWindow(n)
	if err != nil {
		return DefaultAIMemoryWindow, nil
	}
	return out, nil
}

func (d *DB) SetAIMemoryWindow(ctx context.Context, n int) error {
	out, err := NormalizeAIMemoryWindow(n)
	if err != nil {
		return err
	}
	return d.SetSetting(ctx, SettingAIMemoryWindow, fmt.Sprintf("%d", out))
}

// LoadAISampling reads Admin sampling sliders, or built-in defaults when unset.
func (d *DB) LoadAISampling(ctx context.Context) (ai.Sampling, error) {
	raw, present, err := d.getSettingPresent(ctx, SettingAISampling)
	if err != nil {
		return ai.Sampling{}, err
	}
	if !present || strings.TrimSpace(raw) == "" {
		return ai.DefaultSampling(), nil
	}
	out, err := ai.ParseSamplingJSON([]byte(raw))
	if err != nil {
		return ai.Sampling{}, fmt.Errorf("ai sampling settings: %w", err)
	}
	return out, nil
}

// SaveAISampling clamps and writes the Admin sampling sliders.
func (d *DB) SaveAISampling(ctx context.Context, in ai.Sampling) (ai.Sampling, error) {
	out := ai.NormalizeSampling(in)
	b, err := json.Marshal(out)
	if err != nil {
		return ai.Sampling{}, err
	}
	if err := d.SetSetting(ctx, SettingAISampling, string(b)); err != nil {
		return ai.Sampling{}, err
	}
	return out, nil
}

// LoadAIModels reads the Admin OpenRouter catalog. envModel is used only when the key is missing.
func (d *DB) LoadAIModels(ctx context.Context, envModel string) (ai.ModelCatalog, error) {
	raw, present, err := d.getSettingPresent(ctx, SettingAIModels)
	if err != nil {
		return ai.ModelCatalog{}, err
	}
	if !present || strings.TrimSpace(raw) == "" {
		return ai.DefaultCatalog(envModel), nil
	}
	var in ai.ModelCatalog
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return ai.ModelCatalog{}, fmt.Errorf("ai model settings: %w", err)
	}
	out, err := ai.NormalizeCatalog(in)
	if err != nil {
		return ai.ModelCatalog{}, fmt.Errorf("ai model settings: %w", err)
	}
	return out, nil
}

// SaveAIModels clamps and writes the Admin OpenRouter catalog.
func (d *DB) SaveAIModels(ctx context.Context, in ai.ModelCatalog) (ai.ModelCatalog, error) {
	out, err := ai.NormalizeCatalog(in)
	if err != nil {
		return ai.ModelCatalog{}, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ai.ModelCatalog{}, err
	}
	if err := d.SetSetting(ctx, SettingAIModels, string(b)); err != nil {
		return ai.ModelCatalog{}, err
	}
	return out, nil
}

// ExtraAdminDiscordIDs is the allowlist stored in Admin (not SUPERADMIN_DISCORD_ID).
func (d *DB) ExtraAdminDiscordIDs(ctx context.Context) ([]string, error) {
	raw, err := d.GetSetting(ctx, SettingAdminDiscordIDs)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, fmt.Errorf("admin discord ids: %w", err)
	}
	return NormalizeDiscordIDs(ids), nil
}

// SaveExtraAdminDiscordIDs writes extra operator snowflakes (JSON array).
func (d *DB) SaveExtraAdminDiscordIDs(ctx context.Context, ids []string) error {
	out := NormalizeDiscordIDs(ids)
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return d.SetSetting(ctx, SettingAdminDiscordIDs, string(b))
}

// NormalizeDiscordIDs trims, drops empties, keeps digits-only unique IDs in order.
func NormalizeDiscordIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if !IsDiscordSnowflake(id) {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// IsDiscordSnowflake is a digits-only Discord user/channel id (tests use short ids).
func IsDiscordSnowflake(id string) bool {
	if id == "" {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	return true
}

// TwitchChannels is the IRC join list (logins, no #). Missing/empty = not joining.
// Falls back to legacy twitch_channel when the JSON list has never been saved.
func (d *DB) TwitchChannels(ctx context.Context) ([]string, error) {
	raw, err := d.GetSetting(ctx, SettingTwitchChannels)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var in []string
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return nil, fmt.Errorf("twitch channels: %w", err)
		}
		return uniqueTwitchLogins(in), nil
	}
	legacy, err := d.GetSetting(ctx, SettingTwitchChannel)
	if err != nil {
		return nil, err
	}
	return uniqueTwitchLogins([]string{legacy}), nil
}

// TwitchChannel is the first joined login (legacy callers / status field).
func (d *DB) TwitchChannel(ctx context.Context) (string, error) {
	list, err := d.TwitchChannels(ctx)
	if err != nil || len(list) == 0 {
		return "", err
	}
	return list[0], nil
}

func (d *DB) SaveTwitchChannels(ctx context.Context, channels []string) error {
	out := uniqueTwitchLogins(channels)
	if len(out) > MaxTwitchChannels {
		out = out[:MaxTwitchChannels]
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	first := ""
	if len(out) > 0 {
		first = out[0]
	}
	return d.SetSettings(ctx, map[string]string{
		SettingTwitchChannels: string(b),
		SettingTwitchChannel:  first,
	})
}

func (d *DB) SetTwitchChannel(ctx context.Context, channel string) error {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return d.SaveTwitchChannels(ctx, nil)
	}
	return d.SaveTwitchChannels(ctx, []string{channel})
}

func (d *DB) AddTwitchChannel(ctx context.Context, channel string) (added bool, list []string, err error) {
	list, err = d.TwitchChannels(ctx)
	if err != nil {
		return false, nil, err
	}
	norm := uniqueTwitchLogins([]string{channel})
	if len(norm) == 0 {
		return false, list, nil
	}
	channel = norm[0]
	for _, x := range list {
		if x == channel {
			return false, list, nil
		}
	}
	if len(list) >= MaxTwitchChannels {
		return false, list, fmt.Errorf("at most %d twitch channels", MaxTwitchChannels)
	}
	list = append(list, channel)
	return true, list, d.SaveTwitchChannels(ctx, list)
}

func (d *DB) RemoveTwitchChannel(ctx context.Context, channel string) (removed bool, list []string, err error) {
	list, err = d.TwitchChannels(ctx)
	if err != nil {
		return false, nil, err
	}
	norm := uniqueTwitchLogins([]string{channel})
	if len(norm) == 0 {
		return false, list, nil
	}
	channel = norm[0]
	next := make([]string, 0, len(list))
	for _, x := range list {
		if x == channel {
			removed = true
			continue
		}
		next = append(next, x)
	}
	if !removed {
		return false, list, nil
	}
	return true, next, d.SaveTwitchChannels(ctx, next)
}

func uniqueTwitchLogins(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimPrefix(s, "#")
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// TwitchLogin is the authorized account nick (Helix login).
func (d *DB) TwitchLogin(ctx context.Context) (string, error) {
	v, err := d.GetSetting(ctx, SettingTwitchLogin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

func (d *DB) TwitchDisplay(ctx context.Context) (string, error) {
	v, err := d.GetSetting(ctx, SettingTwitchDisplay)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

func (d *DB) SetTwitchIdentity(ctx context.Context, login, display string) error {
	return d.SetSettings(ctx, map[string]string{
		SettingTwitchLogin:   strings.TrimSpace(login),
		SettingTwitchDisplay: strings.TrimSpace(display),
	})
}

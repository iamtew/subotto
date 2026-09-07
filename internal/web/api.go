package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/scheduler"
	"subotto/internal/youtube"
)

func (s *Server) registerAPI(mux *http.ServeMux) {
	// All /api routes go through basic auth.
	mux.Handle("GET /api/status", s.basicAuth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /api/mappings", s.basicAuth(http.HandlerFunc(s.handleListMappings)))
	mux.Handle("POST /api/mappings", s.basicAuth(http.HandlerFunc(s.handleUpsertMapping)))
	mux.Handle("PATCH /api/mappings/{channel}", s.basicAuth(http.HandlerFunc(s.handlePatchMapping)))
	mux.Handle("DELETE /api/mappings/{channel}", s.basicAuth(http.HandlerFunc(s.handleDeleteMapping)))
	mux.Handle("GET /api/activity", s.basicAuth(http.HandlerFunc(s.handleActivity)))
	mux.Handle("POST /api/resync", s.basicAuth(http.HandlerFunc(s.handleResync)))
}

// ---------- JSON helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// ---------- DTOs (what the browser sees) ----------

type mappingDTO struct {
	ID                int64  `json:"id"`
	DiscordChannelID  string `json:"discord_channel_id"`
	GuildID           string `json:"guild_id"`
	YouTubePlaylistID string `json:"youtube_playlist_id"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	CreatedAt         string `json:"created_at"`
}

func toMappingDTO(m db.ChannelMapping) mappingDTO {
	return mappingDTO{
		ID:                m.ID,
		DiscordChannelID:  m.DiscordChannelID,
		GuildID:           m.GuildID,
		YouTubePlaylistID: m.YouTubePlaylistID,
		Name:              m.Name,
		Enabled:           m.Enabled,
		CreatedAt:         m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type activityDTO struct {
	ID        int64          `json:"id"`
	Timestamp string         `json:"timestamp"`
	EventType string         `json:"event_type"`
	Details   map[string]any `json:"details"`
	Success   bool           `json:"success"`
}

func toActivityDTO(e db.ActivityEntry) activityDTO {
	details := map[string]any{}
	if e.Details != "" {
		_ = json.Unmarshal([]byte(e.Details), &details)
	}
	return activityDTO{
		ID:        e.ID,
		Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
		EventType: e.EventType,
		Details:   details,
		Success:   e.Success,
	}
}

// ---------- Handlers ----------

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mappings, _ := s.store.CountMappings(ctx)
	enabled, _ := s.store.CountEnabledMappings(ctx)
	activity, _ := s.store.CountActivity(ctx)

	hasTok, err := youtube.HasStoredToken(ctx, s.store)
	if err != nil {
		hasTok = false
	}

	discordOK := false
	if s.status != nil {
		discordOK = s.status.Connected()
	}

	// Default scheduler fields when not wired (tests).
	sched := scheduler.Info{Enabled: false, IntervalHours: 0}
	if s.scheduler != nil {
		sched = s.scheduler.Info()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"phase":              6,
		"started_at":         s.startedAt.Format(time.RFC3339),
		"discord_connected":  discordOK,
		"youtube_authorized": hasTok,
		"youtube_channel":    s.youtubeName,
		"mappings_total":     mappings,
		"mappings_enabled":   enabled,
		"activity_total":     activity,
		"listen_addr":        s.addr,
		"resync_interval_hours": sched.IntervalHours,
		"scheduler_enabled":     sched.Enabled,
		"scheduler_last_run_at": sched.LastRunAt,
		"scheduler_last_error":  sched.LastError,
	})
}

func (s *Server) handleListMappings(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListMappings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]mappingDTO, 0, len(list))
	for _, m := range list {
		out = append(out, toMappingDTO(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"mappings": out})
}

type upsertBody struct {
	DiscordChannelID  string `json:"discord_channel_id"`
	GuildID           string `json:"guild_id"`
	YouTubePlaylistID string `json:"youtube_playlist_id"`
	Name              string `json:"name"`
	Enabled           *bool  `json:"enabled"` // nil = default true
}

func (s *Server) handleUpsertMapping(w http.ResponseWriter, r *http.Request) {
	var body upsertBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "mapping-" + strings.TrimSpace(body.DiscordChannelID)
	}
	m, err := s.store.UpsertMapping(r.Context(),
		body.DiscordChannelID, body.GuildID, body.YouTubePlaylistID, name, enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "mapping_upserted", map[string]any{
		"channel_id":  m.DiscordChannelID,
		"playlist_id": m.YouTubePlaylistID,
		"name":        m.Name,
		"source":      "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toMappingDTO(*m))
}

type patchBody struct {
	Enabled           *bool  `json:"enabled"`
	Name              string `json:"name"`
	YouTubePlaylistID string `json:"youtube_playlist_id"`
	GuildID           string `json:"guild_id"`
}

func (s *Server) handlePatchMapping(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("channel")
	ctx := r.Context()

	existing, err := s.store.GetMappingByChannel(ctx, channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeErr(w, http.StatusNotFound, "no mapping for channel "+channelID)
		return
	}

	var body patchBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Enable/disable-only shortcut.
	if body.Enabled != nil && body.Name == "" && body.YouTubePlaylistID == "" && body.GuildID == "" {
		m, err := s.store.SetMappingEnabled(ctx, channelID, *body.Enabled)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		event := "mapping_enabled"
		if !*body.Enabled {
			event = "mapping_disabled"
		}
		_ = s.store.LogActivity(ctx, event, map[string]any{
			"channel_id": m.DiscordChannelID,
			"source":     "admin_ui",
		}, true)
		writeJSON(w, http.StatusOK, toMappingDTO(*m))
		return
	}

	name := existing.Name
	if strings.TrimSpace(body.Name) != "" {
		name = strings.TrimSpace(body.Name)
	}
	playlist := existing.YouTubePlaylistID
	if strings.TrimSpace(body.YouTubePlaylistID) != "" {
		playlist = strings.TrimSpace(body.YouTubePlaylistID)
	}
	guild := existing.GuildID
	if body.GuildID != "" {
		guild = body.GuildID
	}
	enabled := existing.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	m, err := s.store.UpsertMapping(ctx, channelID, guild, playlist, name, enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.LogActivity(ctx, "mapping_upserted", map[string]any{
		"channel_id":  m.DiscordChannelID,
		"playlist_id": m.YouTubePlaylistID,
		"name":        m.Name,
		"source":      "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toMappingDTO(*m))
}

func (s *Server) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("channel")
	ctx := r.Context()
	existing, err := s.store.GetMappingByChannel(ctx, channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.DeleteMapping(ctx, channelID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	details := map[string]any{"channel_id": channelID, "source": "admin_ui"}
	if existing != nil {
		details["playlist_id"] = existing.YouTubePlaylistID
		details["name"] = existing.Name
	}
	_ = s.store.LogActivity(ctx, "mapping_deleted", details, true)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": channelID})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			limit = n
		}
	}
	list, err := s.store.ListActivity(r.Context(), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]activityDTO, 0, len(list))
	for _, e := range list {
		out = append(out, toActivityDTO(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": out})
}

type resyncBody struct {
	ChannelID string `json:"channel_id"`
	Limit     int    `json:"limit"`
}

func (s *Server) handleResync(w http.ResponseWriter, r *http.Request) {
	var body resyncBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.ChannelID) == "" {
		writeErr(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	if s.yt == nil {
		writeErr(w, http.StatusServiceUnavailable, "YouTube client not ready")
		return
	}
	if strings.TrimSpace(s.discordTok) == "" {
		writeErr(w, http.StatusServiceUnavailable, "Discord token missing")
		return
	}

	limit := body.Limit
	if limit <= 0 {
		limit = 100
	}

	summary, err := discord.ResyncChannel(r.Context(), s.discordTok, s.store, s.yt, body.ChannelID, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"channel_id":       body.ChannelID,
		"messages_scanned": summary.MessagesScanned,
		"added":            summary.Result.Added,
		"skipped":          summary.Result.Skipped,
		"failed":           summary.Result.Failed,
	})
}

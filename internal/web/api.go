package web

import (
	"context"
	"encoding/json"
	"log/slog"
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
	// Product term: listener / listen. Legacy /api/airs and /api/mappings stay as aliases.
	mux.Handle("GET /api/listens", s.basicAuth(http.HandlerFunc(s.handleListMappings)))
	mux.Handle("POST /api/listens", s.basicAuth(http.HandlerFunc(s.handleUpsertMapping)))
	mux.Handle("PATCH /api/listens/{channel}", s.basicAuth(http.HandlerFunc(s.handlePatchMapping)))
	mux.Handle("DELETE /api/listens/{channel}", s.basicAuth(http.HandlerFunc(s.handleDeleteMapping)))
	mux.Handle("GET /api/airs", s.basicAuth(http.HandlerFunc(s.handleListMappings)))
	mux.Handle("POST /api/airs", s.basicAuth(http.HandlerFunc(s.handleUpsertMapping)))
	mux.Handle("PATCH /api/airs/{channel}", s.basicAuth(http.HandlerFunc(s.handlePatchMapping)))
	mux.Handle("DELETE /api/airs/{channel}", s.basicAuth(http.HandlerFunc(s.handleDeleteMapping)))
	mux.Handle("GET /api/mappings", s.basicAuth(http.HandlerFunc(s.handleListMappings)))
	mux.Handle("POST /api/mappings", s.basicAuth(http.HandlerFunc(s.handleUpsertMapping)))
	mux.Handle("PATCH /api/mappings/{channel}", s.basicAuth(http.HandlerFunc(s.handlePatchMapping)))
	mux.Handle("DELETE /api/mappings/{channel}", s.basicAuth(http.HandlerFunc(s.handleDeleteMapping)))
	mux.Handle("GET /api/activity", s.basicAuth(http.HandlerFunc(s.handleActivity)))
	mux.Handle("POST /api/resync", s.basicAuth(http.HandlerFunc(s.handleResync)))
	mux.Handle("GET /api/discord/guilds", s.basicAuth(http.HandlerFunc(s.handleDiscordGuilds)))
	mux.Handle("GET /api/discord/guilds/{guild}/channels", s.basicAuth(http.HandlerFunc(s.handleDiscordChannels)))
	mux.Handle("GET /api/settings/listen-messages", s.basicAuth(http.HandlerFunc(s.handleGetListenMessages)))
	mux.Handle("PUT /api/settings/listen-messages", s.basicAuth(http.HandlerFunc(s.handlePutListenMessages)))
	mux.Handle("GET /api/settings/air-messages", s.basicAuth(http.HandlerFunc(s.handleGetListenMessages)))
	mux.Handle("PUT /api/settings/air-messages", s.basicAuth(http.HandlerFunc(s.handlePutListenMessages)))
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
	ID                 int64  `json:"id"`
	DiscordChannelID   string `json:"discord_channel_id"`
	DiscordChannelName string `json:"discord_channel_name,omitempty"`
	GuildID            string `json:"guild_id"`
	YouTubePlaylistID  string `json:"youtube_playlist_id"`
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"created_at"`
	ActiveFrom         string `json:"active_from"`
	ActiveUntil        string `json:"active_until,omitempty"`
}

func toMappingDTO(m db.ChannelMapping) mappingDTO {
	dto := mappingDTO{
		ID:                m.ID,
		DiscordChannelID:  m.DiscordChannelID,
		GuildID:           m.GuildID,
		YouTubePlaylistID: m.YouTubePlaylistID,
		Name:              m.Name,
		Enabled:           m.Enabled,
		CreatedAt:         m.CreatedAt.UTC().Format(time.RFC3339),
		ActiveFrom:        m.ActiveFrom.UTC().Format(time.RFC3339),
	}
	if m.ActiveUntil != nil {
		dto.ActiveUntil = m.ActiveUntil.UTC().Format(time.RFC3339)
	}
	return dto
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

	picsTotal, _ := s.store.CountPictureListeners(ctx)
	picsEnabled, _ := s.store.CountEnabledPictureListeners(ctx)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"phase":              8,
		"started_at":         s.startedAt.Format(time.RFC3339),
		"discord_connected":  discordOK,
		"youtube_authorized": hasTok,
		"youtube_channel":    s.youtubeName,
		"mappings_total":     mappings,
		"mappings_enabled":   enabled,
		"airs_total":         mappings,
		"airs_enabled":       enabled,
		"listens_total":      mappings,
		"listens_enabled":    enabled,
		"picture_listens_total":   picsTotal,
		"picture_listens_enabled": picsEnabled,
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
	// Resolve live Discord channel names for the Admin table (best-effort).
	names := s.channelNamesForMappings(list)
	out := make([]mappingDTO, 0, len(list))
	for _, m := range list {
		dto := toMappingDTO(m)
		dto.DiscordChannelName = names[m.DiscordChannelID]
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"listens": out, "airs": out, "mappings": out})
}

// channelNamesForMappings looks up #channel names for the guilds present in
// the listen list. Meat Bag: if Discord is down we just leave names empty and
// the UI falls back to the snowflake id.
func (s *Server) channelNamesForMappings(list []db.ChannelMapping) map[string]string {
	out := map[string]string{}
	if s.discord == nil || len(list) == 0 {
		return out
	}
	seenGuild := map[string]bool{}
	for _, m := range list {
		guildID := strings.TrimSpace(m.GuildID)
		if guildID == "" || seenGuild[guildID] {
			continue
		}
		seenGuild[guildID] = true
		chs, err := s.discord.ListTextChannels(guildID)
		if err != nil {
			slog.Debug("channel name lookup skipped", "guild", guildID, "err", err)
			continue
		}
		for _, ch := range chs {
			out[ch.ID] = ch.Name
		}
	}
	return out
}

type upsertBody struct {
	DiscordChannelID     string `json:"discord_channel_id"`
	GuildID              string `json:"guild_id"`
	YouTubePlaylistID    string `json:"youtube_playlist_id"`
	YouTubePlaylistTitle string `json:"youtube_playlist_title"`
	Name                 string `json:"name"`
	Enabled              *bool  `json:"enabled"` // nil = default true
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

	channelID := strings.TrimSpace(body.DiscordChannelID)
	playlistID := strings.TrimSpace(body.YouTubePlaylistID)
	playlistTitle := strings.TrimSpace(body.YouTubePlaylistTitle)
	createdPlaylist := false

	before, err := s.store.GetMappingByChannel(r.Context(), channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Prefer create-by-title when Meat Bag gives a name (Admin UI start-listen path).
	if playlistTitle != "" {
		if s.yt == nil {
			writeErr(w, http.StatusServiceUnavailable, "YouTube client not ready")
			return
		}
		id, err := s.yt.CreatePlaylist(r.Context(), playlistTitle, "Created by Subotto for Discord channel "+channelID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		playlistID = id
		createdPlaylist = true
	}
	if playlistID == "" {
		writeErr(w, http.StatusBadRequest, "provide youtube_playlist_title (create) or youtube_playlist_id (existing)")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		if playlistTitle != "" {
			name = playlistTitle
		} else {
			name = "listen-" + channelID
		}
	}

	// One listener per channel: UpsertMapping closes any previous epoch first.
	m, err := s.store.UpsertMapping(r.Context(),
		channelID, body.GuildID, playlistID, name, enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	openedListen := before == nil || before.YouTubePlaylistID != m.YouTubePlaylistID
	if before != nil && before.YouTubePlaylistID != m.YouTubePlaylistID {
		s.announceListen(r.Context(), before, false)
	}
	if openedListen {
		s.announceListen(r.Context(), m, true)
	}

	_ = s.store.LogActivity(r.Context(), "listen_upserted", map[string]any{
		"channel_id":       m.DiscordChannelID,
		"playlist_id":      m.YouTubePlaylistID,
		"playlist_title":   playlistTitle,
		"playlist_created": createdPlaylist,
		"name":             m.Name,
		"listening":        openedListen,
		"source":           "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toMappingDTO(*m))
}

func (s *Server) handleDiscordGuilds(w http.ResponseWriter, r *http.Request) {
	if s.discord == nil {
		writeErr(w, http.StatusServiceUnavailable, "Discord catalog not available")
		return
	}
	list, err := s.discord.ListGuilds()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if list == nil {
		list = []discord.GuildInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"guilds": list})
}

func (s *Server) handleDiscordChannels(w http.ResponseWriter, r *http.Request) {
	if s.discord == nil {
		writeErr(w, http.StatusServiceUnavailable, "Discord catalog not available")
		return
	}
	guildID := r.PathValue("guild")
	list, err := s.discord.ListTextChannels(guildID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if list == nil {
		list = []discord.ChannelInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": list})
}

type patchBody struct {
	Enabled              *bool  `json:"enabled"`
	Name                 string `json:"name"`
	YouTubePlaylistID    string `json:"youtube_playlist_id"`
	YouTubePlaylistTitle string `json:"youtube_playlist_title"` // rename on YouTube (same listen epoch)
	GuildID              string `json:"guild_id"`
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
		writeErr(w, http.StatusNotFound, "no listener for channel "+channelID)
		return
	}

	var body patchBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	playlistTitle := strings.TrimSpace(body.YouTubePlaylistTitle)
	if playlistTitle != "" {
		if s.yt == nil {
			writeErr(w, http.StatusServiceUnavailable, "YouTube client not ready")
			return
		}
		if err := s.yt.UpdatePlaylistTitle(ctx, existing.YouTubePlaylistID, playlistTitle); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// Keep the listen label in sync when Meat Bag only renames the playlist.
		if strings.TrimSpace(body.Name) == "" {
			body.Name = playlistTitle
		}
		_ = s.store.LogActivity(ctx, "listen_playlist_renamed", map[string]any{
			"channel_id":  channelID,
			"playlist_id": existing.YouTubePlaylistID,
			"title":       playlistTitle,
			"source":      "admin_ui",
		}, true)
	}

	onlyEnable := body.Enabled != nil && body.Name == "" && body.YouTubePlaylistID == "" && body.GuildID == "" && playlistTitle == ""
	if onlyEnable {
		m, err := s.store.SetMappingEnabled(ctx, channelID, *body.Enabled)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		event := "listen_enabled"
		if !*body.Enabled {
			event = "listen_disabled"
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
	_ = s.store.LogActivity(ctx, "listen_upserted", map[string]any{
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
	if existing != nil {
		s.announceListen(ctx, existing, false)
	}
	details := map[string]any{"channel_id": channelID, "source": "admin_ui"}
	if existing != nil {
		details["playlist_id"] = existing.YouTubePlaylistID
		details["name"] = existing.Name
	}
	_ = s.store.LogActivity(ctx, "listen_ceased", details, true)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": channelID, "listening": false})
}

type listenMessagesBody struct {
	StartMessage string `json:"start_message"`
	StopMessage  string `json:"stop_message"`
}

func (s *Server) handleGetListenMessages(w http.ResponseWriter, r *http.Request) {
	start, stop, err := s.store.ListenAnnounceMessages(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"start_message": start,
		"stop_message":  stop,
		"placeholders":  []string{"{{name}}", "{{playlist_id}}", "{{channel_id}}"},
		"note": "Global ONLINE/OFFLINE notices for every guild/channel.",
	})
}

func (s *Server) handlePutListenMessages(w http.ResponseWriter, r *http.Request) {
	var body listenMessagesBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := s.store.SetSetting(r.Context(), db.SettingListenStartMessage, body.StartMessage); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.SetSetting(r.Context(), db.SettingListenStopMessage, body.StopMessage); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "listen_messages_updated", map[string]any{"source": "admin_ui"}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"start_message": body.StartMessage,
		"stop_message":  body.StopMessage,
	})
}

// announceListen posts the global ONLINE / OFFLINE listener notice.
func (s *Server) announceListen(ctx context.Context, listen *db.ChannelMapping, online bool) {
	if listen == nil || strings.TrimSpace(s.discordTok) == "" {
		return
	}
	start, stop, err := s.store.ListenAnnounceMessages(ctx)
	if err != nil {
		slog.Warn("could not load listen announce templates", "err", err)
		return
	}
	tmpl := stop
	event := "listen_stop_announced"
	if online {
		tmpl = start
		event = "listen_start_announced"
	}
	msg := discord.FormatListenMessage(tmpl, listen)
	if err := discord.Announce(s.discordTok, listen.DiscordChannelID, msg); err != nil {
		slog.Warn("listen announce failed", "channel", listen.DiscordChannelID, "online", online, "err", err)
		_ = s.store.LogActivity(ctx, event, map[string]any{
			"channel_id": listen.DiscordChannelID,
			"error":      err.Error(),
		}, false)
		return
	}
	_ = s.store.LogActivity(ctx, event, map[string]any{
		"channel_id": listen.DiscordChannelID,
		"name":       listen.Name,
		"listening":  online,
	}, true)
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

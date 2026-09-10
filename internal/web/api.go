package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
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
	mux.Handle("GET /api/discord/guilds/{guild}/channels/{channel}/messages", s.basicAuth(http.HandlerFunc(s.handleDiscordMessages)))
	mux.Handle("POST /api/discord/guilds/{guild}/channels/{channel}/messages", s.basicAuth(http.HandlerFunc(s.handleDiscordSendMessage)))
	mux.Handle("POST /api/discord/reconnect", s.basicAuth(http.HandlerFunc(s.handleDiscordReconnect)))
	mux.Handle("GET /api/settings/listen-messages", s.basicAuth(http.HandlerFunc(s.handleGetListenMessages)))
	mux.Handle("PUT /api/settings/listen-messages", s.basicAuth(http.HandlerFunc(s.handlePutListenMessages)))
	mux.Handle("GET /api/settings/air-messages", s.basicAuth(http.HandlerFunc(s.handleGetListenMessages)))
	mux.Handle("PUT /api/settings/air-messages", s.basicAuth(http.HandlerFunc(s.handlePutListenMessages)))
	mux.Handle("GET /api/settings/picture-listen-messages", s.basicAuth(http.HandlerFunc(s.handleGetPictureListenMessages)))
	mux.Handle("PUT /api/settings/picture-listen-messages", s.basicAuth(http.HandlerFunc(s.handlePutPictureListenMessages)))
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
		"ok":                      true,
		"phase":                   8,
		"started_at":              s.startedAt.Format(time.RFC3339),
		"discord_connected":       discordOK,
		"youtube_authorized":      hasTok,
		"youtube_channel":         s.youtubeName,
		"mappings_total":          mappings,
		"mappings_enabled":        enabled,
		"airs_total":              mappings,
		"airs_enabled":            enabled,
		"listens_total":           mappings,
		"listens_enabled":         enabled,
		"picture_listens_total":   picsTotal,
		"picture_listens_enabled": picsEnabled,
		"activity_total":          activity,
		"listen_addr":             s.addr,
		"resync_interval_hours":   sched.IntervalHours,
		"scheduler_enabled":       sched.Enabled,
		"scheduler_last_run_at":   sched.LastRunAt,
		"scheduler_last_error":    sched.LastError,
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
		s.announceListenTransition(before, m)
	} else if openedListen {
		s.announceListenTransition(nil, m)
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

// handleDiscordReconnect forces a Discord gateway Close+Open (Admin status pill).
func (s *Server) handleDiscordReconnect(w http.ResponseWriter, r *http.Request) {
	if s.status == nil {
		writeErr(w, http.StatusServiceUnavailable, "discord status provider not configured")
		return
	}
	slog.Info("admin requested discord reconnect")
	if err := s.status.Reconnect(); err != nil {
		slog.Error("discord reconnect failed", "err", err)
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"discord_connected": s.status.Connected(),
		"message":           "reconnect open succeeded — waiting for ready/resume if still down",
	})
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

// accessibleChannel checks the channel is on this guild's pruned list
// (bot has View Channel). Returns 404-style error if not.
func (s *Server) accessibleChannel(guildID, channelID string) error {
	if s.discord == nil {
		return fmt.Errorf("Discord catalog not available")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return fmt.Errorf("channel id is required")
	}
	list, err := s.discord.ListTextChannels(guildID)
	if err != nil {
		return err
	}
	for _, ch := range list {
		if ch.ID == channelID {
			return nil
		}
	}
	return errChannelNotVisible
}

var errChannelNotVisible = fmt.Errorf("channel not visible to the bot")

type chatSendBody struct {
	Content string `json:"content"`
}

func (s *Server) handleDiscordMessages(w http.ResponseWriter, r *http.Request) {
	if s.discord == nil {
		writeErr(w, http.StatusServiceUnavailable, "Discord catalog not available")
		return
	}
	guildID := r.PathValue("guild")
	channelID := r.PathValue("channel")
	if err := s.accessibleChannel(guildID, channelID); err != nil {
		status := http.StatusBadRequest
		if err == errChannelNotVisible {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	limit := 10
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			limit = n
		}
	}
	list, err := s.discord.ListRecentMessages(channelID, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if list == nil {
		list = []discord.ChatMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": list})
}

func (s *Server) handleDiscordSendMessage(w http.ResponseWriter, r *http.Request) {
	if s.discord == nil {
		writeErr(w, http.StatusServiceUnavailable, "Discord catalog not available")
		return
	}
	guildID := r.PathValue("guild")
	channelID := r.PathValue("channel")
	if err := s.accessibleChannel(guildID, channelID); err != nil {
		status := http.StatusBadRequest
		if err == errChannelNotVisible {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, discord.MaxChatUploadBytes)
	content, files, err := readChatSend(r)
	if err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		writeErr(w, status, err.Error())
		return
	}
	msg, err := s.discord.SendChannelMessage(channelID, content, files)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func readChatSend(r *http.Request) (string, []discord.ChatFile, error) {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media == "multipart/form-data" {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return "", nil, err
		}
		content := ""
		if r.MultipartForm != nil {
			content = r.FormValue("content")
		}
		var files []discord.ChatFile
		if r.MultipartForm != nil {
			for _, fh := range r.MultipartForm.File["files"] {
				if fh == nil {
					continue
				}
				name := filepath.Base(strings.TrimSpace(fh.Filename))
				src, err := fh.Open()
				if err != nil {
					return "", nil, fmt.Errorf("read %s: %w", name, err)
				}
				data, err := io.ReadAll(io.LimitReader(src, discord.MaxChatFileBytes+1))
				_ = src.Close()
				if err != nil {
					return "", nil, fmt.Errorf("read %s: %w", name, err)
				}
				files = append(files, discord.ChatFile{
					Name:        name,
					ContentType: fh.Header.Get("Content-Type"),
					Data:        data,
				})
			}
		}
		return content, files, nil
	}
	var body chatSendBody
	if err := readJSON(r, &body); err != nil {
		return "", nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return body.Content, nil, nil
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
		s.announceListenTransition(existing, nil)
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
		"note":          "Global ONLINE/OFFLINE notices for every guild/channel. Clear a box and save to silence that notice.",
		"max_chars":     db.MaxAnnounceTemplateLength,
	})
}

func (s *Server) handlePutListenMessages(w http.ResponseWriter, r *http.Request) {
	var body listenMessagesBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	start := db.NormalizeAnnounceTemplate(body.StartMessage)
	stop := db.NormalizeAnnounceTemplate(body.StopMessage)
	if err := db.ValidateAnnounceTemplate("ONLINE notice", start); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.ValidateAnnounceTemplate("OFFLINE notice", stop); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{
		db.SettingListenStartMessage: start,
		db.SettingListenStopMessage:  stop,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "listen_messages_updated", map[string]any{"source": "admin_ui"}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"start_message": start,
		"stop_message":  stop,
	})
}

const announceDiscordTimeout = 20 * time.Second

// announceListenTransition posts OFFLINE then ONLINE in order without blocking Admin HTTP.
func (s *Server) announceListenTransition(offline *db.ChannelMapping, online *db.ChannelMapping) {
	var offCopy, onCopy *db.ChannelMapping
	if offline != nil {
		c := *offline
		offCopy = &c
	}
	if online != nil {
		c := *online
		onCopy = &c
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), announceDiscordTimeout)
		defer cancel()
		if offCopy != nil {
			s.announceListen(ctx, offCopy, false)
		}
		if onCopy != nil {
			s.announceListen(ctx, onCopy, true)
		}
	}()
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
	msg := strings.TrimSpace(discord.FormatListenMessage(tmpl, listen))
	if msg == "" {
		_ = s.store.LogActivity(ctx, event, map[string]any{
			"channel_id": listen.DiscordChannelID,
			"name":       listen.Name,
			"listening":  online,
			"skipped":    "empty_notice",
		}, true)
		return
	}
	if err := discord.Announce(ctx, s.discordTok, listen.DiscordChannelID, msg); err != nil {
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

func (s *Server) handleGetPictureListenMessages(w http.ResponseWriter, r *http.Request) {
	start, stop, err := s.store.PictureListenAnnounceMessages(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"start_message": start,
		"stop_message":  stop,
		"placeholders":  []string{"{{name}}", "{{slug}}", "{{channel_id}}"},
		"note":          "Global ONLINE/OFFLINE notices for every picture listener. Clear a box and save to silence that notice.",
		"max_chars":     db.MaxAnnounceTemplateLength,
	})
}

func (s *Server) handlePutPictureListenMessages(w http.ResponseWriter, r *http.Request) {
	var body listenMessagesBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	start := db.NormalizeAnnounceTemplate(body.StartMessage)
	stop := db.NormalizeAnnounceTemplate(body.StopMessage)
	if err := db.ValidateAnnounceTemplate("ONLINE notice", start); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.ValidateAnnounceTemplate("OFFLINE notice", stop); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{
		db.SettingPictureListenStartMessage: start,
		db.SettingPictureListenStopMessage:  stop,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "picture_listen_messages_updated", map[string]any{"source": "admin_ui"}, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"start_message": start,
		"stop_message":  stop,
	})
}

// announcePictureTransition posts OFFLINE then ONLINE in order without blocking Admin HTTP.
func (s *Server) announcePictureTransition(offline *db.PictureListener, online *db.PictureListener) {
	var offCopy, onCopy *db.PictureListener
	if offline != nil {
		c := *offline
		offCopy = &c
	}
	if online != nil {
		c := *online
		onCopy = &c
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), announceDiscordTimeout)
		defer cancel()
		if offCopy != nil {
			s.announcePicture(ctx, offCopy, false)
		}
		if onCopy != nil {
			s.announcePicture(ctx, onCopy, true)
		}
	}()
}

// announcePicture posts the global ONLINE / OFFLINE picture-listener notice.
func (s *Server) announcePicture(ctx context.Context, listen *db.PictureListener, online bool) {
	if listen == nil || strings.TrimSpace(s.discordTok) == "" {
		return
	}
	start, stop, err := s.store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		slog.Warn("could not load picture listen announce templates", "err", err)
		return
	}
	tmpl := stop
	event := "picture_listen_stop_announced"
	if online {
		tmpl = start
		event = "picture_listen_start_announced"
	}
	msg := strings.TrimSpace(discord.FormatPictureListenMessage(tmpl, listen))
	if msg == "" {
		_ = s.store.LogActivity(ctx, event, map[string]any{
			"channel_id": listen.DiscordChannelID,
			"name":       listen.Name,
			"slug":       listen.Slug,
			"listening":  online,
			"skipped":    "empty_notice",
		}, true)
		return
	}
	if err := discord.Announce(ctx, s.discordTok, listen.DiscordChannelID, msg); err != nil {
		slog.Warn("picture listen announce failed", "channel", listen.DiscordChannelID, "online", online, "err", err)
		_ = s.store.LogActivity(ctx, event, map[string]any{
			"channel_id": listen.DiscordChannelID,
			"error":      err.Error(),
		}, false)
		return
	}
	_ = s.store.LogActivity(ctx, event, map[string]any{
		"channel_id": listen.DiscordChannelID,
		"name":       listen.Name,
		"slug":       listen.Slug,
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

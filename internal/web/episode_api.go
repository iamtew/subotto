package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"subotto/internal/db"
)

func (s *Server) registerEpisodeAPI(mux *http.ServeMux) {
	mux.Handle("GET /api/episodes", s.basicAuth(http.HandlerFunc(s.handleListEpisodes)))
	mux.Handle("GET /api/episodes/unlinked", s.basicAuth(http.HandlerFunc(s.handleListUnlinkedListeners)))
	mux.Handle("POST /api/episodes", s.basicAuth(http.HandlerFunc(s.handleStartEpisode)))
	mux.Handle("POST /api/episodes/absorb", s.basicAuth(http.HandlerFunc(s.handleAbsorbEpisode)))
	mux.Handle("PATCH /api/episodes/{id}", s.basicAuth(http.HandlerFunc(s.handlePatchEpisode)))
	mux.Handle("DELETE /api/episodes/{id}", s.basicAuth(http.HandlerFunc(s.handleCeaseEpisode)))

	mux.Handle("GET /api/episode-templates", s.basicAuth(http.HandlerFunc(s.handleListEpisodeTemplates)))
	mux.Handle("POST /api/episode-templates", s.basicAuth(http.HandlerFunc(s.handleCreateEpisodeTemplate)))
	mux.Handle("PUT /api/episode-templates/{id}", s.basicAuth(http.HandlerFunc(s.handleUpdateEpisodeTemplate)))
	mux.Handle("DELETE /api/episode-templates/{id}", s.basicAuth(http.HandlerFunc(s.handleDeleteEpisodeTemplate)))
}

func (s *Server) createPlaylist(ctx context.Context, title, description string) (string, error) {
	if s.createPlaylistFn != nil {
		return s.createPlaylistFn(ctx, title, description)
	}
	if s.yt == nil {
		return "", fmt.Errorf("YouTube client not ready")
	}
	return s.yt.CreatePlaylist(ctx, title, description)
}

// ---------- DTOs ----------

type episodeListenerDTO struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Channel          string `json:"channel,omitempty"`
	PlaylistID       string `json:"playlist_id,omitempty"`
	Slideshow        string `json:"slideshow,omitempty"`
	State            string `json:"state"`
	DiscordChannelID string `json:"discord_channel_id,omitempty"`
}

type episodeDTO struct {
	ID               int64                `json:"id"`
	Show             string               `json:"show"`
	ShowSlug         string               `json:"show_slug"`
	Episode          int                  `json:"episode"`
	EpisodeShort     string               `json:"episode_short"`
	EpisodeLong      string               `json:"episode_long"`
	Name             string               `json:"name"`
	TwitchSuffix     string               `json:"twitch_suffix"`
	NameFullTemplate string               `json:"name_full_template"`
	EpisodeNameFull  string               `json:"episode_name_full"`
	Since            string               `json:"since"`
	State            string               `json:"state"` // live
	Listeners        []episodeListenerDTO `json:"listeners"`
	PublicURL        string               `json:"public_url"`
}

func toEpisodeDTO(e db.Episode, listeners []episodeListenerDTO) episodeDTO {
	return episodeDTO{
		ID:               e.ID,
		Show:             e.Show,
		ShowSlug:         e.ShowSlug,
		Episode:          e.Episode,
		EpisodeShort:     e.Short(),
		EpisodeLong:      e.Long(),
		Name:             e.Name,
		TwitchSuffix:     e.TwitchSuffix,
		NameFullTemplate: e.NameFullTemplate,
		EpisodeNameFull:  e.NameFull(),
		Since:            e.ActiveFrom.UTC().Format(time.RFC3339),
		State:            "live",
		Listeners:        listeners,
		PublicURL:        "/api/get/episode/" + e.ShowSlug,
	}
}

type episodeTemplateDTO struct {
	ID               int64                    `json:"id"`
	Show             string                   `json:"show"`
	ShowSlug         string                   `json:"show_slug"`
	TwitchSuffix     string                   `json:"twitch_suffix"`
	NameFullTemplate string                   `json:"name_full_template"`
	Listeners        []db.EpisodeListenerStub `json:"listeners"`
	UpdatedAt        string                   `json:"updated_at"`
}

func toEpisodeTemplateDTO(t db.EpisodeTemplate) episodeTemplateDTO {
	listeners := t.Listeners
	if listeners == nil {
		listeners = []db.EpisodeListenerStub{}
	}
	return episodeTemplateDTO{
		ID:               t.ID,
		Show:             t.Show,
		ShowSlug:         t.ShowSlug,
		TwitchSuffix:     t.TwitchSuffix,
		NameFullTemplate: t.NameFullTemplate,
		Listeners:        listeners,
		UpdatedAt:        t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) episodeListenersDTO(ctx context.Context, episodeID int64) ([]episodeListenerDTO, error) {
	maps, err := s.store.ListMappingsByEpisode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	pics, err := s.store.ListPictureListenersByEpisode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if len(maps) > 0 {
		for k, v := range s.channelNamesForMappings(maps) {
			names[k] = v
		}
	}
	if len(pics) > 0 {
		for k, v := range s.channelNamesForPictureListeners(pics) {
			names[k] = v
		}
	}

	out := make([]episodeListenerDTO, 0, len(maps)+len(pics))
	for _, m := range maps {
		out = append(out, episodeListenerDTO{
			Kind:             "content",
			Name:             m.Name,
			Channel:          names[m.DiscordChannelID],
			PlaylistID:       m.YouTubePlaylistID,
			State:            listenerState(m.Enabled),
			DiscordChannelID: m.DiscordChannelID,
		})
	}
	for _, p := range pics {
		out = append(out, episodeListenerDTO{
			Kind:             "picture",
			Name:             p.Name,
			Channel:          names[p.DiscordChannelID],
			Slideshow:        "/slideshow/" + p.Slug,
			State:            listenerState(p.Enabled),
			DiscordChannelID: p.DiscordChannelID,
		})
	}
	return out, nil
}

// publicEpisodeListeners strips Discord snowflakes for Streamer.bot.
func publicEpisodeListeners(in []episodeListenerDTO) []episodeListenerDTO {
	out := make([]episodeListenerDTO, 0, len(in))
	for _, l := range in {
		out = append(out, episodeListenerDTO{
			Kind:       l.Kind,
			Name:       l.Name,
			Channel:    l.Channel,
			PlaylistID: l.PlaylistID,
			Slideshow:  l.Slideshow,
			State:      l.State,
		})
	}
	return out
}

// ---------- Episodes ----------

func (s *Server) handleListEpisodes(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListLiveEpisodes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]episodeDTO, 0, len(list))
	for _, e := range list {
		listeners, err := s.episodeListenersDTO(r.Context(), e.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, toEpisodeDTO(e, listeners))
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": out})
}

type startEpisodeBody struct {
	TemplateID   int64   `json:"template_id"`
	Episode      int     `json:"episode"`
	Name         string  `json:"name"`
	TwitchSuffix *string `json:"twitch_suffix"` // nil = use template default
}

func (s *Server) handleStartEpisode(w http.ResponseWriter, r *http.Request) {
	var body startEpisodeBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.TemplateID < 1 {
		writeErr(w, http.StatusBadRequest, "template_id is required")
		return
	}
	if body.Episode < 1 {
		writeErr(w, http.StatusBadRequest, "episode must be >= 1")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	tmpl, err := s.store.GetEpisodeTemplateByID(r.Context(), body.TemplateID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tmpl == nil {
		writeErr(w, http.StatusNotFound, "episode template not found")
		return
	}

	suffix := tmpl.TwitchSuffix
	if body.TwitchSuffix != nil {
		suffix = strings.TrimSpace(*body.TwitchSuffix)
	}

	ep, err := s.store.CreateEpisode(r.Context(), tmpl.Show, body.Episode, name, suffix, tmpl.NameFullTemplate, tmpl.Listeners)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	fields := db.EpisodeFieldMap(ep.Show, ep.Episode, ep.Name, ep.TwitchSuffix)
	fields["episode_name_full"] = ep.NameFull()

	var started []episodeStartedStub
	for i, stub := range tmpl.Listeners {
		kind, channelID, err := s.startEpisodeStub(r.Context(), ep, stub, fields)
		if err != nil {
			if channelID != "" {
				started = append(started, episodeStartedStub{kind: kind, channelID: channelID})
			}
			s.rollbackStartedStubs(r.Context(), started)
			_ = s.store.CeaseEpisode(r.Context(), ep.ID)
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("listener stub %d: %v", i, err))
			return
		}
		started = append(started, episodeStartedStub{kind: kind, channelID: channelID})
	}

	listeners, err := s.episodeListenersDTO(r.Context(), ep.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "episode_started", map[string]any{
		"episode_id": ep.ID,
		"show":       ep.Show,
		"episode":    ep.Episode,
		"name":       ep.Name,
		"source":     "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toEpisodeDTO(*ep, listeners))
}

// absorbEpisodeBody links already-live orphan listeners into an existing live episode.
// Meat Bag: start the episode first, then absorb PROD orphans into it (no new playlists / no cease).
type absorbEpisodeBody struct {
	EpisodeID int64 `json:"episode_id"`
}

func (s *Server) handleAbsorbEpisode(w http.ResponseWriter, r *http.Request) {
	var body absorbEpisodeBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.EpisodeID < 1 {
		writeErr(w, http.StatusBadRequest, "episode_id is required — pick the live show episode to absorb into")
		return
	}

	ep, err := s.store.GetEpisodeByID(r.Context(), body.EpisodeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ep == nil || ep.ActiveUntil != nil {
		writeErr(w, http.StatusNotFound, "no live episode with that id")
		return
	}

	maps, err := s.store.ListUnlinkedMappings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pics, err := s.store.ListUnlinkedPictureListeners(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(maps) == 0 && len(pics) == 0 {
		writeErr(w, http.StatusBadRequest, "no unlinked live listeners to absorb")
		return
	}

	for _, m := range maps {
		if err := s.store.SetMappingEpisodeID(r.Context(), m.DiscordChannelID, ep.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "link content listener: "+err.Error())
			return
		}
	}
	for _, p := range pics {
		if err := s.store.SetPictureListenerEpisodeID(r.Context(), p.DiscordChannelID, ep.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "link picture listener: "+err.Error())
			return
		}
	}

	listeners, err := s.episodeListenersDTO(r.Context(), ep.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "episode_absorbed", map[string]any{
		"episode_id":     ep.ID,
		"show":           ep.Show,
		"episode":        ep.Episode,
		"name":           ep.Name,
		"content_linked": len(maps),
		"picture_linked": len(pics),
		"source":         "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toEpisodeDTO(*ep, listeners))
}

func (s *Server) handleListUnlinkedListeners(w http.ResponseWriter, r *http.Request) {
	maps, err := s.store.ListUnlinkedMappings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pics, err := s.store.ListUnlinkedPictureListeners(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	content := make([]map[string]string, 0, len(maps))
	for _, m := range maps {
		content = append(content, map[string]string{
			"discord_channel_id": m.DiscordChannelID,
			"name":               m.Name,
			"playlist_id":        m.YouTubePlaylistID,
		})
	}
	picture := make([]map[string]string, 0, len(pics))
	for _, p := range pics {
		picture = append(picture, map[string]string{
			"discord_channel_id": p.DiscordChannelID,
			"name":               p.Name,
			"slug":               p.Slug,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content": content,
		"picture": picture,
		"total":   len(maps) + len(pics),
	})
}

func (s *Server) startEpisodeStub(ctx context.Context, ep *db.Episode, stub db.EpisodeListenerStub, fields map[string]string) (kind, channelID string, err error) {
	kind = strings.ToLower(strings.TrimSpace(stub.Kind))
	channelID = strings.TrimSpace(stub.DiscordChannelID)
	guildID := strings.TrimSpace(stub.GuildID)
	resolvedName := strings.TrimSpace(db.ResolveTemplate(stub.Name, fields))

	switch kind {
	case "content":
		title := strings.TrimSpace(db.ResolveTemplate(stub.PlaylistTitle, fields))
		if title == "" {
			title = resolvedName
		}
		if title == "" {
			return "", "", fmt.Errorf("content stub needs name or playlist_title")
		}
		if resolvedName == "" {
			resolvedName = title
		}

		before, err := s.store.GetMappingByChannel(ctx, channelID)
		if err != nil {
			return "", "", err
		}
		playlistID, err := s.createPlaylist(ctx, title, "Created by Subotto for episode "+ep.ShowSlug)
		if err != nil {
			return "", "", err
		}
		m, err := s.store.UpsertMapping(ctx, channelID, guildID, playlistID, resolvedName, true)
		if err != nil {
			return "", "", err
		}
		if err := s.store.SetMappingEpisodeID(ctx, channelID, ep.ID); err != nil {
			return kind, channelID, err // channel live — caller must rollback
		}
		opened := before == nil || before.YouTubePlaylistID != m.YouTubePlaylistID
		if before != nil && before.YouTubePlaylistID != m.YouTubePlaylistID {
			s.announceListenTransition(before, m)
		} else if opened {
			s.announceListenTransition(nil, m)
		}
		return kind, channelID, nil

	case "picture":
		slug := strings.TrimSpace(db.ResolveTemplate(stub.Slug, fields))
		if resolvedName == "" {
			resolvedName = slug
		}
		if resolvedName == "" && slug == "" {
			return "", "", fmt.Errorf("picture stub needs name or slug")
		}
		before, err := s.store.GetPictureListenerByChannel(ctx, channelID)
		if err != nil {
			return "", "", err
		}
		p, err := s.store.UpsertPictureListener(ctx, db.PictureListenerInput{
			DiscordChannelID:  channelID,
			GuildID:           guildID,
			Name:              resolvedName,
			Slug:              slug,
			Enabled:           true,
			ShowCredit:        true,
			ShowReactions:     true,
			ReactionsAnimated: true,
		})
		if err != nil {
			return "", "", err
		}
		if err := s.store.SetPictureListenerEpisodeID(ctx, channelID, ep.ID); err != nil {
			return kind, channelID, err
		}
		opened := before == nil || before.Slug != p.Slug
		if before != nil && before.Slug != p.Slug {
			s.announcePictureTransition(before, p)
		} else if opened {
			s.announcePictureTransition(nil, p)
		}
		return kind, channelID, nil

	default:
		return "", "", fmt.Errorf("kind must be content or picture")
	}
}

// episodeStartedStub tracks a listener channel opened during episode start.
type episodeStartedStub struct {
	kind      string
	channelID string
}

// rollbackStartedStubs ceases channels touched during a failed episode start,
// even if episode_id was never set (upsert-before-link orphans).
func (s *Server) rollbackStartedStubs(ctx context.Context, started []episodeStartedStub) {
	for _, it := range started {
		switch it.kind {
		case "content":
			existing, _ := s.store.GetMappingByChannel(ctx, it.channelID)
			if existing == nil {
				continue
			}
			_ = s.store.DeleteMapping(ctx, it.channelID)
			s.announceListenTransition(existing, nil)
		case "picture":
			existing, _ := s.store.GetPictureListenerByChannel(ctx, it.channelID)
			if existing == nil {
				continue
			}
			_ = s.store.DeletePictureListener(ctx, it.channelID)
			s.announcePictureTransition(existing, nil)
		}
	}
}

type patchEpisodeBody struct {
	Name             *string `json:"name"`
	TwitchSuffix     *string `json:"twitch_suffix"`
	NameFullTemplate *string `json:"name_full_template"`
}

func (s *Server) handlePatchEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid episode id")
		return
	}
	var body patchEpisodeBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Name == nil && body.TwitchSuffix == nil && body.NameFullTemplate == nil {
		writeErr(w, http.StatusBadRequest, "provide name, twitch_suffix, and/or name_full_template")
		return
	}

	ep, err := s.store.UpdateEpisodeMutable(r.Context(), id, body.Name, body.TwitchSuffix, body.NameFullTemplate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Re-resolve linked listener labels from snapshotted stubs (Meat Bag renames mid-show).
	if err := s.syncEpisodeListenersFromStubs(r.Context(), ep); err != nil {
		writeErr(w, http.StatusBadRequest, "episode saved but listener sync failed: "+err.Error())
		return
	}
	listeners, err := s.episodeListenersDTO(r.Context(), ep.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "episode_updated", map[string]any{
		"episode_id": ep.ID,
		"show":       ep.Show,
		"episode":    ep.Episode,
		"name":       ep.Name,
		"source":     "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, toEpisodeDTO(*ep, listeners))
}

// syncEpisodeListenersFromStubs re-applies stub name/playlist templates after an episode rename.
// Picture slideshow slug stays put (OBS URLs). Content playlist title is renamed on YouTube when possible.
func (s *Server) syncEpisodeListenersFromStubs(ctx context.Context, ep *db.Episode) error {
	if ep == nil || len(ep.Listeners) == 0 {
		return nil
	}
	fields := db.EpisodeFieldMap(ep.Show, ep.Episode, ep.Name, ep.TwitchSuffix)
	fields["episode_name_full"] = ep.NameFull()

	maps, err := s.store.ListMappingsByEpisode(ctx, ep.ID)
	if err != nil {
		return err
	}
	pics, err := s.store.ListPictureListenersByEpisode(ctx, ep.ID)
	if err != nil {
		return err
	}
	contentByCh := map[string]db.ChannelMapping{}
	for _, m := range maps {
		contentByCh[m.DiscordChannelID] = m
	}
	picByCh := map[string]db.PictureListener{}
	for _, p := range pics {
		picByCh[p.DiscordChannelID] = p
	}

	for _, stub := range ep.Listeners {
		kind := strings.ToLower(strings.TrimSpace(stub.Kind))
		ch := strings.TrimSpace(stub.DiscordChannelID)
		resolvedName := strings.TrimSpace(db.ResolveTemplate(stub.Name, fields))
		switch kind {
		case "content":
			m, ok := contentByCh[ch]
			if !ok {
				continue
			}
			if resolvedName == "" {
				resolvedName = m.Name
			}
			title := strings.TrimSpace(db.ResolveTemplate(stub.PlaylistTitle, fields))
			if title == "" {
				title = resolvedName
			}
			if title != "" && s.yt != nil {
				_ = s.yt.UpdatePlaylistTitle(ctx, m.YouTubePlaylistID, title)
			}
			if _, err := s.store.UpsertMapping(ctx, ch, m.GuildID, m.YouTubePlaylistID, resolvedName, m.Enabled); err != nil {
				return fmt.Errorf("content %s: %w", ch, err)
			}
			_ = s.store.SetMappingEpisodeID(ctx, ch, ep.ID)
		case "picture":
			p, ok := picByCh[ch]
			if !ok {
				continue
			}
			if resolvedName == "" {
				resolvedName = p.Name
			}
			in := db.PictureListenerInput{
				Name:               resolvedName,
				Enabled:            p.Enabled,
				CreditCorner:       p.CreditCorner,
				IntervalSeconds:    p.IntervalSeconds,
				Shuffle:            p.Shuffle,
				ShowCredit:         p.ShowCredit,
				ShowReactions:      p.ShowReactions,
				ReactionsAnimated:  p.ReactionsAnimated,
				ReactionMultiplier: p.ReactionMultiplier,
				CreditScale:        p.CreditScale,
				ReactionScale:      p.ReactionScale,
				Transition:         p.Transition,
			}
			if _, err := s.store.UpdatePictureListenerSettings(ctx, ch, in); err != nil {
				return fmt.Errorf("picture %s: %w", ch, err)
			}
		}
	}
	return nil
}

func (s *Server) handleCeaseEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid episode id")
		return
	}
	ep, err := s.store.GetEpisodeByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ep == nil || ep.ActiveUntil != nil {
		writeErr(w, http.StatusNotFound, "no live episode with that id")
		return
	}

	if err := s.ceaseEpisodeListeners(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.CeaseEpisode(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.LogActivity(r.Context(), "episode_ceased", map[string]any{
		"episode_id": id,
		"show":       ep.Show,
		"episode":    ep.Episode,
		"source":     "admin_ui",
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) ceaseEpisodeListeners(ctx context.Context, episodeID int64) error {
	maps, err := s.store.ListMappingsByEpisode(ctx, episodeID)
	if err != nil {
		return err
	}
	pics, err := s.store.ListPictureListenersByEpisode(ctx, episodeID)
	if err != nil {
		return err
	}
	for i := range maps {
		m := maps[i]
		if err := s.store.DeleteMapping(ctx, m.DiscordChannelID); err != nil {
			return err
		}
		s.announceListenTransition(&m, nil)
	}
	for i := range pics {
		p := pics[i]
		if err := s.store.DeletePictureListener(ctx, p.DiscordChannelID); err != nil {
			return err
		}
		s.announcePictureTransition(&p, nil)
	}
	return nil
}

// ---------- Templates ----------

func (s *Server) handleListEpisodeTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListEpisodeTemplates(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]episodeTemplateDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toEpisodeTemplateDTO(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

type episodeTemplateBody struct {
	Show             string                   `json:"show"`
	TwitchSuffix     string                   `json:"twitch_suffix"`
	NameFullTemplate string                   `json:"name_full_template"`
	Listeners        []db.EpisodeListenerStub `json:"listeners"`
}

func (s *Server) handleCreateEpisodeTemplate(w http.ResponseWriter, r *http.Request) {
	var body episodeTemplateBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := s.store.CreateEpisodeTemplate(r.Context(), body.Show, body.TwitchSuffix, body.NameFullTemplate, body.Listeners)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEpisodeTemplateDTO(*t))
}

func (s *Server) handleUpdateEpisodeTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid template id")
		return
	}
	var body episodeTemplateBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	t, err := s.store.UpdateEpisodeTemplate(r.Context(), id, body.Show, body.TwitchSuffix, body.NameFullTemplate, body.Listeners)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEpisodeTemplateDTO(*t))
}

func (s *Server) handleDeleteEpisodeTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid template id")
		return
	}
	if err := s.store.DeleteEpisodeTemplate(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

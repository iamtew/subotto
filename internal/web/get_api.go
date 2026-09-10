package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// publicListenerDTO is the compact Streamer.bot payload for
// GET /api/get/{content|picture}/{channel-name}.
// Meat Bag: public (no Basic Auth), one open epoch only.
type publicListenerDTO struct {
	Name       string `json:"name"`
	Channel    string `json:"channel"`
	PlaylistID string `json:"playlist_id,omitempty"`
	Slideshow  string `json:"slideshow,omitempty"`
	Since      string `json:"since"`
	State      string `json:"state"` // listening | paused
}

// resolvedChannel is a Discord text channel found by name.
type resolvedChannel struct {
	ID   string
	Name string // normalized, no leading #
}

func (s *Server) handlePublicGetListener(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(strings.TrimSpace(r.PathValue("kind")))
	rawName := strings.TrimSpace(r.PathValue("channel"))
	if rawName == "" {
		writeErr(w, http.StatusBadRequest, "channel name is required")
		return
	}
	wantName := normalizeChannelName(rawName)

	switch kind {
	case "content", "picture":
		// ok
	default:
		writeErr(w, http.StatusBadRequest, "kind must be content or picture")
		return
	}

	matches, err := s.resolveDiscordChannelsByName(wantName)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if len(matches) == 0 {
		writeErr(w, http.StatusNotFound, "discord channel not found: "+wantName)
		return
	}

	ctx := r.Context()
	switch kind {
	case "content":
		s.respondPublicContent(w, ctx, matches)
	case "picture":
		s.respondPublicPicture(w, ctx, matches)
	}
}

func (s *Server) respondPublicContent(w http.ResponseWriter, ctx context.Context, matches []resolvedChannel) {
	for _, ch := range matches {
		m, err := s.store.GetMappingByChannel(ctx, ch.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if m == nil {
			continue
		}
		writeJSON(w, http.StatusOK, publicListenerDTO{
			Name:       m.Name,
			Channel:    ch.Name,
			PlaylistID: m.YouTubePlaylistID,
			Since:      m.ActiveFrom.UTC().Format(time.RFC3339),
			State:      listenerState(m.Enabled),
		})
		return
	}
	writeErr(w, http.StatusNotFound, "no live content listener on that channel")
}

func (s *Server) respondPublicPicture(w http.ResponseWriter, ctx context.Context, matches []resolvedChannel) {
	for _, ch := range matches {
		p, err := s.store.GetPictureListenerByChannel(ctx, ch.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if p == nil {
			continue
		}
		writeJSON(w, http.StatusOK, publicListenerDTO{
			Name:      p.Name,
			Channel:   ch.Name,
			Slideshow: "/slideshow/" + p.Slug,
			Since:     p.ActiveFrom.UTC().Format(time.RFC3339),
			State:     listenerState(p.Enabled),
		})
		return
	}
	writeErr(w, http.StatusNotFound, "no live picture listener on that channel")
}

func listenerState(enabled bool) string {
	if enabled {
		return "listening"
	}
	return "paused"
}

// handlePublicGetEpisode serves Streamer.bot JSON for one live show episode.
// Meat Bag: GET /api/get/episode/{show} — {show} is show_slug (e.g. sesh-sofa).
func (s *Server) handlePublicGetEpisode(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.PathValue("show"))
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "show slug is required")
		return
	}
	ep, err := s.store.GetLiveEpisodeByShowSlug(r.Context(), raw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ep == nil {
		writeErr(w, http.StatusNotFound, "no live episode for show: "+raw)
		return
	}
	listeners, err := s.episodeListenersDTO(r.Context(), ep.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"show":              ep.Show,
		"show_slug":         ep.ShowSlug,
		"episode":           ep.Episode,
		"episode_short":     ep.Short(),
		"episode_long":      ep.Long(),
		"name":              ep.Name,
		"twitch_suffix":     ep.TwitchSuffix,
		"episode_name_full": ep.NameFull(),
		"since":             ep.ActiveFrom.UTC().Format(time.RFC3339),
		"state":             "live",
		"listeners":         publicEpisodeListeners(listeners),
	})
}

func normalizeChannelName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "#")
	return strings.TrimSpace(name)
}

// resolveDiscordChannelsByName finds text channels whose name matches
// (case-insensitive). Order follows ListGuilds then ListTextChannels.
// Empty result means "not found"; error means Discord is unavailable.
func (s *Server) resolveDiscordChannelsByName(want string) ([]resolvedChannel, error) {
	if s.discord == nil {
		return nil, fmt.Errorf("discord is not available")
	}
	wantLower := strings.ToLower(want)
	guilds, err := s.discord.ListGuilds()
	if err != nil {
		return nil, fmt.Errorf("discord guilds: %w", err)
	}

	var out []resolvedChannel
	for _, g := range guilds {
		chs, err := s.discord.ListTextChannels(g.ID)
		if err != nil {
			return nil, fmt.Errorf("discord channels for guild %s: %w", g.ID, err)
		}
		for _, ch := range chs {
			got := normalizeChannelName(ch.Name)
			if strings.ToLower(got) != wantLower {
				continue
			}
			out = append(out, resolvedChannel{ID: ch.ID, Name: got})
		}
	}
	return out, nil
}

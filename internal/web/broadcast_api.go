package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
)

func (s *Server) registerBroadcastAPI(mux *http.ServeMux) {
	mux.Handle("GET /api/broadcasts", s.basicAuth(http.HandlerFunc(s.handleListBroadcasts)))
	mux.Handle("POST /api/broadcasts", s.basicAuth(http.HandlerFunc(s.handleCreateBroadcast)))
	mux.Handle("GET /api/broadcasts/{id}", s.basicAuth(http.HandlerFunc(s.handleGetBroadcast)))
	mux.Handle("PATCH /api/broadcasts/{id}", s.basicAuth(http.HandlerFunc(s.handlePatchBroadcast)))
	mux.Handle("DELETE /api/broadcasts/{id}", s.basicAuth(http.HandlerFunc(s.handleDeleteBroadcast)))
	// Streamer.bot + Admin Fire button — slug in path is the predictable trigger name.
	mux.Handle("GET /api/broadcasts/{slug}/fire", s.adminOrAPIAuth(http.HandlerFunc(s.handleFireBroadcast)))
}

type broadcastMessageDTO struct {
	ID         string   `json:"id"`
	Body       string   `json:"body"`
	ChannelIDs []string `json:"channel_ids"`
}

type broadcastDTO struct {
	ID                int64                 `json:"id"`
	Slug              string                `json:"slug"`
	Name              string                `json:"name"`
	EpisodeTemplateID *int64                `json:"episode_template_id"`
	TemplateShow      string                `json:"template_show,omitempty"`
	TemplateSlug      string                `json:"template_slug,omitempty"`
	Messages          []broadcastMessageDTO `json:"messages"`
	FireURL           string                `json:"fire_url"`
	UpdatedAt         string                `json:"updated_at"`
}

func toBroadcastDTO(b db.Broadcast) broadcastDTO {
	msgs := make([]broadcastMessageDTO, 0, len(b.Messages))
	for _, m := range b.Messages {
		chans := m.ChannelIDs
		if chans == nil {
			chans = []string{}
		}
		msgs = append(msgs, broadcastMessageDTO{
			ID:         m.ID,
			Body:       m.Body,
			ChannelIDs: chans,
		})
	}
	return broadcastDTO{
		ID:                b.ID,
		Slug:              b.Slug,
		Name:              b.Name,
		EpisodeTemplateID: b.EpisodeTemplateID,
		TemplateShow:      b.TemplateShow,
		TemplateSlug:      b.TemplateSlug,
		Messages:          msgs,
		FireURL:           "/api/broadcasts/" + b.Slug + "/fire",
		UpdatedAt:         b.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

type broadcastBody struct {
	Name              string                `json:"name"`
	Slug              string                `json:"slug"`
	EpisodeTemplateID *int64                `json:"episode_template_id"`
	Messages          []broadcastMessageDTO `json:"messages"`
}

func (s *Server) handleListBroadcasts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListBroadcasts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]broadcastDTO, 0, len(list))
	for _, b := range list {
		out = append(out, toBroadcastDTO(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"broadcasts": out})
}

func (s *Server) handleGetBroadcast(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid broadcast id")
		return
	}
	b, err := s.store.GetBroadcastByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeErr(w, http.StatusNotFound, "broadcast not found")
		return
	}
	writeJSON(w, http.StatusOK, toBroadcastDTO(*b))
}

func (s *Server) handleCreateBroadcast(w http.ResponseWriter, r *http.Request) {
	var body broadcastBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	b, err := s.store.CreateBroadcast(r.Context(), body.Name, body.Slug, body.EpisodeTemplateID, dtoToBroadcastMessages(body.Messages))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toBroadcastDTO(*b))
}

func (s *Server) handlePatchBroadcast(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid broadcast id")
		return
	}
	var body broadcastBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	b, err := s.store.UpdateBroadcast(r.Context(), id, body.Name, body.Slug, body.EpisodeTemplateID, dtoToBroadcastMessages(body.Messages))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toBroadcastDTO(*b))
}

func (s *Server) handleDeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid broadcast id")
		return
	}
	if err := s.store.DeleteBroadcast(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func dtoToBroadcastMessages(in []broadcastMessageDTO) []db.BroadcastMessage {
	out := make([]db.BroadcastMessage, 0, len(in))
	for _, m := range in {
		out = append(out, db.BroadcastMessage{
			ID:         m.ID,
			Body:       m.Body,
			ChannelIDs: m.ChannelIDs,
		})
	}
	return out
}

func (s *Server) handleFireBroadcast(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "slug is required")
		return
	}
	b, err := s.store.GetBroadcastBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeErr(w, http.StatusNotFound, "broadcast not found")
		return
	}

	fields, err := s.broadcastResolveFields(r.Context(), b)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}

	sent := 0
	var errs []string
	for _, msg := range b.Messages {
		body := msg.Body
		if fields != nil {
			body = db.ResolveTemplate(body, fields)
		}
		for _, channelID := range msg.ChannelIDs {
			if err := s.announce(r.Context(), channelID, body); err != nil {
				slog.Warn("broadcast fire failed", "slug", b.Slug, "channel", channelID, "err", err)
				errs = append(errs, fmt.Sprintf("%s: %v", channelID, err))
				continue
			}
			sent++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      len(errs) == 0,
		"slug":    b.Slug,
		"sent":    sent,
		"errors":  errs,
		"message": fmt.Sprintf("sent %d message(s)", sent),
	})
}

// broadcastResolveFields returns episode placeholder map for template-bound broadcasts.
// Standalone → nil fields (bodies sent as stored). Missing live episode → error.
func (s *Server) broadcastResolveFields(ctx context.Context, b *db.Broadcast) (map[string]string, error) {
	if b.EpisodeTemplateID == nil || *b.EpisodeTemplateID < 1 {
		return nil, nil
	}
	tmpl, err := s.store.GetEpisodeTemplateByID(ctx, *b.EpisodeTemplateID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, fmt.Errorf("episode template %d missing — re-link this broadcast", *b.EpisodeTemplateID)
	}
	ep, err := s.store.GetLiveEpisodeByShowSlug(ctx, tmpl.ShowSlug)
	if err != nil {
		return nil, err
	}
	if ep == nil {
		return nil, fmt.Errorf("no live episode for show %q — start one before firing", tmpl.Show)
	}
	fields := db.EpisodeFieldMap(ep.Show, ep.Episode, ep.Name, ep.TwitchSuffix)
	fields["episode_name_full"] = ep.NameFull()
	return fields, nil
}

func (s *Server) announce(ctx context.Context, channelID, content string) error {
	if s.announceFn != nil {
		return s.announceFn(ctx, s.discordTok, channelID, content)
	}
	return discord.Announce(ctx, s.discordTok, channelID, content)
}

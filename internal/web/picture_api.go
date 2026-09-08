package web

import (
	"net/http"
	"strings"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
)

func (s *Server) registerPictureAPI(mux *http.ServeMux) {
	mux.Handle("GET /api/picture-listens", s.basicAuth(http.HandlerFunc(s.handleListPictureListeners)))
	mux.Handle("POST /api/picture-listens", s.basicAuth(http.HandlerFunc(s.handleUpsertPictureListener)))
	mux.Handle("PATCH /api/picture-listens/{channel}", s.basicAuth(http.HandlerFunc(s.handlePatchPictureListener)))
	mux.Handle("DELETE /api/picture-listens/{channel}", s.basicAuth(http.HandlerFunc(s.handleDeletePictureListener)))
	mux.Handle("POST /api/picture-resync", s.basicAuth(http.HandlerFunc(s.handlePictureResync)))
}

type pictureListenerDTO struct {
	ID                 int64  `json:"id"`
	DiscordChannelID   string `json:"discord_channel_id"`
	DiscordChannelName string `json:"discord_channel_name,omitempty"`
	GuildID            string `json:"guild_id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Enabled            bool   `json:"enabled"`
	CreditCorner       string `json:"credit_corner"`
	IntervalSeconds    int    `json:"interval_seconds"`
	Shuffle            bool   `json:"shuffle"`
	ShowCredit         bool   `json:"show_credit"`
	ShowReactions      bool   `json:"show_reactions"`
	ReactionMultiplier int     `json:"reaction_multiplier"`
	CreditScale        float64 `json:"credit_scale"`
	ReactionScale      float64 `json:"reaction_scale"`
	PictureCount       int     `json:"picture_count"`
	SlideshowURL       string  `json:"slideshow_url"`
	CreatedAt          string  `json:"created_at"`
	ActiveFrom         string  `json:"active_from"`
	ActiveUntil        string  `json:"active_until,omitempty"`
}

func toPictureListenerDTO(p db.PictureListener, count int) pictureListenerDTO {
	dto := pictureListenerDTO{
		ID:                 p.ID,
		DiscordChannelID:   p.DiscordChannelID,
		GuildID:            p.GuildID,
		Name:               p.Name,
		Slug:               p.Slug,
		Enabled:            p.Enabled,
		CreditCorner:       p.CreditCorner,
		IntervalSeconds:    p.IntervalSeconds,
		Shuffle:            p.Shuffle,
		ShowCredit:         p.ShowCredit,
		ShowReactions:      p.ShowReactions,
		ReactionMultiplier: p.ReactionMultiplier,
		CreditScale:        p.CreditScale,
		ReactionScale:      p.ReactionScale,
		PictureCount:       count,
		SlideshowURL:       "/slideshow/" + p.Slug,
		CreatedAt:          p.CreatedAt.UTC().Format(time.RFC3339),
		ActiveFrom:         p.ActiveFrom.UTC().Format(time.RFC3339),
	}
	if p.ActiveUntil != nil {
		dto.ActiveUntil = p.ActiveUntil.UTC().Format(time.RFC3339)
	}
	return dto
}

func (s *Server) handleListPictureListeners(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPictureListeners(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	names := s.channelNamesForPictureListeners(list)
	out := make([]pictureListenerDTO, 0, len(list))
	for _, p := range list {
		n, _ := s.store.CountCollectedPictures(r.Context(), p.ID)
		dto := toPictureListenerDTO(p, n)
		dto.DiscordChannelName = names[p.DiscordChannelID]
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"picture_listens": out})
}

func (s *Server) channelNamesForPictureListeners(list []db.PictureListener) map[string]string {
	// Reuse the same Discord catalog walk as content listeners.
	fake := make([]db.ChannelMapping, 0, len(list))
	for _, p := range list {
		fake = append(fake, db.ChannelMapping{
			DiscordChannelID: p.DiscordChannelID,
			GuildID:          p.GuildID,
		})
	}
	return s.channelNamesForMappings(fake)
}

type pictureUpsertBody struct {
	DiscordChannelID string `json:"discord_channel_id"`
	GuildID          string `json:"guild_id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Enabled          *bool  `json:"enabled"`
	CreditCorner     string `json:"credit_corner"`
	IntervalSeconds  *int   `json:"interval_seconds"`
	Shuffle          *bool  `json:"shuffle"`
	ShowCredit       *bool  `json:"show_credit"`
	ShowReactions    *bool  `json:"show_reactions"`
	ReactionMultiplier *int     `json:"reaction_multiplier"`
	CreditScale        *float64 `json:"credit_scale"`
	ReactionScale      *float64 `json:"reaction_scale"`
}

func (s *Server) pictureInputFromBody(body pictureUpsertBody) db.PictureListenerInput {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	interval := 8
	if body.IntervalSeconds != nil && *body.IntervalSeconds > 0 {
		interval = *body.IntervalSeconds
	}
	shuffle := false
	if body.Shuffle != nil {
		shuffle = *body.Shuffle
	}
	showCredit := true
	if body.ShowCredit != nil {
		showCredit = *body.ShowCredit
	}
	showReactions := true
	if body.ShowReactions != nil {
		showReactions = *body.ShowReactions
	}
	mult := 1
	if body.ReactionMultiplier != nil {
		mult = *body.ReactionMultiplier
	}
	creditScale := 1.5
	if body.CreditScale != nil {
		creditScale = *body.CreditScale
	}
	reactionScale := 1.5
	if body.ReactionScale != nil {
		reactionScale = *body.ReactionScale
	}
	return db.PictureListenerInput{
		DiscordChannelID:   body.DiscordChannelID,
		GuildID:            body.GuildID,
		Name:               body.Name,
		Slug:               body.Slug,
		Enabled:            enabled,
		CreditCorner:       body.CreditCorner,
		IntervalSeconds:    interval,
		Shuffle:            shuffle,
		ShowCredit:         showCredit,
		ShowReactions:      showReactions,
		ReactionMultiplier: mult,
		CreditScale:        creditScale,
		ReactionScale:      reactionScale,
	}
}

func (s *Server) handleUpsertPictureListener(w http.ResponseWriter, r *http.Request) {
	var body pictureUpsertBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.DiscordChannelID) == "" {
		writeErr(w, http.StatusBadRequest, "discord_channel_id is required")
		return
	}
	if strings.TrimSpace(body.Name) == "" && strings.TrimSpace(body.Slug) == "" {
		writeErr(w, http.StatusBadRequest, "name or slug is required")
		return
	}

	in := s.pictureInputFromBody(body)
	p, err := s.store.UpsertPictureListener(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _ := s.store.CountCollectedPictures(r.Context(), p.ID)
	_ = s.store.LogActivity(r.Context(), "picture_listen_started", map[string]any{
		"channel_id": p.DiscordChannelID,
		"slug":       p.Slug,
		"name":       p.Name,
	}, true)
	writeJSON(w, http.StatusOK, map[string]any{"picture_listen": toPictureListenerDTO(*p, n)})
}

func (s *Server) handlePatchPictureListener(w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.PathValue("channel"))
	if channelID == "" {
		writeErr(w, http.StatusBadRequest, "channel id required")
		return
	}

	existing, err := s.store.GetPictureListenerByChannel(r.Context(), channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeErr(w, http.StatusNotFound, "no picture listener for channel")
		return
	}

	var body pictureUpsertBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Enabled-only toggle (pause/resume) — common Admin action.
	if body.Enabled != nil &&
		body.Name == "" && body.Slug == "" && body.CreditCorner == "" &&
		body.IntervalSeconds == nil && body.Shuffle == nil &&
		body.ShowCredit == nil && body.ShowReactions == nil &&
		body.ReactionMultiplier == nil && body.CreditScale == nil && body.ReactionScale == nil {
		p, err := s.store.SetPictureListenerEnabled(r.Context(), channelID, *body.Enabled)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		n, _ := s.store.CountCollectedPictures(r.Context(), p.ID)
		writeJSON(w, http.StatusOK, map[string]any{"picture_listen": toPictureListenerDTO(*p, n)})
		return
	}

	in := db.PictureListenerInput{
		DiscordChannelID:   channelID,
		GuildID:            existing.GuildID,
		Name:               existing.Name,
		Slug:               existing.Slug,
		Enabled:            existing.Enabled,
		CreditCorner:       existing.CreditCorner,
		IntervalSeconds:    existing.IntervalSeconds,
		Shuffle:            existing.Shuffle,
		ShowCredit:         existing.ShowCredit,
		ShowReactions:      existing.ShowReactions,
		ReactionMultiplier: existing.ReactionMultiplier,
		CreditScale:        existing.CreditScale,
		ReactionScale:      existing.ReactionScale,
	}
	if body.GuildID != "" {
		in.GuildID = body.GuildID
	}
	if body.Name != "" {
		in.Name = body.Name
	}
	if body.Enabled != nil {
		in.Enabled = *body.Enabled
	}
	if body.CreditCorner != "" {
		in.CreditCorner = body.CreditCorner
	}
	if body.IntervalSeconds != nil {
		in.IntervalSeconds = *body.IntervalSeconds
	}
	if body.Shuffle != nil {
		in.Shuffle = *body.Shuffle
	}
	if body.ShowCredit != nil {
		in.ShowCredit = *body.ShowCredit
	}
	if body.ShowReactions != nil {
		in.ShowReactions = *body.ShowReactions
	}
	if body.ReactionMultiplier != nil {
		in.ReactionMultiplier = *body.ReactionMultiplier
	}
	if body.CreditScale != nil {
		in.CreditScale = *body.CreditScale
	}
	if body.ReactionScale != nil {
		in.ReactionScale = *body.ReactionScale
	}

	p, err := s.store.UpdatePictureListenerSettings(r.Context(), channelID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _ := s.store.CountCollectedPictures(r.Context(), p.ID)
	writeJSON(w, http.StatusOK, map[string]any{"picture_listen": toPictureListenerDTO(*p, n)})
}

func (s *Server) handleDeletePictureListener(w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.PathValue("channel"))
	if channelID == "" {
		writeErr(w, http.StatusBadRequest, "channel id required")
		return
	}
	existing, _ := s.store.GetPictureListenerByChannel(r.Context(), channelID)
	if err := s.store.DeletePictureListener(r.Context(), channelID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	details := map[string]any{"channel_id": channelID}
	if existing != nil {
		details["slug"] = existing.Slug
		details["name"] = existing.Name
	}
	_ = s.store.LogActivity(r.Context(), "picture_listen_ceased", details, true)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ceased"})
}

func (s *Server) handlePictureResync(w http.ResponseWriter, r *http.Request) {
	var body resyncBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.ChannelID) == "" {
		writeErr(w, http.StatusBadRequest, "channel_id is required")
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

	summary, err := discord.ResyncPictureChannel(r.Context(), s.discordTok, s.store, body.ChannelID, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"channel_id":       body.ChannelID,
		"messages_scanned": summary.MessagesScanned,
		"saved":            summary.Result.Saved,
		"skipped":          summary.Result.Skipped,
		"failed":           summary.Result.Failed,
	})
}

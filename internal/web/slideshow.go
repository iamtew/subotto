package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"subotto/internal/db"
)

// registerPublic mounts unauthenticated routes (OBS slideshow + media +
// Streamer.bot listener GET). Must run before the Basic-Auth "/" catch-all.
func (s *Server) registerPublic(mux *http.ServeMux) {
	mux.HandleFunc("GET /slideshow/latest", s.handleSlideshowLatest)
	mux.HandleFunc("GET /slideshow/{slug}", s.handleSlideshowPage)
	mux.HandleFunc("GET /api/slideshow/{slug}", s.handleSlideshowFeed)
	mux.HandleFunc("GET /media/pictures/{slug}/{file}", s.handlePictureMedia)
	mux.HandleFunc("GET /api/get/episode/{show}", s.handlePublicGetEpisode)
	mux.HandleFunc("GET /api/get/{kind}/{channel}", s.handlePublicGetListener)

	// OBS page assets (no auth).
	staticDir := filepath.Join(s.webroot, "slideshow")
	mux.Handle("GET /slideshow/static/", http.StripPrefix("/slideshow/static/",
		http.FileServer(http.Dir(staticDir))))
}

func (s *Server) handleSlideshowLatest(w http.ResponseWriter, r *http.Request) {
	pl, err := s.store.GetLatestPictureListener(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pl == nil {
		writeErr(w, http.StatusNotFound, "no live picture listener")
		return
	}
	http.Redirect(w, r, "/slideshow/"+pl.Slug, http.StatusFound)
}

func (s *Server) handleSlideshowPage(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" || slug == "latest" {
		s.handleSlideshowLatest(w, r)
		return
	}
	if slug == "static" {
		http.NotFound(w, r)
		return
	}
	pl, err := s.store.GetPictureListenerBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pl == nil {
		writeErr(w, http.StatusNotFound, "picture listener not found or not live")
		return
	}
	http.ServeFile(w, r, filepath.Join(s.webroot, "slideshow", "index.html"))
}

type slideshowFeedDTO struct {
	Slug               string              `json:"slug"`
	Name               string              `json:"name"`
	CreditCorner       string              `json:"credit_corner"`
	IntervalSeconds    int                 `json:"interval_seconds"`
	Shuffle            bool                `json:"shuffle"`
	ShowCredit         bool                `json:"show_credit"`
	ShowReactions      bool                `json:"show_reactions"`
	ReactionsAnimated  bool                `json:"reactions_animated"`
	ReactionMultiplier int                 `json:"reaction_multiplier"`
	CreditScale        float64             `json:"credit_scale"`
	ReactionScale      float64             `json:"reaction_scale"`
	Transition         string              `json:"transition"`
	Images             []slideshowImageDTO `json:"images"`
}

type slideshowImageDTO struct {
	ID        int64              `json:"id"`
	URL       string             `json:"url"`
	Author    string             `json:"author"`
	Reactions []db.ReactionCount `json:"reactions"`
	Collected string             `json:"collected_at"`
}

func (s *Server) handleSlideshowFeed(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	pl, err := s.store.GetPictureListenerBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pl == nil {
		writeErr(w, http.StatusNotFound, "picture listener not found or not live")
		return
	}

	pics, err := s.store.ListCollectedPicturesForListener(r.Context(), pl.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	images := make([]slideshowImageDTO, 0, len(pics))
	for _, p := range pics {
		reactions := p.Reactions
		if reactions == nil {
			reactions = []db.ReactionCount{}
		}
		images = append(images, slideshowImageDTO{
			ID:        p.ID,
			URL:       "/media/pictures/" + filepath.ToSlash(p.StoredPath),
			Author:    p.AuthorDisplayName,
			Reactions: reactions,
			Collected: p.CollectedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, slideshowFeedDTO{
		Slug:               pl.Slug,
		Name:               pl.Name,
		CreditCorner:       pl.CreditCorner,
		IntervalSeconds:    pl.IntervalSeconds,
		Shuffle:            pl.Shuffle,
		ShowCredit:         pl.ShowCredit,
		ShowReactions:      pl.ShowReactions,
		ReactionsAnimated:  pl.ReactionsAnimated,
		ReactionMultiplier: pl.ReactionMultiplier,
		CreditScale:        pl.CreditScale,
		ReactionScale:      pl.ReactionScale,
		Transition:         pl.Transition,
		Images:             images,
	})
}

func (s *Server) handlePictureMedia(w http.ResponseWriter, r *http.Request) {
	slug := db.Slugify(r.PathValue("slug"))
	file := filepath.Base(r.PathValue("file"))
	if slug == "" || file == "" || file == "." || file == ".." {
		http.NotFound(w, r)
		return
	}

	// Only serve files for a known live (or any historical) slug folder under pictures/.
	// Path must stay inside PicturesDir.
	rel := filepath.ToSlash(filepath.Join(slug, file))
	abs, err := s.store.AbsolutePicturePath(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Cache briefly — OBS reloads often; reactions are on the JSON feed, not the file.
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeFile(w, r, abs)
}

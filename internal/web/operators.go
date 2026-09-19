package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"subotto/internal/db"
)

func (s *Server) handleGetOperators(w http.ResponseWriter, r *http.Request) {
	ident, _ := adminIdentFrom(r.Context())
	extra, err := s.store.ExtraAdminDiscordIDs(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"superadmin_id": s.superadminID,
		"extra_ids":     extra,
		"you": map[string]any{
			"discord_id": ident.DiscordID,
			"superadmin": ident.Superadmin,
		},
	})
}

func (s *Server) handlePutOperators(w http.ResponseWriter, r *http.Request) {
	ident, ok := adminIdentFrom(r.Context())
	if !ok || !ident.Superadmin {
		writeErr(w, http.StatusForbidden, "only the superadmin can edit operators")
		return
	}
	var body struct {
		ExtraIDs []string `json:"extra_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cleaned := make([]string, 0, len(body.ExtraIDs))
	for _, id := range body.ExtraIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !db.IsDiscordSnowflake(id) {
			writeErr(w, http.StatusBadRequest, "invalid Discord user id: "+id)
			return
		}
		if s.superadminID != "" && id == s.superadminID {
			continue
		}
		cleaned = append(cleaned, id)
	}
	cleaned = db.NormalizeDiscordIDs(cleaned)
	if err := s.store.SaveExtraAdminDiscordIDs(r.Context(), cleaned); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleGetOperators(w, r)
}

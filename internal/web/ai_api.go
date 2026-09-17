package web

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"subotto/internal/db"
)

const maxAISystemPromptRunes = 8000

func (s *Server) registerAIAPI(mux *http.ServeMux) {
	mux.Handle("GET /api/ai", s.basicAuth(http.HandlerFunc(s.handleGetAI)))
	mux.Handle("PUT /api/ai", s.basicAuth(http.HandlerFunc(s.handlePutAI)))
	mux.Handle("POST /api/ai/chat", s.basicAuth(http.HandlerFunc(s.handleAIChat)))
}

func (s *Server) handleGetAI(w http.ResponseWriter, r *http.Request) {
	s.writeAISettings(w, r)
}

type aiSettingsBody struct {
	SystemPrompt *string `json:"system_prompt"`
	Enabled      *bool   `json:"enabled"`
}

func (s *Server) handlePutAI(w http.ResponseWriter, r *http.Request) {
	var body aiSettingsBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.SystemPrompt == nil && body.Enabled == nil {
		writeErr(w, http.StatusBadRequest, "nothing to save")
		return
	}
	if body.SystemPrompt != nil {
		prompt := strings.TrimSpace(*body.SystemPrompt)
		if utf8.RuneCountInString(prompt) > maxAISystemPromptRunes {
			writeErr(w, http.StatusBadRequest, "system prompt is too long")
			return
		}
		if err := s.store.SetSetting(r.Context(), db.SettingAISystemPrompt, prompt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = s.store.LogActivity(r.Context(), "ai_prompt_updated", map[string]any{"source": "admin_ui"}, true)
	}
	if body.Enabled != nil {
		if err := s.store.SetAIEnabled(r.Context(), *body.Enabled); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = s.store.LogActivity(r.Context(), "ai_enabled_updated", map[string]any{"enabled": *body.Enabled, "source": "admin_ui"}, true)
	}
	s.writeAISettings(w, r)
}

func (s *Server) writeAISettings(w http.ResponseWriter, r *http.Request) {
	prompt, err := s.store.AISystemPrompt(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	enabled, err := s.store.AIEnabled(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	model := ""
	configured := false
	if s.ai != nil {
		model = s.ai.Model()
		configured = s.ai.Configured()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"model":         model,
		"configured":    configured,
		"enabled":       enabled,
		"system_prompt": prompt,
	})
}

type aiChatBody struct {
	Message string `json:"message"`
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if s.ai == nil || !s.ai.Configured() {
		writeErr(w, http.StatusServiceUnavailable, "OPENROUTER_API_KEY is not set")
		return
	}
	var body aiChatBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		writeErr(w, http.StatusBadRequest, "message is empty")
		return
	}
	prompt, err := s.store.AISystemPrompt(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reply, err := s.ai.Chat(r.Context(), prompt, msg)
	if err != nil {
		slog.Error("admin hesh helper chat failed", "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reply": reply})
}

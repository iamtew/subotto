package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"subotto/internal/ai"
	"subotto/internal/db"
)

const maxAISystemPromptRunes = 8000

func (s *Server) registerAIAPI(mux *http.ServeMux) {
	mux.Handle("GET /api/ai", s.basicAuth(http.HandlerFunc(s.handleGetAI)))
	mux.Handle("PUT /api/ai", s.basicAuth(http.HandlerFunc(s.handlePutAI)))
	mux.Handle("POST /api/ai/chat", s.basicAuth(http.HandlerFunc(s.handleAIChat)))
	mux.Handle("GET /api/ai/logs", s.basicAuth(http.HandlerFunc(s.handleAILogs)))
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
		err := &ai.APIError{Msg: "OPENROUTER_API_KEY is not set"}
		s.logAdminAI(r, "", "", 0, err)
		writeErr(w, http.StatusServiceUnavailable, err.Error())
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
		s.logAdminAI(r, msg, "", 0, err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reply, err := s.ai.Chat(r.Context(), prompt, msg)
	if err != nil {
		slog.Error("admin hesh helper chat failed", "err", err)
		s.logAdminAI(r, msg, "", ai.HTTPStatus(err), err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	s.logAdminAI(r, msg, reply, 0, nil)
	writeJSON(w, http.StatusOK, map[string]any{"reply": reply})
}

func (s *Server) logAdminAI(r *http.Request, userText, reply string, httpStatus int, apiErr error) {
	if s == nil || s.store == nil {
		return
	}
	errText := ""
	if apiErr != nil {
		errText = apiErr.Error()
	}
	_ = s.store.LogAIRequest(r.Context(), db.AIRequestEntry{
		Level:       db.ClassifyAILevel(httpStatus, apiErr, nil),
		Trigger:     "admin_chat",
		UserMessage: userText,
		Reply:       reply,
		Error:       errText,
		HTTPStatus:  httpStatus,
	})
}

func (s *Server) handleAILogs(w http.ResponseWriter, r *http.Request) {
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	if level == "" {
		level = "all"
	}
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			limit = n
		}
	}
	list, err := s.store.ListAIRequests(r.Context(), level, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]aiLogDTO, 0, len(list))
	for _, e := range list {
		out = append(out, toAILogDTO(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": out})
}

type aiLogDTO struct {
	ID          int64  `json:"id"`
	Timestamp   string `json:"timestamp"`
	Level       string `json:"level"`
	Trigger     string `json:"trigger"`
	Author      string `json:"author"`
	ChannelID   string `json:"channel_id"`
	UserMessage string `json:"user_message"`
	Reply       string `json:"reply"`
	Error       string `json:"error"`
	HTTPStatus  int    `json:"http_status"`
}

func toAILogDTO(e db.AIRequestEntry) aiLogDTO {
	return aiLogDTO{
		ID:          e.ID,
		Timestamp:   e.Timestamp.UTC().Format(time.RFC3339),
		Level:       e.Level,
		Trigger:     e.Trigger,
		Author:      e.Author,
		ChannelID:   e.ChannelID,
		UserMessage: e.UserMessage,
		Reply:       e.Reply,
		Error:       e.Error,
		HTTPStatus:  e.HTTPStatus,
	}
}

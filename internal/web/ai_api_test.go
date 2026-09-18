package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"subotto/internal/ai"
	"subotto/internal/db"
)

func TestAIGetPutAndChatUnconfigured(t *testing.T) {
	s, _ := testServer(t)
	s.ai = ai.New("", ai.DefaultModel)

	req := httptest.NewRequest(http.MethodGet, "/api/ai", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["configured"] != false {
		t.Fatalf("configured: %#v", got["configured"])
	}
	if got["system_prompt"] != db.DefaultAISystemPrompt {
		t.Fatalf("prompt: %#v", got["system_prompt"])
	}
	if got["enabled"] != true {
		t.Fatalf("enabled default: %#v", got["enabled"])
	}

	off := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"enabled":false}`))
	off.SetBasicAuth("admin", "test-pass")
	off.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, off)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != false {
		t.Fatalf("disabled: %#v", got["enabled"])
	}

	put := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"system_prompt":"be brief"}`))
	put.SetBasicAuth("admin", "test-pass")
	put.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != false || got["system_prompt"] != "be brief" {
		t.Fatalf("prompt save should keep enabled=false: %#v", got)
	}

	chat := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"message":"hi"}`))
	chat.SetBasicAuth("admin", "test-pass")
	chat.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, chat)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("chat unconfigured: %d %s", rec.Code, rec.Body.String())
	}

	logs := httptest.NewRequest(http.MethodGet, "/api/ai/logs?level=all", nil)
	logs.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, logs)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: %d %s", rec.Code, rec.Body.String())
	}
	var logBody struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &logBody); err != nil {
		t.Fatal(err)
	}
	if len(logBody.Logs) != 1 {
		t.Fatalf("logs=%#v", logBody.Logs)
	}
	if logBody.Logs[0]["level"] != "critical" || logBody.Logs[0]["trigger"] != "admin_chat" {
		t.Fatalf("row=%#v", logBody.Logs[0])
	}

	warnOnly := httptest.NewRequest(http.MethodGet, "/api/ai/logs?level=warning", nil)
	warnOnly.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, warnOnly)
	if rec.Code != http.StatusOK {
		t.Fatalf("warn logs: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &logBody); err != nil {
		t.Fatal(err)
	}
	if len(logBody.Logs) != 1 {
		t.Fatalf("warning filter should include critical, got %#v", logBody.Logs)
	}
}

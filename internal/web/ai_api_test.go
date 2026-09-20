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
	s.ai = ai.New("")

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
	if got["twitch_enabled"] != true {
		t.Fatalf("twitch_enabled default: %#v", got["twitch_enabled"])
	}
	sampling, _ := json.Marshal(got["sampling"])
	var samp ai.Sampling
	if err := json.Unmarshal(sampling, &samp); err != nil {
		t.Fatal(err)
	}
	if samp != ai.DefaultSampling() {
		t.Fatalf("sampling default: %#v", got["sampling"])
	}
	if got["model"] != ai.DefaultModel {
		t.Fatalf("model default: %#v", got["model"])
	}
	models, _ := got["models"].([]any)
	if len(models) != 2 || models[1] != ai.DefaultFlashModel {
		t.Fatalf("models default: %#v", got["models"])
	}
	if got["memory_enabled"] != false {
		t.Fatalf("memory default: %#v", got["memory_enabled"])
	}
	if got["memory_window"] != float64(db.DefaultAIMemoryWindow) {
		t.Fatalf("memory window default: %#v", got["memory_window"])
	}

	mem := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"memory_enabled":true,"memory_window":4}`))
	mem.SetBasicAuth("admin", "test-pass")
	mem.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, mem)
	if rec.Code != http.StatusOK {
		t.Fatalf("memory: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["memory_enabled"] != true || got["memory_window"] != float64(4) {
		t.Fatalf("memory save: %#v", got)
	}
	badWin := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"memory_window":99}`))
	badWin.SetBasicAuth("admin", "test-pass")
	badWin.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, badWin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad window: %d %s", rec.Code, rec.Body.String())
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
	if got["twitch_enabled"] != false {
		t.Fatalf("twitch should follow Discord until set: %#v", got["twitch_enabled"])
	}

	twOff := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"twitch_enabled":true}`))
	twOff.SetBasicAuth("admin", "test-pass")
	twOff.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, twOff)
	if rec.Code != http.StatusOK {
		t.Fatalf("twitch enable: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["enabled"] != false || got["twitch_enabled"] != true {
		t.Fatalf("split flags: enabled=%v twitch=%v", got["enabled"], got["twitch_enabled"])
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
	if got["twitch_enabled"] != true {
		t.Fatalf("prompt save should keep twitch_enabled=true: %#v", got["twitch_enabled"])
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

func TestAIPutSampling(t *testing.T) {
	s, _ := testServer(t)
	s.ai = ai.New("")

	put := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"sampling":{"temperature":0.25,"top_k":8}}`))
	put.SetBasicAuth("admin", "test-pass")
	put.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got["sampling"])
	var samp ai.Sampling
	if err := json.Unmarshal(raw, &samp); err != nil {
		t.Fatal(err)
	}
	if samp.Temperature != 0.25 || samp.TopK != 8 || samp.TopP != 1 {
		t.Fatalf("saved sampling: %+v", samp)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/ai", nil)
	get.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, get)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(got["sampling"])
	if err := json.Unmarshal(raw, &samp); err != nil {
		t.Fatal(err)
	}
	if samp.Temperature != 0.25 || samp.TopK != 8 {
		t.Fatalf("get sampling: %+v", samp)
	}
}

func TestAIPutModels(t *testing.T) {
	s, _ := testServer(t)
	s.ai = ai.New("")

	put := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"model":"deepseek/deepseek-v4-flash-0731:free"}`))
	put.SetBasicAuth("admin", "test-pass")
	put.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, put)
	if rec.Code != http.StatusOK {
		t.Fatalf("put model: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != ai.DefaultFlashModel {
		t.Fatalf("selected: %#v", got["model"])
	}

	add := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"model":"acme/foo","models":["nvidia/nemotron-3-ultra-550b-a55b:free","deepseek/deepseek-v4-flash-0731:free","acme/foo"]}`))
	add.SetBasicAuth("admin", "test-pass")
	add.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, add)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "acme/foo" {
		t.Fatalf("added: %#v", got)
	}
	models, _ := got["models"].([]any)
	if len(models) != 3 {
		t.Fatalf("catalog: %#v", models)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/ai", nil)
	get.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, get)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "acme/foo" {
		t.Fatalf("get model: %#v", got["model"])
	}

	empty := httptest.NewRequest(http.MethodPut, "/api/ai", strings.NewReader(`{"models":[]}`))
	empty.SetBasicAuth("admin", "test-pass")
	empty.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, empty)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty catalog: %d %s", rec.Code, rec.Body.String())
	}
}

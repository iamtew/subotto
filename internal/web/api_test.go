package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subotto/internal/db"
)

type fakeStatus struct{ on bool }

func (f fakeStatus) Connected() bool { return f.on }

func testServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Copy a tiny webroot so New() finds index.html.
	webroot := filepath.Join(dir, "webroot")
	if err := os.MkdirAll(webroot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webroot, "index.html"), []byte("<h1>Subotto</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New(Options{
		Store:         store,
		Status:        fakeStatus{on: true},
		AdminPassword: "test-pass",
		AdminHost:     "127.0.0.1",
		AdminPort:     18080,
		Webroot:       webroot,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s, store
}

func TestStatusRequiresAuth(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestStatusAndMappingsCRUD(t *testing.T) {
	s, _ := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["discord_connected"] != true {
		t.Fatalf("expected discord_connected true: %v", status)
	}
	if status["phase"] != float64(6) {
		t.Fatalf("expected phase 6, got %v", status["phase"])
	}
	if status["scheduler_enabled"] != false {
		t.Fatalf("expected scheduler_enabled false without Scheduler wired: %v", status)
	}

	body := `{"discord_channel_id":"111","youtube_playlist_id":"PL1","name":"alpha","enabled":true}`
	req = httptest.NewRequest(http.MethodPost, "/api/mappings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/mappings", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Mappings []mappingDTO `json:"mappings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Mappings) != 1 || listed.Mappings[0].DiscordChannelID != "111" {
		t.Fatalf("unexpected list: %+v", listed)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/mappings/111", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/mappings/111", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

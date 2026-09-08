package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"subotto/internal/db"
)

var errReconnectBoom = errors.New("reconnect boom")

type fakeStatus struct {
	on  bool
	err error // if set, Reconnect returns this error
}

func (f fakeStatus) Connected() bool { return f.on }

func (f fakeStatus) Reconnect() error { return f.err }

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

func TestDiscordGuildsRequiresCatalog(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/discord/guilds", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 without Discord catalog, got %d body=%s", rec.Code, rec.Body.String())
	}
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
	if status["phase"] != float64(8) {
		t.Fatalf("expected phase 8, got %v", status["phase"])
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

func TestPictureListensAndPublicSlideshow(t *testing.T) {
	s, store := testServer(t)

	// Slideshow page asset for ServeFile.
	webroot := s.webroot
	if err := os.MkdirAll(filepath.Join(webroot, "slideshow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webroot, "slideshow", "index.html"), []byte("<html>slide</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"discord_channel_id":"222","guild_id":"g1","name":"OBS Night","enabled":true,"credit_corner":"tl","interval_seconds":5,"show_credit":true,"show_reactions":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/picture-listens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("picture upsert: %d %s", rec.Code, rec.Body.String())
	}

	// Public feed — no auth.
	req = httptest.NewRequest(http.MethodGet, "/api/slideshow/obs_night", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public feed: %d %s", rec.Code, rec.Body.String())
	}
	var feed slideshowFeedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	if feed.Slug != "obs_night" || feed.CreditCorner != "tl" || feed.IntervalSeconds != 5 {
		t.Fatalf("bad feed: %+v", feed)
	}

	// Public page — no auth.
	req = httptest.NewRequest(http.MethodGet, "/slideshow/obs_night", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("slideshow page: %d", rec.Code)
	}

	// latest redirect
	req = httptest.NewRequest(http.MethodGet, "/slideshow/latest", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("latest: want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/slideshow/obs_night" {
		t.Fatalf("latest location: %s", loc)
	}

	// Admin still requires auth for picture list.
	req = httptest.NewRequest(http.MethodGet, "/api/picture-listens", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("picture list should require auth, got %d", rec.Code)
	}

	// Seed a picture file + row, then hit media URL publicly.
	pl, err := store.GetPictureListenerBySlug(context.Background(), "obs_night")
	if err != nil || pl == nil {
		t.Fatalf("get pl: %v", err)
	}
	rel := pl.Slug + "/m1_a1.png"
	abs, err := store.AbsolutePicturePath(rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = store.InsertCollectedPicture(context.Background(), db.CollectedPicture{
		ListenerID:          pl.ID,
		DiscordMessageID:    "m1",
		DiscordAttachmentID: "a1",
		AuthorDisplayName:   "Bob",
		StoredPath:          rel,
		ContentType:         "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/media/pictures/"+rel, nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("media: %d", rec.Code)
	}
}

func TestDiscordReconnect(t *testing.T) {
	s, _ := testServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/discord/reconnect", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reconnect ok: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok true: %v", body)
	}

	// Failure path: status provider returns an error.
	s.status = fakeStatus{on: false, err: errReconnectBoom}
	req = httptest.NewRequest(http.MethodPost, "/api/discord/reconnect", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("reconnect fail: want 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

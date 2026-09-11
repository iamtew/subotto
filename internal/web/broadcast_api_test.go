package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestBroadcastCRUDAndFireAuth(t *testing.T) {
	s, store := testServer(t)
	s.apiPassword = "api-secret"
	s.discordTok = "fake-token"

	var mu sync.Mutex
	var posts []string
	s.announceFn = func(ctx context.Context, token, channelID, content string) error {
		mu.Lock()
		defer mu.Unlock()
		posts = append(posts, channelID+"|"+content)
		return nil
	}

	ctx := context.Background()
	tmpl, err := store.CreateEpisodeTemplate(ctx, "Sesh Sofa", "LIVE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEpisode(ctx, tmpl.Show, 20, "Creature Park", tmpl.TwitchSuffix, tmpl.NameFullTemplate, nil); err != nil {
		t.Fatal(err)
	}

	body := `{
		"name":"Cold Open",
		"slug":"cold-open",
		"episode_template_id":` + strconv.FormatInt(tmpl.ID, 10) + `,
		"messages":[{"body":"Hello {{show}} {{episode_short}}","channel_ids":["111","222"]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/broadcasts", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/broadcasts/cold-open/fire", nil)
	req.SetBasicAuth("api", "api-secret")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fire api: %d %s", rec.Code, rec.Body.String())
	}
	var fire map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fire); err != nil {
		t.Fatal(err)
	}
	if fire["ok"] != true || fire["sent"].(float64) != 2 {
		t.Fatalf("fire result: %+v", fire)
	}
	mu.Lock()
	gotPosts := append([]string{}, posts...)
	mu.Unlock()
	if len(gotPosts) != 2 {
		t.Fatalf("posts: %v", gotPosts)
	}
	if !strings.Contains(gotPosts[0], "Hello Sesh Sofa EP20") {
		t.Fatalf("resolved body: %v", gotPosts)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/broadcasts/cold-open/fire", nil)
	req.SetBasicAuth("api", "nope")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 bad api pass, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/broadcasts/cold-open/fire", nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fire admin: %d %s", rec.Code, rec.Body.String())
	}

	list, err := store.ListLiveEpisodes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("live: %v %v", list, err)
	}
	if err := store.CeaseEpisode(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/broadcasts/cold-open/fire", nil)
	req.SetBasicAuth("api", "api-secret")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409 no live episode, got %d %s", rec.Code, rec.Body.String())
	}
}

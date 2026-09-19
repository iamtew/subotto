package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"subotto/internal/db"
)

func TestEpisodeTemplatesAndPublicGet(t *testing.T) {
	s, store := testServer(t)
	s.createPlaylistFn = func(ctx context.Context, title, description string) (string, error) {
		return "PL-" + strings.ReplaceAll(title, " ", "_"), nil
	}

	body := `{
		"show":"Sesh Sofa",
		"twitch_suffix":"LIVE",
		"name_full_template":"",
		"listeners":[
			{"kind":"content","guild_id":"g1","discord_channel_id":"111","name":"{{episode_short}} {{name}}","playlist_title":"{{episode_short}} {{name}}"},
			{"kind":"picture","guild_id":"g1","discord_channel_id":"222","name":"{{episode_short}} pics","slug":"{{episode_short}}_pics"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/episode-templates", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create template: %d %s", rec.Code, rec.Body.String())
	}
	var tmplDTO map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tmplDTO); err != nil {
		t.Fatal(err)
	}
	tmplID := int64(tmplDTO["id"].(float64))
	if tmplDTO["show_slug"] != "sesh-sofa" {
		t.Fatalf("slug: %v", tmplDTO["show_slug"])
	}

	start := `{"template_id":` + strconv.FormatInt(tmplID, 10) + `,"episode":20,"name":"Creature Park"}`
	req = httptest.NewRequest(http.MethodPost, "/api/episodes", strings.NewReader(start))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start episode: %d %s", rec.Code, rec.Body.String())
	}
	var ep episodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.EpisodeShort != "EP20" || ep.ShowSlug != "sesh-sofa" {
		t.Fatalf("episode dto: %+v", ep)
	}
	if len(ep.Listeners) != 2 {
		t.Fatalf("listeners: %+v", ep.Listeners)
	}
	if ep.EpisodeNameFull != "EP20 Creature Park | Sesh Sofa | LIVE" {
		t.Fatalf("full name: %q", ep.EpisodeNameFull)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/get/episode/sesh-sofa", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public get: %d %s", rec.Code, rec.Body.String())
	}
	var pub map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pub); err != nil {
		t.Fatal(err)
	}
	if pub["episode_short"] != "EP20" || pub["state"] != "live" {
		t.Fatalf("public: %+v", pub)
	}
	if _, ok := pub["air_datetime"]; !ok {
		t.Fatalf("public missing air_datetime: %+v", pub)
	}
	if _, ok := pub["spot_image"]; !ok {
		t.Fatalf("public missing spot_image: %+v", pub)
	}
	if _, ok := pub["stream_background"]; !ok {
		t.Fatalf("public missing stream_background: %+v", pub)
	}
	if _, ok := pub["stream_background_url"]; !ok {
		t.Fatalf("public missing stream_background_url: %+v", pub)
	}
	listeners, ok := pub["listeners"].([]any)
	if !ok || len(listeners) != 2 {
		t.Fatalf("public listeners: %+v", pub["listeners"])
	}
	for _, raw := range listeners {
		l, _ := raw.(map[string]any)
		if _, has := l["discord_channel_id"]; has {
			t.Fatalf("public listener leaked discord_channel_id: %+v", l)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/get/episode/SESH-SOFA", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public get case: %d", rec.Code)
	}

	idPath := "/api/episodes/" + strconv.FormatInt(ep.ID, 10)

	// PATCH rejects unknown/immutable fields (DisallowUnknownFields).
	req = httptest.NewRequest(http.MethodPatch, idPath, strings.NewReader(`{"show":"Nope"}`))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for immutable show, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPatch, idPath, strings.NewReader(`{"name":"Creature Park Remix"}`))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch name: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, idPath, nil)
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cease: %d %s", rec.Code, rec.Body.String())
	}

	maps, err := store.ListMappings(context.Background())
	if err != nil || len(maps) != 0 {
		t.Fatalf("content listeners after cease: %v %v", maps, err)
	}
	pics, err := store.ListPictureListeners(context.Background())
	if err != nil || len(pics) != 0 {
		t.Fatalf("picture listeners after cease: %v %v", pics, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/get/episode/sesh-sofa", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 after cease, got %d", rec.Code)
	}
}

func TestAbsorbLiveListeners(t *testing.T) {
	s, store := testServer(t)
	ctx := context.Background()

	// Destination live episode (start shell with empty stubs via CreateEpisode).
	epRow, err := store.CreateEpisode(ctx, "Sesh Sofa", 19, "Absorb Me", "LIVE", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Orphan live listeners (as after PROD restart with no episode yet).
	if _, err := store.UpsertMapping(ctx, "c-abs", "g1", "PLold", "Old PL", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: "c-pic", GuildID: "g1", Name: "Old Pics", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"episode_id":` + strconv.FormatInt(epRow.ID, 10) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/episodes/absorb", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("absorb: %d %s", rec.Code, rec.Body.String())
	}
	var ep episodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.ID != epRow.ID || len(ep.Listeners) != 2 {
		t.Fatalf("want 2 linked listeners on episode %d, got %+v", epRow.ID, ep)
	}

	// Same playlist / slug still live (no recreate).
	m, err := store.GetMappingByChannel(ctx, "c-abs")
	if err != nil || m == nil || m.YouTubePlaylistID != "PLold" {
		t.Fatalf("content epoch changed: %+v err=%v", m, err)
	}
	p, err := store.GetPictureListenerByChannel(ctx, "c-pic")
	if err != nil || p == nil || p.Slug != "old_pics" {
		t.Fatalf("picture epoch changed: %+v err=%v", p, err)
	}

	unlinkedM, _ := store.ListUnlinkedMappings(ctx)
	unlinkedP, _ := store.ListUnlinkedPictureListeners(ctx)
	if len(unlinkedM) != 0 || len(unlinkedP) != 0 {
		t.Fatalf("still unlinked: maps=%v pics=%v", unlinkedM, unlinkedP)
	}

	// Missing episode_id → 400.
	req = httptest.NewRequest(http.MethodPost, "/api/episodes/absorb", strings.NewReader(`{}`))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 without episode_id, got %d", rec.Code)
	}

	// Second absorb with nothing left → 400.
	req = httptest.NewRequest(http.MethodPost, "/api/episodes/absorb", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when nothing to absorb, got %d", rec.Code)
	}
}

func TestStartEpisodeRollbackOnStubFailure(t *testing.T) {
	s, store := testServer(t)
	// First stub OK (picture), second content stub fails (no YouTube).
	body := `{
		"show":"Rollback Show",
		"listeners":[
			{"kind":"picture","guild_id":"g1","discord_channel_id":"pic1","name":"Pics","slug":"pics_rb"},
			{"kind":"content","guild_id":"g1","discord_channel_id":"vid1","name":"Vids","playlist_title":"Vids"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/episode-templates", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("template: %d %s", rec.Code, rec.Body.String())
	}
	var tmplDTO map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tmplDTO)
	tmplID := int64(tmplDTO["id"].(float64))

	start := `{"template_id":` + strconv.FormatInt(tmplID, 10) + `,"episode":1,"name":"Boom"}`
	req = httptest.NewRequest(http.MethodPost, "/api/episodes", strings.NewReader(start))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want start failure, got %d %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	if pics, _ := store.ListPictureListeners(ctx); len(pics) != 0 {
		t.Fatalf("picture stub should be rolled back, got %+v", pics)
	}
	if maps, _ := store.ListMappings(ctx); len(maps) != 0 {
		t.Fatalf("content should be empty, got %+v", maps)
	}
	if live, _ := store.ListLiveEpisodes(ctx); len(live) != 0 {
		t.Fatalf("episode should be ceased, got %+v", live)
	}
}

func TestEpisodeRenameSyncsListenerNames(t *testing.T) {
	s, store := testServer(t)
	s.createPlaylistFn = func(ctx context.Context, title, description string) (string, error) {
		return "PL-" + strings.ReplaceAll(title, " ", "_"), nil
	}

	body := `{
		"show":"Rename Show",
		"twitch_suffix":"LIVE",
		"listeners":[
			{"kind":"content","guild_id":"g1","discord_channel_id":"r1","name":"{{episode_short}} {{name}}","playlist_title":"{{episode_short}} {{name}}"},
			{"kind":"picture","guild_id":"g1","discord_channel_id":"r2","name":"{{episode_short}} {{name}} pics","slug":"rename_pics"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/episode-templates", strings.NewReader(body))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("template: %d %s", rec.Code, rec.Body.String())
	}
	var tmplDTO map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tmplDTO)
	tmplID := int64(tmplDTO["id"].(float64))

	start := `{"template_id":` + strconv.FormatInt(tmplID, 10) + `,"episode":7,"name":"TBD"}`
	req = httptest.NewRequest(http.MethodPost, "/api/episodes", strings.NewReader(start))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	var ep episodeDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &ep)

	patch := `{"name":"Creature Park"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/episodes/"+strconv.FormatInt(ep.ID, 10), strings.NewReader(patch))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	m, err := store.GetMappingByChannel(ctx, "r1")
	if err != nil || m == nil || m.Name != "EP7 Creature Park" {
		t.Fatalf("content name after rename: %+v err=%v", m, err)
	}
	p, err := store.GetPictureListenerByChannel(ctx, "r2")
	if err != nil || p == nil || p.Name != "EP7 Creature Park pics" {
		t.Fatalf("picture name after rename: %+v err=%v", p, err)
	}
	if p.Slug != "rename_pics" {
		t.Fatalf("slug must stay put, got %q", p.Slug)
	}
}

// 1x1 PNG so DetectContentType + image allowlist both pass.
var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde,
	0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54,
	0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01,
	0x5c, 0xc2, 0xd5, 0x57,
	0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestEpisodeAirDateAndSpot(t *testing.T) {
	s, _ := testServer(t)
	s.createPlaylistFn = func(ctx context.Context, title, description string) (string, error) {
		return "PL-x", nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/episode-templates", strings.NewReader(`{"show":"Sesh Sofa","listeners":[]}`))
	req.SetBasicAuth("admin", "test-pass")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("template: %d %s", rec.Code, rec.Body.String())
	}
	var tmplDTO map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tmplDTO)
	tmplID := int64(tmplDTO["id"].(float64))

	start := `{"template_id":` + strconv.FormatInt(tmplID, 10) + `,"episode":20,"name":"Creature Park","air_datetime":"2026-09-20T20:00:00+02:00"}`
	req = httptest.NewRequest(http.MethodPost, "/api/episodes", strings.NewReader(start))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	var ep episodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.AirDateTime != "2026-09-20T20:00:00+02:00" {
		t.Fatalf("start air_datetime: %q", ep.AirDateTime)
	}
	wantSpot := fmt.Sprintf("/media/episodes/%d/%s", ep.ID, db.SpotPlaceholderFile)
	if ep.SpotImage != wantSpot {
		t.Fatalf("spot should be placeholder, got %q", ep.SpotImage)
	}
	req = httptest.NewRequest(http.MethodGet, ep.SpotImage, nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("placeholder media: %d", rec.Code)
	}
	if ep.StreamBackground != "" || ep.StreamBackgroundURL != "" {
		t.Fatalf("stream background should be empty, got %q %q", ep.StreamBackground, ep.StreamBackgroundURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/media/stream-background/latest", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("latest before upload: %d", rec.Code)
	}

	idPath := "/api/episodes/" + strconv.FormatInt(ep.ID, 10)
	req = httptest.NewRequest(http.MethodPatch, idPath, strings.NewReader(`{"air_datetime":"2026-10-01T20:00:00+02:00"}`))
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch air: %d %s", rec.Code, rec.Body.String())
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "spot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(png1x1); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, idPath+"/spot", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("spot: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.SpotImage == "" || !strings.Contains(ep.SpotImage, "/media/episodes/") {
		t.Fatalf("spot url: %q", ep.SpotImage)
	}
	if strings.Contains(ep.SpotImage, db.SpotPlaceholderFile) {
		t.Fatalf("uploaded spot still placeholder: %q", ep.SpotImage)
	}
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/media/episodes/%d/%s", ep.ID, db.SpotPlaceholderFile), nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("placeholder alias after upload: %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/media/episodes/99999/"+db.SpotPlaceholderFile, nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown episode placeholder: %d", rec.Code)
	}

	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, err = mw.CreateFormFile("file", "bg.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(png1x1); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, idPath+"/stream-background", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "obs.example")
	req.SetBasicAuth("admin", "test-pass")
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream-background: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.StreamBackground != "/media/stream-background/latest" {
		t.Fatalf("stream_background: %q", ep.StreamBackground)
	}
	if ep.StreamBackgroundURL != "https://obs.example/media/stream-background/latest" {
		t.Fatalf("stream_background_url: %q", ep.StreamBackgroundURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/get/episode/sesh-sofa", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public: %d %s", rec.Code, rec.Body.String())
	}
	var pub map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pub); err != nil {
		t.Fatal(err)
	}
	if pub["air_datetime"] != "2026-10-01T20:00:00+02:00" {
		t.Fatalf("public air_datetime: %+v", pub)
	}
	spot, _ := pub["spot_image"].(string)
	if !strings.HasPrefix(spot, "/media/episodes/") {
		t.Fatalf("public spot_image: %+v", pub)
	}
	if pub["stream_background"] != "/media/stream-background/latest" {
		t.Fatalf("public stream_background: %+v", pub)
	}
	bgURL, _ := pub["stream_background_url"].(string)
	if !strings.HasSuffix(bgURL, "/media/stream-background/latest") || !strings.Contains(bgURL, "://") {
		t.Fatalf("public stream_background_url: %+v", pub)
	}

	req = httptest.NewRequest(http.MethodGet, "/media/stream-background/latest", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest media: %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("latest cache: %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.Len() < 8 || rec.Body.Bytes()[0] != 0x89 {
		t.Fatalf("latest body is not png")
	}

	req = httptest.NewRequest(http.MethodGet, "/stream-background", nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream-background page: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("stream-background page body: %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, spot, nil)
	rec = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("media: %d", rec.Code)
	}
	if rec.Body.Len() < 8 || rec.Body.Bytes()[0] != 0x89 {
		t.Fatalf("media body is not png")
	}
}

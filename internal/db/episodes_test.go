package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEpisodeCRUDAndTemplateResolve(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	if got := ResolveEpisodeNameFull("", "Sesh Sofa", 20, "Creature Park", "LIVE"); got != "EP20 Creature Park | Sesh Sofa | LIVE" {
		t.Fatalf("name full: %q", got)
	}
	if ShowSlug("Sesh Sofa") != "sesh-sofa" {
		t.Fatalf("slug: %q", ShowSlug("Sesh Sofa"))
	}

	tmpl, err := store.CreateEpisodeTemplate(ctx, "Sesh Sofa", "LIVE", "", []EpisodeListenerStub{
		{Kind: "content", GuildID: "g1", DiscordChannelID: "c1", Name: "{{episode_short}} {{name}}", PlaylistTitle: "{{episode_short}} {{name}}"},
		{Kind: "picture", GuildID: "g1", DiscordChannelID: "c2", Name: "{{episode_short}} pics", Slug: "{{episode_short}}_pics"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.ShowSlug != "sesh-sofa" || len(tmpl.Listeners) != 2 {
		t.Fatalf("template: %+v", tmpl)
	}

	ep, err := store.CreateEpisode(ctx, tmpl.Show, 20, "Creature Park", tmpl.TwitchSuffix, tmpl.NameFullTemplate, "2026-09-20", tmpl.Listeners)
	if err != nil {
		t.Fatal(err)
	}
	if ep.Short() != "EP20" || ep.NameFull() == "" {
		t.Fatalf("episode: %+v full=%q", ep, ep.NameFull())
	}

	// Second live episode same show must fail.
	if _, err := store.CreateEpisode(ctx, "Sesh Sofa", 21, "Next", "LIVE", "", "", nil); err == nil {
		t.Fatal("expected duplicate live show error")
	}

	got, err := store.GetLiveEpisodeByShowSlug(ctx, "SESH-SOFA")
	if err != nil || got == nil || got.ID != ep.ID {
		t.Fatalf("lookup by slug: %+v err=%v", got, err)
	}

	// Link listeners.
	if _, err := store.UpsertMapping(ctx, "c1", "g1", "pl1", "EP20 Creature Park", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMappingEpisodeID(ctx, "c1", ep.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertPictureListener(ctx, PictureListenerInput{
		DiscordChannelID: "c2", GuildID: "g1", Name: "EP20 pics", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPictureListenerEpisodeID(ctx, "c2", ep.ID); err != nil {
		t.Fatal(err)
	}

	maps, err := store.ListMappingsByEpisode(ctx, ep.ID)
	if err != nil || len(maps) != 1 {
		t.Fatalf("maps: %v %v", maps, err)
	}
	pics, err := store.ListPictureListenersByEpisode(ctx, ep.ID)
	if err != nil || len(pics) != 1 {
		t.Fatalf("pics: %v %v", pics, err)
	}

	if ep.AirDateTime != "2026-09-20T00:00:00+02:00" {
		t.Fatalf("air datetime: %+v", ep)
	}

	name := "Renamed"
	air := "2026-09-20T20:00:00+02:00"
	if _, err := store.UpdateEpisodeMutable(ctx, ep.ID, &name, nil, nil, &air); err != nil {
		t.Fatal(err)
	}
	ep2, _ := store.GetEpisodeByID(ctx, ep.ID)
	if ep2.Name != "Renamed" || ep2.Show != "Sesh Sofa" || ep2.Episode != 20 || ep2.AirDateTime != air {
		t.Fatalf("mutable update: %+v", ep2)
	}

	if _, err := store.SaveEpisodeSpot(ctx, ep.ID, ".png", []byte("not-a-real-png")); err != nil {
		t.Fatal(err)
	}
	ep3, _ := store.GetEpisodeByID(ctx, ep.ID)
	if ep3.SpotPath == "" || ep3.SpotURL() == "" {
		t.Fatalf("spot: %+v", ep3)
	}

	if err := store.CeaseEpisode(ctx, ep.ID); err != nil {
		t.Fatal(err)
	}
	live, err := store.GetLiveEpisodeByShowSlug(ctx, "sesh-sofa")
	if err != nil || live != nil {
		t.Fatalf("after cease: %+v err=%v", live, err)
	}

	all, err := store.ListEpisodes(ctx)
	if err != nil || len(all) != 1 || all[0].ID != ep.ID || all[0].ActiveUntil == nil {
		t.Fatalf("list after cease: %+v err=%v", all, err)
	}

	// New episode same show ok after cease.
	next, err := store.CreateEpisode(ctx, "Sesh Sofa", 21, "Next", "LIVE", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	all, err = store.ListEpisodes(ctx)
	if err != nil || len(all) != 2 || all[0].ID != next.ID || all[0].ActiveUntil != nil || all[1].ID != ep.ID {
		t.Fatalf("list live-first: %+v err=%v", all, err)
	}
	onlyLive, err := store.ListLiveEpisodes(ctx)
	if err != nil || len(onlyLive) != 1 || onlyLive[0].ID != next.ID {
		t.Fatalf("list live: %+v err=%v", onlyLive, err)
	}
}

func TestEpisodePlaceholderMap(t *testing.T) {
	air := "2026-09-20T20:00:00+02:00"
	ep := Episode{
		ID:               7,
		Show:             "Sesh Sofa",
		ShowSlug:         "sesh-sofa",
		Episode:          20,
		Name:             "Creature Park",
		TwitchSuffix:     "LIVE",
		NameFullTemplate: "",
		AirDateTime:      air,
		SpotPath:         "spot.jpg",
		ActiveFrom:       time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC),
	}
	m := ep.PlaceholderMap()
	if m["show_slug"] != "sesh-sofa" {
		t.Fatalf("show_slug: %q", m["show_slug"])
	}
	if m["episode_name_full"] != "EP20 Creature Park | Sesh Sofa | LIVE" {
		t.Fatalf("episode_name_full: %q", m["episode_name_full"])
	}
	if m["air_datetime"] != air || m["air_date"] != "2026-09-20" || m["air_time"] != "20:00" {
		t.Fatalf("air: %+v", m)
	}
	if m["spot_image"] != "/media/episodes/7/spot.jpg" {
		t.Fatalf("spot_image: %q", m["spot_image"])
	}

	empty := Episode{ID: 7}
	if empty.SpotURL() != "/media/episodes/7/"+SpotPlaceholderFile {
		t.Fatalf("placeholder url: %q", empty.SpotURL())
	}
	if m["since"] != "2026-09-19T18:00:00Z" {
		t.Fatalf("since: %q", m["since"])
	}
	if _, ok := m["playlist_id"]; ok {
		t.Fatal("stub map must not include empty playlist_id")
	}
	keys := EpisodePlaceholderKeys()
	if len(keys) < 10 || keys[0] != "{{show}}" {
		t.Fatalf("keys: %v", keys)
	}
}

func TestNormalizeAirDateTime(t *testing.T) {
	got, err := NormalizeAirDateTime("2026-09-20T20:00:00+02:00")
	if err != nil || got != "2026-09-20T20:00:00+02:00" {
		t.Fatalf("rfc3339: %q %v", got, err)
	}
	got, err = NormalizeAirDateTime("2026-09-20")
	if err != nil || got != "2026-09-20T00:00:00+02:00" {
		t.Fatalf("legacy date: %q %v", got, err)
	}
	if _, err := NormalizeAirDateTime("2026-09-20T20:00:00"); err == nil {
		t.Fatal("expected missing-offset error")
	}
}

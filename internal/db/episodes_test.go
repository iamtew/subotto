package db

import (
	"context"
	"path/filepath"
	"testing"
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

	ep, err := store.CreateEpisode(ctx, tmpl.Show, 20, "Creature Park", tmpl.TwitchSuffix, tmpl.NameFullTemplate, tmpl.Listeners)
	if err != nil {
		t.Fatal(err)
	}
	if ep.Short() != "EP20" || ep.NameFull() == "" {
		t.Fatalf("episode: %+v full=%q", ep, ep.NameFull())
	}

	// Second live episode same show must fail.
	if _, err := store.CreateEpisode(ctx, "Sesh Sofa", 21, "Next", "LIVE", "", nil); err == nil {
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

	name := "Renamed"
	if _, err := store.UpdateEpisodeMutable(ctx, ep.ID, &name, nil, nil); err != nil {
		t.Fatal(err)
	}
	ep2, _ := store.GetEpisodeByID(ctx, ep.ID)
	if ep2.Name != "Renamed" || ep2.Show != "Sesh Sofa" || ep2.Episode != 20 {
		t.Fatalf("mutable update: %+v", ep2)
	}

	if err := store.CeaseEpisode(ctx, ep.ID); err != nil {
		t.Fatal(err)
	}
	live, err := store.GetLiveEpisodeByShowSlug(ctx, "sesh-sofa")
	if err != nil || live != nil {
		t.Fatalf("after cease: %+v err=%v", live, err)
	}

	// New episode same show ok after cease.
	if _, err := store.CreateEpisode(ctx, "Sesh Sofa", 21, "Next", "LIVE", "", nil); err != nil {
		t.Fatal(err)
	}
}

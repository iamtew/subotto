// Package main is the entry point for Subotto.
//
// Meat Bag:
//   - `just run` — Discord bot + Admin UI + YouTube (needs tokens + auth-youtube)
//   - `just auth-youtube` — one-time Google OAuth
//   - `just add-mapping CHANNEL PLAYLIST` — map a Discord channel to a playlist
//   - `just list-mappings` / enable / disable / delete / resync — Phase 4 CRUD
//   - Admin UI at http://localhost:50770 (user admin / ADMIN_PASSWORD)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"subotto/internal/config"
	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/scheduler"
	"subotto/internal/web"
	"subotto/internal/youtube"
)

func main() {
	youtubeAuth := flag.Bool("youtube-auth", false, "Run the one-time YouTube OAuth flow and exit")
	addChannel := flag.String("add-mapping-channel", "", "Discord channel ID to map (use with -add-mapping-playlist)")
	addPlaylist := flag.String("add-mapping-playlist", "", "YouTube playlist ID to map")
	addName := flag.String("add-mapping-name", "", "Optional label for the mapping")
	addGuild := flag.String("add-mapping-guild", "", "Optional Discord guild/server ID")

	listMappings := flag.Bool("list-mappings", false, "List all channel ↔ playlist mappings and exit")
	enableMapping := flag.String("enable-mapping", "", "Enable mapping for this Discord channel ID")
	disableMapping := flag.String("disable-mapping", "", "Disable mapping for this Discord channel ID")
	deleteMapping := flag.String("delete-mapping", "", "Delete mapping for this Discord channel ID")
	resyncChannel := flag.String("resync-channel", "", "Rescan recent messages in this Discord channel (content / YouTube)")
	resyncLimit := flag.Int("resync-limit", 100, "How many recent messages to scan (max 500)")
	resyncPictureChannel := flag.String("resync-picture-channel", "", "Rescan recent messages for image attachments (picture listener)")

	addPicChannel := flag.String("add-picture-channel", "", "Discord channel ID for a picture listener")
	addPicName := flag.String("add-picture-name", "", "Picture listener name (becomes slideshow slug)")
	addPicGuild := flag.String("add-picture-guild", "", "Optional Discord guild/server ID")
	listPictureListens := flag.Bool("list-picture-listens", false, "List live picture listeners and exit")
	enablePicture := flag.String("enable-picture", "", "Enable picture listener for this Discord channel ID")
	disablePicture := flag.String("disable-picture", "", "Disable picture listener for this Discord channel ID")
	deletePicture := flag.String("delete-picture", "", "Cease picture listener for this Discord channel ID")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	store, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := store.Close(); cerr != nil {
			slog.Error("failed to close database", "err", cerr)
		}
	}()

	ctx := context.Background()

	if *youtubeAuth {
		if err := runYouTubeAuth(ctx, cfg, store); err != nil {
			slog.Error("YouTube authorization failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *listMappings {
		if err := runListMappings(ctx, store); err != nil {
			slog.Error("list mappings failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *enableMapping != "" {
		if err := runSetMappingEnabled(ctx, store, *enableMapping, true); err != nil {
			slog.Error("enable mapping failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *disableMapping != "" {
		if err := runSetMappingEnabled(ctx, store, *disableMapping, false); err != nil {
			slog.Error("disable mapping failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *deleteMapping != "" {
		if err := runDeleteMapping(ctx, store, *deleteMapping); err != nil {
			slog.Error("delete mapping failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *resyncChannel != "" {
		if err := runResync(ctx, cfg, store, *resyncChannel, *resyncLimit); err != nil {
			slog.Error("resync failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *resyncPictureChannel != "" {
		if err := runPictureResync(ctx, cfg, store, *resyncPictureChannel, *resyncLimit); err != nil {
			slog.Error("picture resync failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *listPictureListens {
		if err := runListPictureListens(ctx, store); err != nil {
			slog.Error("list picture listens failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *enablePicture != "" {
		if err := runSetPictureEnabled(ctx, store, *enablePicture, true); err != nil {
			slog.Error("enable picture listener failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *disablePicture != "" {
		if err := runSetPictureEnabled(ctx, store, *disablePicture, false); err != nil {
			slog.Error("disable picture listener failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *deletePicture != "" {
		if err := runDeletePicture(ctx, store, *deletePicture); err != nil {
			slog.Error("cease picture listener failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *addPicChannel != "" || *addPicName != "" {
		if err := runAddPicture(ctx, cfg, store, *addPicChannel, *addPicName, *addPicGuild); err != nil {
			slog.Error("add picture listener failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if *addChannel != "" || *addPlaylist != "" {
		if err := runAddMapping(ctx, cfg, store, *addChannel, *addPlaylist, *addName, *addGuild); err != nil {
			slog.Error("add mapping failed", "err", err)
			os.Exit(1)
		}
		return
	}

	runBot(ctx, cfg, store)
}

func runYouTubeAuth(ctx context.Context, cfg *config.Config, store *db.DB) error {
	missing := cfg.MissingYouTubeSecrets()
	if len(missing) > 0 {
		return errors.New("set these in .env first: " + strings.Join(missing, ", "))
	}

	slog.Info("starting YouTube OAuth — browser should open; approve playlist access")
	if _, err := youtube.AuthorizeInteractive(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL); err != nil {
		return err
	}

	client, err := youtube.NewClient(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL)
	if err != nil {
		return err
	}
	title, err := client.Ping(ctx)
	if err != nil {
		_ = store.LogActivity(ctx, "youtube_auth", map[string]any{"ok": false, "error": err.Error()}, false)
		return err
	}
	_ = store.LogActivity(ctx, "youtube_auth", map[string]any{"ok": true, "channel": title}, true)
	slog.Info("YouTube authorization OK", "channel", title)
	slog.Info("You can `just run` normally now.")
	return nil
}

func runAddMapping(ctx context.Context, cfg *config.Config, store *db.DB, channelID, playlistID, name, guildID string) error {
	if channelID == "" || playlistID == "" {
		return errors.New("need both -add-mapping-channel and -add-mapping-playlist")
	}
	if guildID == "" {
		guildID = cfg.DiscordGuildID
	}
	if name == "" {
		name = "mapping-" + channelID
	}
	m, err := store.UpsertMapping(ctx, channelID, guildID, playlistID, name, true)
	if err != nil {
		return err
	}
	_ = store.LogActivity(ctx, "mapping_upserted", map[string]any{
		"channel_id":  m.DiscordChannelID,
		"playlist_id": m.YouTubePlaylistID,
		"name":        m.Name,
	}, true)
	slog.Info("channel mapping saved",
		"channel", m.DiscordChannelID,
		"playlist", m.YouTubePlaylistID,
		"name", m.Name,
		"guild", m.GuildID,
	)
	return nil
}

// runListMappings prints a human-readable table (not slog key=value).
func runListMappings(ctx context.Context, store *db.DB) error {
	list, err := store.ListMappings(ctx)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("No channel mappings yet.")
		fmt.Println("Add one with: just add-mapping DISCORD_CHANNEL_ID YOUTUBE_PLAYLIST_ID")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tENABLED\tNAME\tCHANNEL\tPLAYLIST\tGUILD")
	for _, m := range list {
		enabled := "yes"
		if !m.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			m.ID, enabled, m.Name, m.DiscordChannelID, m.YouTubePlaylistID, m.GuildID)
	}
	return w.Flush()
}

func runSetMappingEnabled(ctx context.Context, store *db.DB, channelID string, enabled bool) error {
	m, err := store.SetMappingEnabled(ctx, channelID, enabled)
	if err != nil {
		return err
	}
	event := "mapping_enabled"
	if !enabled {
		event = "mapping_disabled"
	}
	_ = store.LogActivity(ctx, event, map[string]any{
		"channel_id":  m.DiscordChannelID,
		"playlist_id": m.YouTubePlaylistID,
		"name":        m.Name,
	}, true)
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	slog.Info("channel mapping "+state,
		"channel", m.DiscordChannelID,
		"playlist", m.YouTubePlaylistID,
		"name", m.Name,
	)
	return nil
}

func runDeleteMapping(ctx context.Context, store *db.DB, channelID string) error {
	// Grab details before delete so the activity log is useful.
	m, err := store.GetMappingByChannel(ctx, channelID)
	if err != nil {
		return err
	}
	if err := store.DeleteMapping(ctx, channelID); err != nil {
		return err
	}
	details := map[string]any{"channel_id": channelID}
	if m != nil {
		details["playlist_id"] = m.YouTubePlaylistID
		details["name"] = m.Name
	}
	_ = store.LogActivity(ctx, "mapping_deleted", details, true)
	slog.Info("channel mapping deleted", "channel", channelID)
	return nil
}

func runAddPicture(ctx context.Context, cfg *config.Config, store *db.DB, channelID, name, guildID string) error {
	if channelID == "" || name == "" {
		return errors.New("need both -add-picture-channel and -add-picture-name")
	}
	if guildID == "" {
		guildID = cfg.DiscordGuildID
	}
	p, err := store.UpsertPictureListener(ctx, db.PictureListenerInput{
		DiscordChannelID: channelID,
		GuildID:          guildID,
		Name:             name,
		Enabled:          true,
		CreditCorner:     db.CreditCornerBR,
		IntervalSeconds:  8,
		ShowCredit:       true,
		ShowReactions:    true,
	})
	if err != nil {
		return err
	}
	_ = store.LogActivity(ctx, "picture_listen_started", map[string]any{
		"channel_id": p.DiscordChannelID,
		"slug":       p.Slug,
		"name":       p.Name,
		"source":     "cli",
	}, true)
	slog.Info("picture listener saved",
		"channel", p.DiscordChannelID,
		"slug", p.Slug,
		"name", p.Name,
		"slideshow", "/slideshow/"+p.Slug,
	)
	return nil
}

func runListPictureListens(ctx context.Context, store *db.DB) error {
	list, err := store.ListPictureListeners(ctx)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("No picture listeners yet.")
		fmt.Println("Add one with: just start-picture-listen DISCORD_CHANNEL_ID \"Show Name\"")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tENABLED\tNAME\tSLUG\tCHANNEL\tGUILD")
	for _, p := range list {
		enabled := "yes"
		if !p.Enabled {
			enabled = "no"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			p.ID, enabled, p.Name, p.Slug, p.DiscordChannelID, p.GuildID)
	}
	return w.Flush()
}

func runSetPictureEnabled(ctx context.Context, store *db.DB, channelID string, enabled bool) error {
	p, err := store.SetPictureListenerEnabled(ctx, channelID, enabled)
	if err != nil {
		return err
	}
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	slog.Info("picture listener "+state, "channel", p.DiscordChannelID, "slug", p.Slug)
	return nil
}

func runDeletePicture(ctx context.Context, store *db.DB, channelID string) error {
	p, err := store.GetPictureListenerByChannel(ctx, channelID)
	if err != nil {
		return err
	}
	if err := store.DeletePictureListener(ctx, channelID); err != nil {
		return err
	}
	details := map[string]any{"channel_id": channelID}
	if p != nil {
		details["slug"] = p.Slug
		details["name"] = p.Name
	}
	_ = store.LogActivity(ctx, "picture_listen_ceased", details, true)
	slog.Info("picture listener ceased", "channel", channelID)
	return nil
}

func runResync(ctx context.Context, cfg *config.Config, store *db.DB, channelID string, limit int) error {
	if cfg.DiscordBotToken == "" || cfg.DiscordBotToken == "your-discord-bot-token-here" {
		return errors.New("DISCORD_BOT_TOKEN is required for resync — set it in .env")
	}
	if len(cfg.MissingYouTubeSecrets()) > 0 {
		return errors.New("YouTube OAuth client id/secret required: " + strings.Join(cfg.MissingYouTubeSecrets(), ", "))
	}
	hasTok, err := youtube.HasStoredToken(ctx, store)
	if err != nil {
		return err
	}
	if !hasTok {
		return errors.New("no YouTube token yet — run `just auth-youtube` first")
	}

	yt, err := youtube.NewClient(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL)
	if err != nil {
		return err
	}

	slog.Info("starting channel resync", "channel", channelID, "limit", limit)
	summary, err := discord.ResyncChannel(ctx, cfg.DiscordBotToken, store, yt, channelID, limit)
	if err != nil {
		return err
	}
	fmt.Printf("Resync done: scanned=%d %s\n", summary.MessagesScanned, summary.Result.String())
	return nil
}

func runPictureResync(ctx context.Context, cfg *config.Config, store *db.DB, channelID string, limit int) error {
	if cfg.DiscordBotToken == "" || cfg.DiscordBotToken == "your-discord-bot-token-here" {
		return errors.New("DISCORD_BOT_TOKEN is required for picture resync — set it in .env")
	}

	slog.Info("starting picture channel resync", "channel", channelID, "limit", limit)
	summary, err := discord.ResyncPictureChannel(ctx, cfg.DiscordBotToken, store, channelID, limit)
	if err != nil {
		return err
	}
	fmt.Printf("Picture resync done: scanned=%d %s\n", summary.MessagesScanned, summary.Result.String())
	return nil
}

func runBot(ctx context.Context, cfg *config.Config, store *db.DB) {
	slog.Info("Subotto is online. Clanker stands ready, Meat Bag.")
	slog.Info("config loaded",
		"database_path", cfg.DatabasePath,
		"log_level", cfg.LogLevel,
		"admin_host", cfg.AdminHost,
		"admin_port", cfg.AdminPort,
	)

	if cfg.DiscordBotToken == "" || cfg.DiscordBotToken == "your-discord-bot-token-here" {
		slog.Error("DISCORD_BOT_TOKEN is required — set it in .env")
		os.Exit(1)
	}
	if len(cfg.MissingYouTubeSecrets()) > 0 {
		slog.Error("YouTube OAuth client id/secret required", "missing", strings.Join(cfg.MissingYouTubeSecrets(), ", "))
		os.Exit(1)
	}
	hasTok, err := youtube.HasStoredToken(ctx, store)
	if err != nil {
		slog.Error("failed to check youtube token", "err", err)
		os.Exit(1)
	}
	if !hasTok {
		slog.Error("no YouTube token yet — run `just auth-youtube` first")
		os.Exit(1)
	}

	yt, err := youtube.NewClient(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL)
	if err != nil {
		slog.Error("failed to create youtube client", "err", err)
		os.Exit(1)
	}
	ytChannel := ""
	if title, err := yt.Ping(ctx); err != nil {
		slog.Warn("YouTube ping failed", "err", err)
	} else {
		ytChannel = title
		slog.Info("YouTube client ready", "channel", title)
	}

	mappings, err := store.CountMappings(ctx)
	if err != nil {
		slog.Error("failed to count mappings", "err", err)
		os.Exit(1)
	}
	if mappings == 0 {
		slog.Warn("no channel mappings yet — use Admin UI or: just add-mapping CHANNEL_ID PLAYLIST_ID")
	} else {
		slog.Info("channel mappings loaded", "count", mappings)
	}

	if err := store.LogActivity(ctx, "startup", map[string]any{"message": "Phase 8 boot", "phase": 8}, true); err != nil {
		slog.Error("failed to write startup activity", "err", err)
		os.Exit(1)
	}

	bot, err := discord.New(cfg.DiscordBotToken, store, yt, cfg.DiscordGuildID)
	if err != nil {
		slog.Error("failed to create discord bot", "err", err)
		os.Exit(1)
	}
	if err := bot.Open(); err != nil {
		slog.Error("failed to connect to discord", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := bot.Close(); cerr != nil {
			slog.Error("failed to close discord", "err", cerr)
		}
	}()

	sched := scheduler.New(scheduler.Options{
		Store:               store,
		YouTube:             yt,
		DiscordToken:        cfg.DiscordBotToken,
		ResyncIntervalHours: cfg.ResyncIntervalHours,
	})

	admin, err := web.New(web.Options{
		Store:          store,
		YouTube:        yt,
		Status:         bot,
		Scheduler:      sched,
		Discord:        bot,
		DiscordToken:   cfg.DiscordBotToken,
		AdminPassword:  cfg.AdminPassword,
		AdminHost:      cfg.AdminHost,
		AdminPort:      cfg.AdminPort,
		Webroot:        "webroot",
		YouTubeChannel: ytChannel,
	})
	if err != nil {
		slog.Error("failed to create admin UI server", "err", err)
		os.Exit(1)
	}

	adminErr := make(chan error, 1)
	go func() {
		adminErr <- admin.Start()
	}()

	// Scheduler shares this cancel with shutdown so ticks stop promptly.
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	go sched.Run(schedCtx)

	slog.Info("listening for content + picture listeners — Ctrl+C to stop")
	slog.Info("Admin UI ready", "url", fmt.Sprintf("http://%s", admin.Addr()), "user", "admin")
	slog.Info("public slideshow", "latest", fmt.Sprintf("http://%s/slideshow/latest", admin.Addr()))
	if sched.Enabled() {
		slog.Info("background resync enabled", "interval_hours", cfg.ResyncIntervalHours)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		slog.Info("shutting down Subotto — bye Meat Bag")
	case err := <-adminErr:
		if err != nil {
			slog.Error("admin UI server stopped", "err", err)
		}
	}

	schedCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := admin.Shutdown(shutdownCtx); err != nil {
		slog.Error("admin UI shutdown", "err", err)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}

// Package main is the entry point for Subotto.
//
// Meat Bag:
//   - `just run` — Discord bot + YouTube (needs tokens + auth-youtube)
//   - `just auth-youtube` — one-time Google OAuth
//   - `just add-mapping CHANNEL PLAYLIST` — map a Discord channel to a playlist
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"subotto/internal/config"
	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/youtube"
)

func main() {
	youtubeAuth := flag.Bool("youtube-auth", false, "Run the one-time YouTube OAuth flow and exit")
	addChannel := flag.String("add-mapping-channel", "", "Discord channel ID to map (use with -add-mapping-playlist)")
	addPlaylist := flag.String("add-mapping-playlist", "", "YouTube playlist ID to map")
	addName := flag.String("add-mapping-name", "", "Optional label for the mapping")
	addGuild := flag.String("add-mapping-guild", "", "Optional Discord guild/server ID")
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

func runBot(ctx context.Context, cfg *config.Config, store *db.DB) {
	slog.Info("Subotto is online. Clanker stands ready, Meat Bag.")
	slog.Info("config loaded",
		"database_path", cfg.DatabasePath,
		"log_level", cfg.LogLevel,
		"admin_host", cfg.AdminHost,
		"admin_port", cfg.AdminPort,
	)

	if cfg.DiscordBotToken == "" || cfg.DiscordBotToken == "your-discord-bot-token-here" {
		slog.Error("DISCORD_BOT_TOKEN is required for Phase 3 — set it in .env")
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
	if title, err := yt.Ping(ctx); err != nil {
		slog.Warn("YouTube ping failed", "err", err)
	} else {
		slog.Info("YouTube client ready", "channel", title)
	}

	mappings, err := store.CountMappings(ctx)
	if err != nil {
		slog.Error("failed to count mappings", "err", err)
		os.Exit(1)
	}
	if mappings == 0 {
		slog.Warn("no channel mappings yet — add one with: just add-mapping CHANNEL_ID PLAYLIST_ID")
	} else {
		slog.Info("channel mappings loaded", "count", mappings)
	}

	if err := store.LogActivity(ctx, "startup", map[string]any{"message": "Phase 3 boot", "phase": 3}, true); err != nil {
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

	slog.Info("listening for YouTube links in mapped channels — Ctrl+C to stop")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down Subotto — bye Meat Bag")
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

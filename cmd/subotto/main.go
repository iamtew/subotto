// Package main is the entry point for Subotto.
//
// Meat Bag:
//   - `just run` — Discord bot + Admin UI + YouTube (needs tokens + auth-youtube)
//   - `just auth-youtube` — DEV Google OAuth (Admin Status can re-auth too)
//   - Admin landing at http://localhost:50770 — Discord OAuth and/or Basic admin / ADMIN_PASSWORD
//   - Admin UI at http://localhost:50770/admin
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
	"time"

	"subotto/internal/ai"
	"subotto/internal/config"
	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/scheduler"
	"subotto/internal/web"
	"subotto/internal/youtube"
)

func main() {
	youtubeAuth := flag.Bool("youtube-auth", false, "Run the one-time YouTube OAuth flow and exit")
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
	// Same Client pointer for Discord, scheduler, and Admin so OAuth Reload
	// picks up a new refresh token without a process restart.
	yt := &youtube.Client{}
	ytChannel := ""
	hasTok, err := youtube.HasStoredToken(ctx, store)
	if err != nil {
		slog.Error("failed to check youtube token", "err", err)
		os.Exit(1)
	}
	if !hasTok {
		slog.Warn("no YouTube token yet — authorize in Admin (Status → Authorize YouTube)")
	} else if err := yt.Reload(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL); err != nil {
		slog.Warn("YouTube client not ready", "err", err)
	} else if title, err := yt.Ping(ctx); err != nil {
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
		slog.Warn("no channel mappings yet — start a content listener in Admin")
	} else {
		slog.Info("channel mappings loaded", "count", mappings)
	}

	aiClient := ai.New(cfg.OpenRouterAPIKey)
	if aiClient.Configured() {
		catalog, err := store.LoadAIModels(ctx, cfg.OpenRouterModel)
		model := ai.DefaultModel
		if err != nil {
			slog.Error("hesh helper model load failed", "err", err)
		} else {
			model = catalog.Model
		}
		slog.Info("hesh helper ready", "model", model)
	} else {
		slog.Info("hesh helper disabled — set OPENROUTER_API_KEY in .env")
	}

	if err := store.LogActivity(ctx, "startup", map[string]any{"message": "Phase 8 boot", "phase": 8}, true); err != nil {
		slog.Error("failed to write startup activity", "err", err)
		os.Exit(1)
	}

	bot, err := discord.New(cfg.DiscordBotToken, store, yt, cfg.DiscordGuildID, aiClient, cfg.OpenRouterModel, cfg.SuperadminDiscordID)
	if err != nil {
		slog.Error("failed to create discord bot", "err", err)
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
		Store:                   store,
		YouTube:                 yt,
		Status:                  bot,
		Scheduler:               sched,
		Discord:                 bot,
		DiscordToken:            cfg.DiscordBotToken,
		AdminPassword:           cfg.AdminPassword,
		APIPassword:             cfg.APIPassword,
		AdminHost:               cfg.AdminHost,
		AdminPort:               cfg.AdminPort,
		Webroot:                 "webroot",
		YouTubeChannel:          ytChannel,
		ResyncIntervalHours:     cfg.ResyncIntervalHours,
		YouTubeClientID:         cfg.YouTubeClientID,
		YouTubeClientSecret:     cfg.YouTubeClientSecret,
		YouTubeRedirectURL:      cfg.YouTubeRedirectURL,
		DiscordClientID:         cfg.DiscordClientID,
		DiscordClientSecret:     cfg.DiscordClientSecret,
		DiscordOAuthRedirectURL: cfg.DiscordOAuthRedirectURL,
		SuperadminDiscordID:     cfg.SuperadminDiscordID,
		AI:                      aiClient,
		OpenRouterModel:         cfg.OpenRouterModel,
	})
	if err != nil {
		slog.Error("failed to create admin UI server", "err", err)
		os.Exit(1)
	}

	adminErr := make(chan error, 1)
	go func() {
		adminErr <- admin.Start()
	}()

	// Shared cancel: Discord maintain + scheduler stop together on SIGINT.
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	// Keep Discord online in the background — do not exit if the first Open fails.
	go bot.Maintain(runCtx)
	go sched.Run(runCtx)

	slog.Info("listening for content + picture listeners — Ctrl+C to stop")
	slog.Info("Admin UI ready", "url", fmt.Sprintf("http://%s/admin", admin.Addr()), "landing", fmt.Sprintf("http://%s/", admin.Addr()))
	slog.Info("public slideshow", "latest", fmt.Sprintf("http://%s/slideshow/latest", admin.Addr()))
	if info := sched.Info(); info.Enabled {
		slog.Info("background resync enabled", "interval_hours", info.IntervalHours, "resync_limit", info.Limit)
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

	runCancel()

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

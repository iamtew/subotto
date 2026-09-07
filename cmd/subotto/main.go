// Package main is the entry point for Subotto.
//
// Meat Bag:
//   - `just run` — normal boot (config + DB + YouTube client if authorized)
//   - `just auth-youtube` — one-time Google OAuth so Subotto can edit playlists
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"strings"

	"subotto/internal/config"
	"subotto/internal/db"
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

	runNormal(ctx, cfg, store)
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
	slog.Info("Phase 2 auth done. You can `just run` normally now.")
	return nil
}

func runNormal(ctx context.Context, cfg *config.Config, store *db.DB) {
	slog.Info("Subotto is online. Clanker stands ready, Meat Bag.")
	slog.Info("config loaded",
		"database_path", cfg.DatabasePath,
		"log_level", cfg.LogLevel,
		"admin_host", cfg.AdminHost,
		"admin_port", cfg.AdminPort,
		"resync_interval_hours", cfg.ResyncIntervalHours,
	)

	if missing := cfg.MissingSecrets(); len(missing) > 0 {
		slog.Warn("some API credentials are not set yet",
			"missing", strings.Join(missing, ", "),
			"hint", "copy .env.example to .env; for YouTube also run `just auth-youtube`",
		)
	}

	if err := store.LogActivity(ctx, "startup", map[string]any{
		"message": "Phase 2 boot",
		"phase":   2,
	}, true); err != nil {
		slog.Error("failed to write startup activity", "err", err)
		os.Exit(1)
	}

	mappings, err := store.CountMappings(ctx)
	if err != nil {
		slog.Error("failed to count mappings", "err", err)
		os.Exit(1)
	}
	activities, err := store.CountActivity(ctx)
	if err != nil {
		slog.Error("failed to count activity", "err", err)
		os.Exit(1)
	}
	slog.Info("database ready",
		"path", cfg.DatabasePath,
		"mappings", mappings,
		"activity_rows", activities,
	)

	// YouTube client: optional until Meat Bag finishes OAuth.
	if len(cfg.MissingYouTubeSecrets()) > 0 {
		slog.Warn("YouTube client skipped — OAuth client id/secret not configured")
	} else {
		hasTok, err := youtube.HasStoredToken(ctx, store)
		if err != nil {
			slog.Error("failed to check youtube token", "err", err)
			os.Exit(1)
		}
		if !hasTok {
			slog.Warn("YouTube client skipped — no token yet",
				"hint", "run `just auth-youtube` once after filling YOUTUBE_CLIENT_ID/SECRET",
			)
		} else {
			yt, err := youtube.NewClient(ctx, store, cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeRedirectURL)
			if err != nil {
				slog.Error("failed to create youtube client", "err", err)
				os.Exit(1)
			}
			title, err := yt.Ping(ctx)
			if err != nil {
				slog.Warn("YouTube token present but ping failed", "err", err)
			} else {
				slog.Info("YouTube client ready", "channel", title)
			}
		}
	}

	slog.Info("Phase 2 ready — OAuth + AddVideoToPlaylist are implemented. Next: Discord bot core (Phase 3).")
}

// newLogger builds a text slog logger at the requested level.
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

// Package main is the entry point for Subotto.
//
// Meat Bag: run with `just run`. Phase 1 loads config, sets up structured
// logging (slog), opens the SQLite database, and writes a startup activity
// row so we know the DB works. Discord and YouTube come in later phases.
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"subotto/internal/config"
	"subotto/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Logger is not ready yet — plain stderr is fine for boot failures.
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	slog.Info("Subotto is online. Clanker stands ready, Meat Bag.")
	slog.Info("config loaded",
		"database_path", cfg.DatabasePath,
		"log_level", cfg.LogLevel,
		"admin_host", cfg.AdminHost,
		"admin_port", cfg.AdminPort,
		"resync_interval_hours", cfg.ResyncIntervalHours,
	)

	if missing := cfg.MissingSecrets(); len(missing) > 0 {
		slog.Warn("some API credentials are not set yet (ok for Phase 1)",
			"missing", strings.Join(missing, ", "),
			"hint", "copy .env.example to .env and fill them in before Phase 2/3",
		)
	}

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
	if err := store.LogActivity(ctx, "startup", map[string]any{
		"message": "Phase 1 boot OK",
		"phase":   1,
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
	slog.Info("Phase 1 complete for this run — config + SQLite + slog are wired. Next: YouTube OAuth (Phase 2).")
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

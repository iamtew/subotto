// Package config loads Subotto settings from the environment.
//
// Meat Bag: copy .env.example to .env, fill in your tokens, and Subotto
// will pick them up on startup. You can also set real OS environment
// variables instead — those always win over the .env file.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds every setting Subotto needs to run.
// Phase 1 only *uses* DatabasePath and LogLevel, but we load the rest
// early so later phases (Discord, YouTube, Admin UI) have them ready.
type Config struct {
	// Discord
	DiscordBotToken string
	DiscordGuildID  string // optional: limit the bot to one server

	// YouTube / Google OAuth
	YouTubeClientID     string
	YouTubeClientSecret string
	YouTubeRedirectURL  string

	// Admin UI
	AdminPassword string
	APIPassword   string // optional; Basic Auth user "api" for broadcast fire GETs
	AdminPort     int
	AdminHost     string

	// Database
	DatabasePath string

	// Optional behaviour
	LogLevel            string // debug, info, warn, error
	ResyncIntervalHours int    // 0 = disabled
}

// Load reads optional .env, then environment variables, then applies defaults.
// It returns an error if something is clearly broken (e.g. a non-number port).
func Load() (*Config, error) {
	// Try to load .env from the current working directory.
	// Missing file is fine — Meat Bag might set env vars another way.
	_ = godotenv.Load()

	cfg := &Config{
		DiscordBotToken:     os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordGuildID:      os.Getenv("DISCORD_GUILD_ID"),
		YouTubeClientID:     os.Getenv("YOUTUBE_CLIENT_ID"),
		YouTubeClientSecret: os.Getenv("YOUTUBE_CLIENT_SECRET"),
		YouTubeRedirectURL:  envOr("YOUTUBE_REDIRECT_URL", "http://localhost:50770/oauth/callback"),
		AdminPassword:       envOr("ADMIN_PASSWORD", "change-me-please"),
		APIPassword:         strings.TrimSpace(os.Getenv("API_PASSWORD")),
		AdminHost:           envOr("ADMIN_HOST", "0.0.0.0"),
		DatabasePath:        envOr("DATABASE_PATH", "./data/subotto.db"),
		LogLevel:            strings.ToLower(envOr("LOG_LEVEL", "info")),
	}

	port, err := strconv.Atoi(envOr("ADMIN_PORT", "50770"))
	if err != nil {
		return nil, fmt.Errorf("ADMIN_PORT must be a number: %w", err)
	}
	cfg.AdminPort = port

	resync, err := strconv.Atoi(envOr("RESYNC_INTERVAL_HOURS", "0"))
	if err != nil {
		return nil, fmt.Errorf("RESYNC_INTERVAL_HOURS must be a number: %w", err)
	}
	cfg.ResyncIntervalHours = resync

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate checks values we can verify without talking to Discord/YouTube yet.
func (c *Config) validate() error {
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error (got %q)", c.LogLevel)
	}
	if c.AdminPort < 1 || c.AdminPort > 65535 {
		return fmt.Errorf("ADMIN_PORT must be between 1 and 65535 (got %d)", c.AdminPort)
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return fmt.Errorf("DATABASE_PATH must not be empty")
	}
	if c.ResyncIntervalHours < 0 {
		return fmt.Errorf("RESYNC_INTERVAL_HOURS must be >= 0")
	}
	return nil
}

// MissingSecrets lists Discord/YouTube credentials that are still empty.
func (c *Config) MissingSecrets() []string {
	var missing []string
	if strings.TrimSpace(c.DiscordBotToken) == "" || c.DiscordBotToken == "your-discord-bot-token-here" {
		missing = append(missing, "DISCORD_BOT_TOKEN")
	}
	missing = append(missing, c.MissingYouTubeSecrets()...)
	return missing
}

// MissingYouTubeSecrets lists Google OAuth client settings still empty/placeholder.
// Needed before `just auth-youtube` (Phase 2).
func (c *Config) MissingYouTubeSecrets() []string {
	var missing []string
	if strings.TrimSpace(c.YouTubeClientID) == "" || c.YouTubeClientID == "your-google-oauth-client-id" {
		missing = append(missing, "YOUTUBE_CLIENT_ID")
	}
	if strings.TrimSpace(c.YouTubeClientSecret) == "" || c.YouTubeClientSecret == "your-google-oauth-client-secret" {
		missing = append(missing, "YOUTUBE_CLIENT_SECRET")
	}
	return missing
}

// envOr returns the environment value, or fallback when empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

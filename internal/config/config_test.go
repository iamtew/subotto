package config

import (
	"testing"
)

func TestAdminPasswordEmptyDisablesDefault(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("ADMIN_PORT", "50770")
	t.Setenv("DATABASE_PATH", "./data/subotto.db")
	t.Setenv("LOG_LEVEL", "info")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminPassword != "" {
		t.Fatalf("expected empty ADMIN_PASSWORD, got %q", cfg.AdminPassword)
	}
}

func TestLoadReadsDiscordOAuth(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "x")
	t.Setenv("ADMIN_PORT", "50770")
	t.Setenv("DATABASE_PATH", "./data/subotto.db")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("SUPERADMIN_DISCORD_ID", "42")
	t.Setenv("DISCORD_CLIENT_ID", "cid")
	t.Setenv("DISCORD_CLIENT_SECRET", "sec")
	t.Setenv("DISCORD_OAUTH_REDIRECT_URL", "http://localhost:50770/auth/discord/callback")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SuperadminDiscordID != "42" || cfg.DiscordClientID != "cid" || cfg.DiscordClientSecret != "sec" {
		t.Fatalf("oauth fields: %+v", cfg)
	}
	if cfg.DiscordOAuthRedirectURL != "http://localhost:50770/auth/discord/callback" {
		t.Fatalf("redirect %q", cfg.DiscordOAuthRedirectURL)
	}
}

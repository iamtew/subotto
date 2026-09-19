// Package web serves Subotto's Admin UI, JSON API, and public OBS slideshow /
// Streamer.bot listener GETs.
//
// Meat Bag: public landing at http://localhost:50770 — Discord OAuth and/or Basic fallback.
// Admin UI at /admin (session cookie or admin / ADMIN_PASSWORD).
// Broadcast fire also accepts api / API_PASSWORD on GET /api/broadcasts/{slug}/fire.
// Public (no login): /, /slideshow/..., /api/slideshow/..., /api/get/{content|picture}/{channel},
// /api/get/episode/{show}, /stream-background, /media/stream-background/latest,
// GET /oauth/callback (Google), GET /auth/discord/callback (Discord login).
package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/ai"
	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/scheduler"
	"subotto/internal/youtube"
)

// StatusProvider tells the Admin UI whether Discord is connected,
// and lets the operator force a gateway reconnect.
// *discord.Bot already matches this.
type StatusProvider interface {
	Connected() bool
	Reconnect() error
}

// DiscordCatalog lists servers/channels the bot can see (Admin dropdowns)
// and posts/reads Chat-tab messages as the bot.
// *discord.Bot matches this.
type DiscordCatalog interface {
	ListGuilds() ([]discord.GuildInfo, error)
	ListTextChannels(guildID string) ([]discord.ChannelInfo, error)
	ListRecentMessages(channelID string, limit int) ([]discord.ChatMessage, error)
	SendChannelMessage(channelID, content string, files []discord.ChatFile) (*discord.ChatMessage, error)
}

// SchedulerStatus reports background resync state for /api/status.
// *scheduler.Scheduler matches this; nil means "not wired / disabled view".
type SchedulerStatus interface {
	Info() scheduler.Info
	Reload()
}

// Server is the integrated Admin HTTP server (static files + /api/...).
type Server struct {
	store       *db.DB
	yt          *youtube.Client
	status      StatusProvider
	scheduler   SchedulerStatus
	discord     DiscordCatalog
	discordTok  string
	password    string
	apiPassword string
	webroot     string
	addr        string
	httpServer  *http.Server
	startedAt   time.Time
	youtubeName string // optional; filled by Ping at boot / after Admin OAuth
	envHours    int    // RESYNC_INTERVAL_HOURS until Admin saves scheduler row
	envAIModel  string // OPENROUTER_MODEL until Admin saves ai_models

	ytClientID     string
	ytClientSecret string
	ytRedirectURL  string

	discordClientID     string
	discordClientSecret string
	discordRedirectURL  string
	superadminID        string

	oauthMu      sync.Mutex
	oauthPending struct {
		state string
		until time.Time
	}
	discordOAuthPending struct {
		state string
		until time.Time
	}

	// exchangeFn / youtubePing let tests stub Google (no live OAuth).
	exchangeFn  func(ctx context.Context, code string) (*oauth2.Token, error)
	youtubePing func(ctx context.Context) (string, error)

	// discordExchangeFn / discordMeFn let tests stub Discord login (no live OAuth).
	discordExchangeFn func(ctx context.Context, code string) (*oauth2.Token, error)
	discordMeFn       func(ctx context.Context, accessToken string) (string, error)

	// createPlaylistFn lets tests stub YouTube playlist create (episode start).
	createPlaylistFn func(ctx context.Context, title, description string) (string, error)
	// announceFn lets tests stub Discord channel posts (broadcast fire).
	announceFn func(ctx context.Context, token, channelID, content string) error

	ai *ai.Client
}

// Options configures the Admin server.
type Options struct {
	Store                   *db.DB
	YouTube                 *youtube.Client
	Status                  StatusProvider
	Scheduler               SchedulerStatus
	Discord                 DiscordCatalog
	DiscordToken            string
	AdminPassword           string
	APIPassword             string // optional; user "api" for broadcast fire
	AdminHost               string
	AdminPort               int
	Webroot                 string // folder with index.html / css / js; default ./webroot
	YouTubeChannel          string // display name from Ping, may be empty
	ResyncIntervalHours     int    // .env bootstrap for GET until Admin saves
	OpenRouterModel         string // .env bootstrap until Admin saves ai_models
	YouTubeClientID         string
	YouTubeClientSecret     string
	YouTubeRedirectURL      string
	DiscordClientID         string
	DiscordClientSecret     string
	DiscordOAuthRedirectURL string
	SuperadminDiscordID     string
	AI                      *ai.Client
}

// New builds an Admin server (does not listen yet — call Start).
func New(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("web server needs a database")
	}
	webroot := opts.Webroot
	if webroot == "" {
		webroot = "webroot"
	}
	abs, err := filepath.Abs(webroot)
	if err != nil {
		return nil, fmt.Errorf("resolve webroot: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("webroot folder missing at %s — keep webroot/ next to the binary", abs)
	}
	password := opts.AdminPassword
	host := opts.AdminHost
	if host == "" {
		host = "0.0.0.0"
	}
	port := opts.AdminPort
	if port == 0 {
		port = 50770
	}

	s := &Server{
		store:               opts.Store,
		yt:                  opts.YouTube,
		status:              opts.Status,
		scheduler:           opts.Scheduler,
		discord:             opts.Discord,
		discordTok:          opts.DiscordToken,
		password:            password,
		apiPassword:         opts.APIPassword,
		webroot:             abs,
		addr:                net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		startedAt:           time.Now().UTC(),
		youtubeName:         opts.YouTubeChannel,
		envHours:            opts.ResyncIntervalHours,
		envAIModel:          strings.TrimSpace(opts.OpenRouterModel),
		ytClientID:          opts.YouTubeClientID,
		ytClientSecret:      opts.YouTubeClientSecret,
		ytRedirectURL:       opts.YouTubeRedirectURL,
		discordClientID:     strings.TrimSpace(opts.DiscordClientID),
		discordClientSecret: strings.TrimSpace(opts.DiscordClientSecret),
		discordRedirectURL:  strings.TrimSpace(opts.DiscordOAuthRedirectURL),
		superadminID:        strings.TrimSpace(opts.SuperadminDiscordID),
		ai:                  opts.AI,
	}

	mux := http.NewServeMux()
	s.registerPublic(mux)
	s.registerAPI(mux)
	s.registerPictureAPI(mux)
	s.registerEpisodeAPI(mux)
	s.registerBroadcastAPI(mux)
	s.registerAIAPI(mux)
	fileServer := http.FileServer(http.Dir(s.webroot))
	mux.Handle("GET /css/", s.requireAdmin(fileServer))
	mux.Handle("GET /js/", s.requireAdmin(fileServer))
	mux.Handle("GET /admin", s.requireAdmin(http.HandlerFunc(s.handleAdminPage)))
	mux.Handle("GET /admin/{$}", s.requireAdmin(http.HandlerFunc(s.handleAdminPage)))
	mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})

	s.httpServer = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.webroot, "index.html"))
}

// Addr returns host:port the server listens on.
func (s *Server) Addr() string {
	return s.addr
}

// Start begins listening. Blocks until the server stops; run it in a goroutine.
func (s *Server) Start() error {
	slog.Info("HTTP listening", "addr", s.addr, "webroot", s.webroot, "admin", "/admin", "public", "/")
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown stops the HTTP server cleanly (Ctrl+C path).
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

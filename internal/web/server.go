// Package web serves Subotto's Admin UI, JSON API, and public OBS slideshow /
// Streamer.bot listener GETs.
//
// Meat Bag: Admin is http://localhost:50770 (Basic Auth: admin / ADMIN_PASSWORD).
// Public (no password): /slideshow/..., /api/slideshow/..., /api/get/{content|picture}/{channel},
// /api/get/episode/{show}.
package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	webroot     string
	addr        string
	httpServer  *http.Server
	startedAt   time.Time
	youtubeName string // optional; filled by Ping at boot if available

	// createPlaylistFn lets tests stub YouTube playlist create (episode start).
	createPlaylistFn func(ctx context.Context, title, description string) (string, error)
}

// Options configures the Admin server.
type Options struct {
	Store          *db.DB
	YouTube        *youtube.Client
	Status         StatusProvider
	Scheduler      SchedulerStatus
	Discord        DiscordCatalog
	DiscordToken   string
	AdminPassword  string
	AdminHost      string
	AdminPort      int
	Webroot        string // folder with index.html / css / js; default ./webroot
	YouTubeChannel string // display name from Ping, may be empty
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
	if password == "" {
		password = "change-me-please"
	}
	host := opts.AdminHost
	if host == "" {
		host = "0.0.0.0"
	}
	port := opts.AdminPort
	if port == 0 {
		port = 50770
	}

	s := &Server{
		store:       opts.Store,
		yt:          opts.YouTube,
		status:      opts.Status,
		scheduler:   opts.Scheduler,
		discord:     opts.Discord,
		discordTok:  opts.DiscordToken,
		password:    password,
		webroot:     abs,
		addr:        net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		startedAt:   time.Now().UTC(),
		youtubeName: opts.YouTubeChannel,
	}

	mux := http.NewServeMux()
	s.registerPublic(mux) // OBS slideshow — no Basic Auth
	s.registerAPI(mux)
	s.registerPictureAPI(mux)
	s.registerEpisodeAPI(mux)
	// Static Admin files last — "/" catches everything else under webroot (auth’d).
	fileServer := http.FileServer(http.Dir(s.webroot))
	mux.Handle("/", s.basicAuth(fileServer))

	s.httpServer = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// Addr returns host:port the server listens on.
func (s *Server) Addr() string {
	return s.addr
}

// Start begins listening. Blocks until the server stops; run it in a goroutine.
func (s *Server) Start() error {
	slog.Info("HTTP listening", "addr", s.addr, "webroot", s.webroot, "public", "/slideshow/...")
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

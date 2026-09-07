// Package web serves Subotto's Admin UI and JSON API.
//
// Meat Bag: when Subotto is running, open http://localhost:8080 (or whatever
// ADMIN_HOST / ADMIN_PORT you set). The browser will ask for a username and
// password — use username "admin" and the ADMIN_PASSWORD from your .env.
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
	"subotto/internal/youtube"
)

// StatusProvider tells the Admin UI whether Discord is connected.
// *discord.Bot already matches this.
type StatusProvider interface {
	Connected() bool
}

// Server is the integrated Admin HTTP server (static files + /api/...).
type Server struct {
	store       *db.DB
	yt          *youtube.Client
	status      StatusProvider
	discordTok  string
	password    string
	webroot     string
	addr        string
	httpServer  *http.Server
	startedAt   time.Time
	youtubeName string // optional; filled by Ping at boot if available
}

// Options configures the Admin server.
type Options struct {
	Store          *db.DB
	YouTube        *youtube.Client
	Status         StatusProvider
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
		port = 8080
	}

	s := &Server{
		store:       opts.Store,
		yt:          opts.YouTube,
		status:      opts.Status,
		discordTok:  opts.DiscordToken,
		password:    password,
		webroot:     abs,
		addr:        net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		startedAt:   time.Now().UTC(),
		youtubeName: opts.YouTubeChannel,
	}

	mux := http.NewServeMux()
	s.registerAPI(mux)
	// Static files last — "/" catches everything else under webroot.
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
	slog.Info("admin UI listening", "addr", s.addr, "webroot", s.webroot)
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

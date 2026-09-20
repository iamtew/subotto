package twitch

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"

	"subotto/internal/ai"
	"subotto/internal/db"
)

const idleWait = 5 * time.Second

const maxBackoff = 2 * time.Minute

// Client keeps Twitch IRC up and runs Hesh Helper on addressed chat.
type Client struct {
	store      *db.DB
	ai         *ai.Client
	envAIModel string
	clientID   string
	secret     string
	redirect   string

	srcMu sync.Mutex
	src   oauth2.TokenSource

	wake chan struct{}

	connMu sync.Mutex
	conn   net.Conn

	ircUp atomic.Bool

	memMu sync.Mutex
	mem   []memLine
}

type memLine struct {
	login    string
	text     string
	fromSelf bool
}

// Options wires SQLite, OpenRouter, and the Twitch app credentials.
type Options struct {
	Store              *db.DB
	AI                 *ai.Client
	OpenRouterModel    string
	TwitchClientID     string
	TwitchClientSecret string
	TwitchRedirectURL  string
}

// New builds a client. Empty app credentials = Configured() false (IRC stays idle).
func New(opts Options) *Client {
	return &Client{
		store:      opts.Store,
		ai:         opts.AI,
		envAIModel: strings.TrimSpace(opts.OpenRouterModel),
		clientID:   strings.TrimSpace(opts.TwitchClientID),
		secret:     strings.TrimSpace(opts.TwitchClientSecret),
		redirect:   strings.TrimSpace(opts.TwitchRedirectURL),
		wake:       make(chan struct{}, 1),
	}
}

// Configured is true when the Twitch app id/secret are set in .env.
func (c *Client) Configured() bool {
	return c != nil && c.clientID != "" && c.secret != ""
}

// Connected is true while an IRC session is up.
func (c *Client) Connected() bool {
	return c != nil && c.ircUp.Load()
}

// Kick closes IRC (if any) and wakes Maintain so it re-reads token/channel.
func (c *Client) Kick() {
	if c == nil {
		return
	}
	c.closeConn()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// ReloadToken rebuilds the token source from SQLite (after Admin OAuth).
func (c *Client) ReloadToken(ctx context.Context) error {
	if c == nil || c.store == nil {
		return errors.New("twitch client is nil")
	}
	if !c.Configured() {
		return errors.New("twitch oauth is not configured (client id/secret)")
	}
	tok, err := LoadToken(ctx, c.store)
	if err != nil {
		return err
	}
	cfg := OAuthConfig(c.clientID, c.secret, c.redirect)
	src := newSavingSource(c.store, cfg, tok)
	c.srcMu.Lock()
	c.src = src
	c.srcMu.Unlock()
	c.Kick()
	return nil
}

func (c *Client) tokenSource() oauth2.TokenSource {
	c.srcMu.Lock()
	defer c.srcMu.Unlock()
	return c.src
}

func (c *Client) accessToken() (string, error) {
	src := c.tokenSource()
	if src == nil {
		return "", ErrNotAuthorized
	}
	tok, err := src.Token()
	if err != nil {
		return "", err
	}
	if tok == nil || strings.TrimSpace(tok.AccessToken) == "" {
		return "", ErrNotAuthorized
	}
	return tok.AccessToken, nil
}

// Maintain stays on IRC until ctx is canceled. Safe to run with no token yet.
func (c *Client) Maintain(ctx context.Context) {
	if c == nil {
		return
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if !c.Configured() || c.store == nil {
			waitIdle(ctx, c.wake, idleWait)
			continue
		}
		if c.tokenSource() == nil {
			if err := c.ReloadToken(ctx); err != nil {
				waitIdle(ctx, c.wake, idleWait)
				continue
			}
		}
		channel, err := c.store.TwitchChannel(ctx)
		if err != nil {
			slog.Error("twitch channel load failed", "err", err)
			waitIdle(ctx, c.wake, idleWait)
			continue
		}
		channel = NormalizeChannel(channel)
		if channel == "" {
			waitIdle(ctx, c.wake, idleWait)
			continue
		}
		err = c.runSession(ctx, channel)
		if ctx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Warn("twitch irc disconnected", "err", err)
			waitIdle(ctx, c.wake, backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = time.Second
		waitIdle(ctx, c.wake, time.Second)
	}
}

func (c *Client) runSession(ctx context.Context, channel string) error {
	access, err := c.accessToken()
	if err != nil {
		return err
	}
	login, err := c.store.TwitchLogin(ctx)
	if err != nil {
		return err
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		u, ferr := FetchUser(ctx, c.clientID, access)
		if ferr != nil {
			return ferr
		}
		login = u.Login
		if err := c.store.SetTwitchIdentity(ctx, u.Login, u.DisplayName); err != nil {
			slog.Warn("twitch identity save failed", "err", err)
		}
	}

	d := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", ircHost, &tls.Config{ServerName: "irc.chat.twitch.tv", MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("twitch irc dial: %w", err)
	}
	c.setConn(conn)
	c.ircUp.Store(true)
	done := make(chan struct{})
	defer func() {
		close(done)
		c.ircUp.Store(false)
		c.closeConn()
	}()
	go func() {
		select {
		case <-ctx.Done():
			c.closeConn()
		case <-done:
		}
	}()

	if err := c.ircWrite(fmt.Sprintf("PASS oauth:%s\r\nNICK %s\r\nCAP REQ :twitch.tv/tags twitch.tv/commands\r\nJOIN #%s\r\n", access, login, channel)); err != nil {
		return err
	}
	slog.Info("twitch irc joined", "as", login, "channel", channel)

	br := bufio.NewReader(conn)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		msg, ok := parseIRCLine(line)
		if !ok {
			continue
		}
		switch msg.Command {
		case "PING":
			payload := "tmi.twitch.tv"
			if len(msg.Params) > 0 {
				payload = msg.Params[0]
			}
			_ = c.ircWrite("PONG :" + payload + "\r\n")
		case "RECONNECT":
			return errors.New("twitch requested reconnect")
		case "PRIVMSG":
			c.handlePRIVMSG(msg, login, channel)
		}
	}
}

func (c *Client) setConn(conn net.Conn) {
	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()
}

func (c *Client) closeConn() {
	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *Client) ircWrite(s string) error {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()
	if conn == nil {
		return net.ErrClosed
	}
	_, err := conn.Write([]byte(s))
	return err
}

func (c *Client) sendPRIVMSG(channel, text, parentID string) error {
	return c.ircWrite(privmsgOut(channel, text, parentID) + "\r\n")
}

// Status is what Admin /api/status shows for Twitch.
type Status struct {
	Configured bool   `json:"configured"`
	Authorized bool   `json:"authorized"`
	Connected  bool   `json:"connected"`
	Login      string `json:"login"`
	Display    string `json:"display"`
	Channel    string `json:"channel"`
}

func (c *Client) Status(ctx context.Context) Status {
	out := Status{}
	if c == nil {
		return out
	}
	out.Configured = c.Configured()
	out.Connected = c.Connected()
	if c.store == nil {
		return out
	}
	has, err := HasStoredToken(ctx, c.store)
	out.Authorized = err == nil && has
	out.Login, _ = c.store.TwitchLogin(ctx)
	out.Display, _ = c.store.TwitchDisplay(ctx)
	out.Channel, _ = c.store.TwitchChannel(ctx)
	out.Channel = NormalizeChannel(out.Channel)
	return out
}

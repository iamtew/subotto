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

	chatMu  sync.Mutex
	chatLog map[string][]ChatMessage
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
		channels, err := c.store.TwitchChannels(ctx)
		if err != nil {
			slog.Error("twitch channel load failed", "err", err)
			waitIdle(ctx, c.wake, idleWait)
			continue
		}
		var cleaned []string
		for _, ch := range channels {
			if n := NormalizeChannel(ch); n != "" {
				cleaned = append(cleaned, n)
			}
		}
		if len(cleaned) == 0 {
			waitIdle(ctx, c.wake, idleWait)
			continue
		}
		err = c.runSession(ctx, cleaned)
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

func (c *Client) runSession(ctx context.Context, channels []string) error {
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

	join := joinLine(channels)
	if join == "" {
		return nil
	}
	if err := c.ircWrite(fmt.Sprintf("PASS oauth:%s\r\nNICK %s\r\nCAP REQ :twitch.tv/tags twitch.tv/commands\r\n%s", access, login, join)); err != nil {
		return err
	}
	slog.Info("twitch irc joined", "as", login, "channels", channels)

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
			c.handlePRIVMSG(msg, login)
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

const chatKeep = 10

// ChatMessage is one Twitch IRC line for the Admin Chat tab (same JSON as Discord).
type ChatMessage struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	AuthorID  string `json:"author_id,omitempty"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Bot       bool   `json:"bot"`
	Self      bool   `json:"self"`
}

func (c *Client) rememberChat(channel string, m ChatMessage) {
	if c == nil {
		return
	}
	channel = NormalizeChannel(channel)
	if channel == "" {
		return
	}
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatLog == nil {
		c.chatLog = map[string][]ChatMessage{}
	}
	list := append(c.chatLog[channel], m)
	if len(list) > chatKeep {
		list = list[len(list)-chatKeep:]
	}
	c.chatLog[channel] = list
}

func (c *Client) dropChat(channel string) {
	if c == nil {
		return
	}
	c.chatMu.Lock()
	delete(c.chatLog, NormalizeChannel(channel))
	c.chatMu.Unlock()
}

// ListRecentMessages is the last PRIVMSG lines seen in a joined room (since process start / join).
func (c *Client) ListRecentMessages(channel string, limit int) []ChatMessage {
	if c == nil {
		return []ChatMessage{}
	}
	channel = NormalizeChannel(channel)
	if limit <= 0 || limit > chatKeep {
		limit = chatKeep
	}
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	list := c.chatLog[channel]
	if len(list) == 0 {
		return []ChatMessage{}
	}
	if len(list) > limit {
		list = list[len(list)-limit:]
	}
	out := make([]ChatMessage, len(list))
	copy(out, list)
	return out
}

// SendChat posts as the authorized Twitch user. Channel must already be joined.
func (c *Client) SendChat(channel, text string) error {
	if c == nil {
		return errors.New("twitch client is nil")
	}
	channel = NormalizeChannel(channel)
	text = strings.TrimSpace(text)
	if channel == "" || text == "" {
		return errors.New("channel and message are required")
	}
	if err := c.sendPRIVMSG(channel, text, ""); err != nil {
		return err
	}
	c.rememberChat(channel, ChatMessage{
		Author:    c.selfDisplay(),
		AuthorID:  c.selfLogin(),
		Content:   text,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Self:      true,
	})
	return nil
}

func (c *Client) selfDisplay() string {
	if c.store == nil {
		return c.selfLogin()
	}
	display, _ := c.store.TwitchDisplay(context.Background())
	if display == "" {
		return c.selfLogin()
	}
	return display
}

// AfterChannelsChanged JOINs/PARTs on a live IRC session, or Kick()s if idle/empty.
func (c *Client) AfterChannelsChanged(added, removed string, remaining int) {
	if c == nil {
		return
	}
	if remaining == 0 {
		c.Kick()
		return
	}
	if !c.Connected() {
		c.Kick()
		return
	}
	if added != "" {
		added = NormalizeChannel(added)
		if added != "" {
			_ = c.ircWrite("JOIN #" + added + "\r\n")
		}
	}
	if removed != "" {
		removed = NormalizeChannel(removed)
		if removed != "" {
			_ = c.ircWrite("PART #" + removed + "\r\n")
			c.dropChat(removed)
		}
	}
}

func ChannelJoined(list []string, channel string) bool {
	channel = NormalizeChannel(channel)
	if channel == "" {
		return false
	}
	for _, x := range list {
		if NormalizeChannel(x) == channel {
			return true
		}
	}
	return false
}

// Status is what Admin /api/status shows for Twitch.
type Status struct {
	Configured bool     `json:"configured"`
	Authorized bool     `json:"authorized"`
	Connected  bool     `json:"connected"`
	Login      string   `json:"login"`
	Display    string   `json:"display"`
	Channel    string   `json:"channel"`
	Channels   []string `json:"channels"`
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
	out.Channels, _ = c.store.TwitchChannels(ctx)
	cleaned := make([]string, 0, len(out.Channels))
	for _, ch := range out.Channels {
		if n := NormalizeChannel(ch); n != "" {
			cleaned = append(cleaned, n)
		}
	}
	out.Channels = cleaned
	if len(out.Channels) > 0 {
		out.Channel = out.Channels[0]
	}
	return out
}

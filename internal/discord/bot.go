// Package discord runs the Subotto Discord bot.
//
// Meat Bag: Subotto can run two listener types on the same channel at once:
//   - Content listener — YouTube links → playlist
//   - Picture listener — image attachments → data/pictures/{slug}/ + public slideshow
package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
	"subotto/internal/ingest"
	"subotto/internal/pictures"
	"subotto/internal/youtube"
)

// How long Discord can stay "down" before Maintain forces a Close+Open.
// discordgo already retries on its own; this covers stalled reconnects.
const discordWatchdogGrace = 2 * time.Minute

// How often Maintain checks the ready flag.
const discordWatchdogTick = 30 * time.Second

// Minimum gap between forced Reconnect attempts (watchdog or overlapping calls).
const discordReconnectCooldown = 2 * time.Minute

// Bot listens for Discord messages and forwards content to the right listeners.
type Bot struct {
	session *discordgo.Session
	store   *db.DB
	yt      *youtube.Client
	guildID string // optional filter; empty = all guilds the bot is in
	ready   atomic.Bool

	// reconnectMu serializes forced Close+Open (Admin button + watchdog).
	reconnectMu sync.Mutex
	// reconnecting is set while Reconnect/Open-with-retry is in flight (UI can poll Connected).
	reconnecting atomic.Bool
	// lastDownUnix is when we last marked Discord down (disconnect or failed open). Unix nanos.
	lastDownUnix atomic.Int64
	// lastForcedUnix is when we last ran a forced Reconnect. Unix nanos.
	lastForcedUnix atomic.Int64
}

// New creates a Discord session with the intents Subotto needs.
// Message Content Intent must be enabled in the Discord Developer Portal.
func New(token string, store *db.DB, yt *youtube.Client, guildID string) (*Bot, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is empty")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}

	// discordgo already reconnects forever on gateway errors; keep that on.
	session.ShouldReconnectOnError = true

	// Guilds + messages + message content (YouTube links) + reactions (slideshow credits).
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent |
		discordgo.IntentsGuildMessageReactions

	b := &Bot{
		session: session,
		store:   store,
		yt:      yt,
		guildID: strings.TrimSpace(guildID),
	}
	// Start "down" so Maintain's grace clock begins if Open never succeeds.
	b.markDown("boot")

	session.AddHandler(b.onMessageCreate)
	session.AddHandler(b.onMessageReactionAdd)
	session.AddHandler(b.onMessageReactionRemove)
	session.AddHandler(b.onMessageReactionRemoveAll)

	// Ready fires on a fresh Identify. After a drop, discordgo often Resumes instead —
	// that sends Resumed + Connect, not Ready. We must treat all three as "up".
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		b.markUp()
		user := ""
		if r != nil && r.User != nil {
			user = r.User.Username
		}
		guilds := 0
		if r != nil {
			guilds = len(r.Guilds)
		}
		slog.Info("discord connected", "user", user, "guilds", guilds, "via", "ready")
	})
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		b.markUp()
		slog.Info("discord connected", "via", "resumed")
	})
	session.AddHandler(func(s *discordgo.Session, c *discordgo.Connect) {
		b.markUp()
		slog.Info("discord connected", "via", "connect")
	})
	session.AddHandler(func(s *discordgo.Session, d *discordgo.Disconnect) {
		b.markDown("disconnect")
		slog.Warn("discord disconnected")
	})
	return b, nil
}

// markUp records that the gateway is usable again (Admin shows "discord up").
func (b *Bot) markUp() {
	b.ready.Store(true)
}

// markDown records that the gateway is down and when (for the watchdog grace period).
func (b *Bot) markDown(reason string) {
	b.ready.Store(false)
	b.lastDownUnix.Store(time.Now().UnixNano())
	if reason != "" {
		slog.Debug("discord marked down", "reason", reason)
	}
}

// Connected reports whether Discord is up (Admin UI status).
// True after Ready, Resumed, or Connect; false after Disconnect / failed open.
func (b *Bot) Connected() bool {
	if b == nil {
		return false
	}
	return b.ready.Load()
}

// Open connects to the Discord gateway (one attempt).
func (b *Bot) Open() error {
	if err := b.session.Open(); err != nil {
		b.markDown("open_failed")
		return fmt.Errorf("open discord gateway: %w", err)
	}
	return nil
}

// Close disconnects cleanly.
func (b *Bot) Close() error {
	if b == nil || b.session == nil {
		return nil
	}
	return b.session.Close()
}

// Reconnect forces a gateway Close then Open.
// Used by the Admin "discord down" button and by Maintain's watchdog.
func (b *Bot) Reconnect() error {
	if b == nil || b.session == nil {
		return fmt.Errorf("discord bot not initialized")
	}
	if !b.reconnecting.CompareAndSwap(false, true) {
		return fmt.Errorf("discord reconnect already in progress")
	}
	defer b.reconnecting.Store(false)

	b.reconnectMu.Lock()
	defer b.reconnectMu.Unlock()

	b.lastForcedUnix.Store(time.Now().UnixNano())
	slog.Info("discord reconnect requested")

	// Close may fail if already closed — still try Open.
	if err := b.session.Close(); err != nil {
		slog.Debug("discord close before reconnect", "err", err)
	}
	// Brief settle so discordgo's listen/heartbeat goroutines finish.
	time.Sleep(500 * time.Millisecond)

	if err := b.session.Open(); err != nil {
		b.markDown("reconnect_open_failed")
		return fmt.Errorf("discord reconnect open: %w", err)
	}
	slog.Info("discord reconnect open succeeded — waiting for ready/resume")
	return nil
}

// Maintain keeps Discord online for the life of ctx:
//  1. Retry Open with backoff until connected (or ctx done) — boot must not kill Subotto.
//  2. Watchdog: if still down after discordWatchdogGrace, force Reconnect (with cooldown).
//
// discordgo also auto-reconnects; Maintain is the safety net + Admin Reconnect path.
func (b *Bot) Maintain(ctx context.Context) {
	if b == nil {
		return
	}
	slog.Info("discord maintain loop started")
	b.connectWithBackoff(ctx)
	if ctx.Err() != nil {
		slog.Info("discord maintain loop stopped before watchdog")
		return
	}
	b.watchdog(ctx)
	slog.Info("discord maintain loop stopped")
}

func (b *Bot) connectWithBackoff(ctx context.Context) {
	wait := time.Second
	const maxWait = 60 * time.Second
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if b.Connected() {
			return
		}
		attempt++
		slog.Info("discord connecting", "attempt", attempt)
		err := b.Open()
		if err == nil || errors.Is(err, discordgo.ErrWSAlreadyOpen) {
			// Open succeeded (or socket already open); Ready/Resumed/Connect set ready.
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			if b.Connected() {
				slog.Info("discord maintain: initial connection up")
				return
			}
			slog.Info("discord open returned; waiting for ready/resume event")
			return
		}
		slog.Error("discord connect failed; retrying", "attempt", attempt, "err", err, "wait", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
}

func (b *Bot) watchdog(ctx context.Context) {
	ticker := time.NewTicker(discordWatchdogTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if b.Connected() {
				continue
			}
			downAt := time.Unix(0, b.lastDownUnix.Load())
			if downAt.IsZero() || time.Since(downAt) < discordWatchdogGrace {
				continue
			}
			lastForced := time.Unix(0, b.lastForcedUnix.Load())
			if !lastForced.IsZero() && time.Since(lastForced) < discordReconnectCooldown {
				continue
			}
			if b.reconnecting.Load() {
				continue
			}
			slog.Warn("discord still down after grace; forcing reconnect",
				"down_for", time.Since(downAt).Round(time.Second).String())
			if err := b.Reconnect(); err != nil {
				slog.Error("discord watchdog reconnect failed", "err", err)
			}
		}
	}
}

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore ourselves and other bots.
	if m.Author == nil || m.Author.Bot {
		return
	}
	if b.guildID != "" && m.GuildID != b.guildID {
		return
	}

	ctx := context.Background()
	b.handleContentListener(ctx, s, m)
	b.handlePictureListener(ctx, s, m)
}

func (b *Bot) handleContentListener(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	mapping, err := b.store.GetEnabledMappingByChannel(ctx, m.ChannelID)
	if err != nil {
		slog.Error("lookup content listener failed", "channel", m.ChannelID, "err", err)
		return
	}
	if mapping == nil {
		return
	}

	res := ingest.ProcessContent(ctx, b.store, b.yt, mapping, m.ChannelID, m.ID, m.Content)
	ensureStatusChrome(ctx, s, m.ChannelID, m.ID, m.Message, contentStatusChrome(res))
}

func (b *Bot) handlePictureListener(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	pl, err := b.store.GetEnabledPictureListenerByChannel(ctx, m.ChannelID)
	if err != nil {
		slog.Error("lookup picture listener failed", "channel", m.ChannelID, "err", err)
		return
	}
	if pl == nil || len(m.Attachments) == 0 {
		return
	}

	atts := make([]pictures.Attachment, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		atts = append(atts, pictures.Attachment{
			ID:          a.ID,
			URL:         a.URL,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Width:       a.Width,
			Height:      a.Height,
		})
	}

	reactions := reactionsFromMessage(m.Message)
	res := pictures.ProcessAttachments(
		ctx, b.store, pl, m.ID, m.Author.ID, displayNameFromMessage(m.Message), atts, reactions,
	)
	ensureStatusChrome(ctx, s, m.ChannelID, m.ID, m.Message, pictureStatusChrome(res))
}

func (b *Bot) onMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return // ignore our own reacts
	}
	b.refreshMessageReactions(r.ChannelID, r.MessageID)
}

func (b *Bot) onMessageReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	if s.State.User != nil && r.UserID == s.State.User.ID {
		return // ignore our own removes (status chrome)
	}
	b.refreshMessageReactions(r.ChannelID, r.MessageID)
}

func (b *Bot) onMessageReactionRemoveAll(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
	b.refreshMessageReactions(r.ChannelID, r.MessageID)
}

func (b *Bot) refreshMessageReactions(channelID, messageID string) {
	ctx := context.Background()
	pl, err := b.store.GetEnabledPictureListenerByChannel(ctx, channelID)
	if err != nil || pl == nil {
		return
	}
	pics, err := b.store.ListCollectedPicturesByMessage(ctx, channelID, messageID)
	if err != nil || len(pics) == 0 {
		return
	}

	msg, err := b.session.ChannelMessage(channelID, messageID)
	if err != nil {
		slog.Warn("fetch message for reactions failed", "channel", channelID, "message", messageID, "err", err)
		return
	}
	reactions := reactionsFromMessage(msg)
	if err := b.store.UpdatePictureReactions(ctx, channelID, messageID, reactions); err != nil {
		slog.Error("update picture reactions failed", "err", err)
	}
}

// reactionsFromMessage builds an ordered emoji→count list for the slideshow.
// Order matches Discord's reaction bar (first reaction added → leftmost).
// Counts exclude Subotto's own reacts (Discord's Me flag) so status chrome
// like 💾 / DUPE / OLD / ❌ never become floaters — only other users' reacts.
func reactionsFromMessage(m *discordgo.Message) []db.ReactionCount {
	if m == nil {
		return []db.ReactionCount{}
	}
	out := make([]db.ReactionCount, 0, len(m.Reactions))
	for _, r := range m.Reactions {
		if r == nil || r.Emoji == nil || r.Emoji.Name == "" {
			continue
		}
		count := r.Count
		if r.Me {
			count--
		}
		if count <= 0 {
			continue
		}
		key := r.Emoji.Name
		if r.Emoji.ID != "" {
			// Custom emoji for slideshow CDN: name:id or a:name:id (animated).
			if r.Emoji.Animated {
				key = "a:" + r.Emoji.Name + ":" + r.Emoji.ID
			} else {
				key = r.Emoji.Name + ":" + r.Emoji.ID
			}
		}
		out = append(out, db.ReactionCount{Emoji: key, Count: count})
	}
	return out
}


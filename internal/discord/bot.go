// Package discord runs the Subotto Discord bot.
//
// Meat Bag: when someone posts a YouTube link in a *mapped* channel, Subotto
// grabs the video ID and adds it to that channel's YouTube playlist.
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
	"subotto/internal/ingest"
	"subotto/internal/youtube"
)

// Bot listens for Discord messages and forwards YouTube links to playlists.
type Bot struct {
	session *discordgo.Session
	store   *db.DB
	yt      *youtube.Client
	guildID string // optional filter; empty = all guilds the bot is in
	ready   atomic.Bool
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

	// Guilds: basic server info. GuildMessages: message events.
	// MessageContent: actual message text (required to see YouTube links).
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	b := &Bot{
		session: session,
		store:   store,
		yt:      yt,
		guildID: strings.TrimSpace(guildID),
	}
	session.AddHandler(b.onMessageCreate)
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		b.ready.Store(true)
		slog.Info("discord connected", "user", r.User.Username, "guilds", len(r.Guilds))
	})
	session.AddHandler(func(s *discordgo.Session, d *discordgo.Disconnect) {
		b.ready.Store(false)
		slog.Warn("discord disconnected")
	})
	return b, nil
}

// Connected reports whether Discord has sent the Ready event (Admin UI status).
func (b *Bot) Connected() bool {
	if b == nil {
		return false
	}
	return b.ready.Load()
}

// Open connects to the Discord gateway.
func (b *Bot) Open() error {
	if err := b.session.Open(); err != nil {
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

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore ourselves and other bots.
	if m.Author == nil || m.Author.Bot {
		return
	}
	if b.guildID != "" && m.GuildID != b.guildID {
		return
	}

	ctx := context.Background()
	mapping, err := b.store.GetEnabledMappingByChannel(ctx, m.ChannelID)
	if err != nil {
		slog.Error("lookup channel mapping failed", "channel", m.ChannelID, "err", err)
		return
	}
	if mapping == nil {
		return // channel not mapped — stay quiet
	}

	res := ingest.ProcessContent(ctx, b.store, b.yt, mapping, m.ChannelID, m.ID, m.Content)
	if res.Added == 0 && res.Skipped == 0 && res.Failed == 0 {
		return // no YouTube links in this message
	}

	switch {
	case res.Failed > 0 && res.Added == 0:
		b.react(s, m, "❌")
	case res.Added > 0:
		b.react(s, m, "💾") // saved to playlist (floppy = classic "saved")
	case res.Skipped > 0:
		b.react(s, m, "♻️") // already had these videos
	}
}

func (b *Bot) react(s *discordgo.Session, m *discordgo.MessageCreate, emoji string) {
	// Meat Bag: if videos land on the playlist but you see no emoji, Discord
	// usually denied "Add Reactions". We log at Warn so it shows at default info level.
	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emoji); err != nil {
		slog.Warn("could not add reaction",
			"emoji", emoji,
			"channel", m.ChannelID,
			"message", m.ID,
			"err", err,
		)
	}
}

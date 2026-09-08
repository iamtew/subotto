// Package discord runs the Subotto Discord bot.
//
// Meat Bag: Subotto can run two listener types on the same channel at once:
//   - Content listener — YouTube links → playlist
//   - Picture listener — image attachments → data/pictures/{slug}/ + public slideshow
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
	"subotto/internal/pictures"
	"subotto/internal/youtube"
)

// Bot listens for Discord messages and forwards content to the right listeners.
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
	session.AddHandler(b.onMessageCreate)
	session.AddHandler(b.onMessageReactionAdd)
	session.AddHandler(b.onMessageReactionRemove)
	session.AddHandler(b.onMessageReactionRemoveAll)
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
	if res.Added == 0 && res.Skipped == 0 && res.Failed == 0 {
		return // no YouTube links in this message
	}

	switch {
	case res.Failed > 0 && res.Added == 0:
		b.react(s, m.ChannelID, m.ID, "❌")
	case res.Added > 0:
		b.react(s, m.ChannelID, m.ID, "💾") // saved to playlist
	case res.SkippedOld > 0:
		b.reactAll(s, m.ChannelID, m.ID, "🛑", "🇴", "🇱", "🇩")
	case res.SkippedSame > 0 || res.Skipped > 0:
		b.reactAll(s, m.ChannelID, m.ID, "♻️", "🇩", "🇺", "🇵", "🇪")
	}
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
	if res.Saved == 0 && res.Skipped == 0 && res.Failed == 0 {
		return // no image attachments
	}

	switch {
	case res.Failed > 0 && res.Saved == 0:
		b.react(s, m.ChannelID, m.ID, "❌")
	case res.Saved > 0:
		b.react(s, m.ChannelID, m.ID, "🖼️")
	case res.Skipped > 0:
		b.reactAll(s, m.ChannelID, m.ID, "♻️", "🇩", "🇺", "🇵", "🇪")
	}
}

func (b *Bot) onMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r.UserID == s.State.User.ID {
		return // ignore our own reacts
	}
	b.refreshMessageReactions(r.ChannelID, r.MessageID)
}

func (b *Bot) onMessageReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
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

func reactionsFromMessage(m *discordgo.Message) map[string]int {
	out := map[string]int{}
	if m == nil {
		return out
	}
	for _, r := range m.Reactions {
		if r == nil || r.Emoji.Name == "" {
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
		out[key] = r.Count
	}
	return out
}

func (b *Bot) react(s *discordgo.Session, channelID, messageID, emoji string) {
	b.reactAll(s, channelID, messageID, emoji)
}

// reactAll adds one or more reactions in order (lead emoji, then letter spells).
func (b *Bot) reactAll(s *discordgo.Session, channelID, messageID string, emojis ...string) {
	for _, emoji := range emojis {
		if err := s.MessageReactionAdd(channelID, messageID, emoji); err != nil {
			slog.Warn("could not add reaction",
				"emoji", emoji,
				"channel", channelID,
				"message", messageID,
				"err", err,
			)
			return
		}
	}
}

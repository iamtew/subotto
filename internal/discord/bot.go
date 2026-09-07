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

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
	"subotto/internal/parser"
	"subotto/internal/youtube"
)

// Bot listens for Discord messages and forwards YouTube links to playlists.
type Bot struct {
	session *discordgo.Session
	store   *db.DB
	yt      *youtube.Client
	guildID string // optional filter; empty = all guilds the bot is in
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
		slog.Info("discord connected", "user", r.User.Username, "guilds", len(r.Guilds))
	})
	return b, nil
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

	ids := parser.ExtractVideoIDs(m.Content)
	if len(ids) == 0 {
		return
	}

	if b.yt == nil {
		slog.Warn("youtube client missing; cannot add videos", "channel", m.ChannelID)
		b.react(s, m, "⚠️")
		return
	}

	added := 0
	skipped := 0
	failed := 0

	for _, videoID := range ids {
		already, err := b.store.WasVideoProcessed(ctx, videoID, mapping.YouTubePlaylistID)
		if err != nil {
			slog.Error("dedup check failed", "video", videoID, "err", err)
			failed++
			continue
		}
		if already {
			slog.Info("skip duplicate video", "video", videoID, "playlist", mapping.YouTubePlaylistID)
			skipped++
			_ = b.store.LogActivity(ctx, "video_skipped", map[string]any{
				"reason":     "duplicate",
				"video_id":   videoID,
				"playlist":   mapping.YouTubePlaylistID,
				"channel_id": m.ChannelID,
				"message_id": m.ID,
			}, true)
			continue
		}

		if err := b.yt.AddVideoToPlaylist(ctx, mapping.YouTubePlaylistID, videoID); err != nil {
			slog.Error("add video to playlist failed",
				"video", videoID,
				"playlist", mapping.YouTubePlaylistID,
				"err", err,
			)
			failed++
			_ = b.store.LogActivity(ctx, "video_add_failed", map[string]any{
				"video_id":   videoID,
				"playlist":   mapping.YouTubePlaylistID,
				"channel_id": m.ChannelID,
				"message_id": m.ID,
				"error":      err.Error(),
			}, false)
			continue
		}

		if err := b.store.MarkVideoProcessed(ctx, videoID, mapping.YouTubePlaylistID, m.ID); err != nil {
			slog.Error("mark processed failed", "video", videoID, "err", err)
		}
		_ = b.store.LogActivity(ctx, "video_added", map[string]any{
			"video_id":   videoID,
			"playlist":   mapping.YouTubePlaylistID,
			"channel_id": m.ChannelID,
			"message_id": m.ID,
			"mapping":    mapping.Name,
		}, true)
		slog.Info("added video to playlist",
			"video", videoID,
			"playlist", mapping.YouTubePlaylistID,
			"mapping", mapping.Name,
		)
		added++
	}

	switch {
	case failed > 0 && added == 0:
		b.react(s, m, "❌")
	case added > 0:
		b.react(s, m, "✅")
	case skipped > 0:
		b.react(s, m, "♻️") // already had these videos
	}
}

func (b *Bot) react(s *discordgo.Session, m *discordgo.MessageCreate, emoji string) {
	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emoji); err != nil {
		slog.Debug("could not add reaction", "emoji", emoji, "err", err)
	}
}

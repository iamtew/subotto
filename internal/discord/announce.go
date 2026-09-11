package discord

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
)

// DiscordMaxMessageLength is Discord's hard cap for message content
// (counted in UTF-16 code units, not Go runes or bytes).
const DiscordMaxMessageLength = 2000

// Shared REST client so ONLINE/OFFLINE notices reuse TLS connections
// instead of allocating a fresh http.Client per announce.
var announceHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

// DiscordUTF16Len returns Discord's character length for a string.
func DiscordUTF16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// FormatListenMessage fills {{name}}, {{playlist_id}}, {{channel_id}} in a template.
// Meat Bag configures these globally in Admin — same notices everywhere.
func FormatListenMessage(tmpl string, listen *db.ChannelMapping) string {
	if listen == nil {
		return tmpl
	}
	out := tmpl
	out = strings.ReplaceAll(out, "{{name}}", sanitizeAnnounceField(listen.Name))
	out = strings.ReplaceAll(out, "{{playlist_id}}", sanitizeAnnounceField(listen.YouTubePlaylistID))
	out = strings.ReplaceAll(out, "{{channel_id}}", sanitizeAnnounceField(listen.DiscordChannelID))
	return out
}

// FormatAirMessage is a legacy alias for FormatListenMessage.
func FormatAirMessage(tmpl string, listen *db.ChannelMapping) string {
	return FormatListenMessage(tmpl, listen)
}

// FormatPictureListenMessage fills {{name}}, {{slug}}, {{channel_id}} in a template.
func FormatPictureListenMessage(tmpl string, listen *db.PictureListener) string {
	if listen == nil {
		return tmpl
	}
	out := tmpl
	out = strings.ReplaceAll(out, "{{name}}", sanitizeAnnounceField(listen.Name))
	out = strings.ReplaceAll(out, "{{slug}}", sanitizeAnnounceField(listen.Slug))
	out = strings.ReplaceAll(out, "{{channel_id}}", sanitizeAnnounceField(listen.DiscordChannelID))
	return out
}

// sanitizeAnnounceField softens values Meat Bag (or Discord usernames) put into
// notice placeholders — blocks mass-ping tokens and keeps markdown links stable.
func sanitizeAnnounceField(s string) string {
	s = strings.ReplaceAll(s, "@everyone", "@\u200beveryone")
	s = strings.ReplaceAll(s, "@here", "@\u200bhere")
	// Break ]( sequences so a crafty name cannot close a markdown link early.
	s = strings.ReplaceAll(s, "]", "］")
	return s
}

// Announce posts a plain message to a Discord channel using the bot token (REST).
// Empty content is a no-op (Meat Bag silenced the template). Oversize content errors.
func Announce(ctx context.Context, token, channelID, content string) error {
	return announce(ctx, token, channelID, content, false)
}

// AnnounceBroadcast is Announce for Fire: when the target is a Discord
// announcement (news) channel, crosspost so followers receive it.
func AnnounceBroadcast(ctx context.Context, token, channelID, content string) error {
	return announce(ctx, token, channelID, content, true)
}

func announce(ctx context.Context, token, channelID, content string, crosspost bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	token = strings.TrimSpace(token)
	channelID = strings.TrimSpace(channelID)
	content = strings.TrimSpace(content)
	if token == "" {
		return fmt.Errorf("discord token empty")
	}
	if channelID == "" {
		return fmt.Errorf("channel id empty")
	}
	if content == "" {
		return nil // nothing to say — silenced template
	}
	if DiscordUTF16Len(content) > DiscordMaxMessageLength {
		return fmt.Errorf("notice exceeds Discord %d-character limit (%d)", DiscordMaxMessageLength, DiscordUTF16Len(content))
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("discord session: %w", err)
	}
	session.Client = announceHTTPClient

	// AllowedMentions with empty Parse = no @everyone / role / user pings from content.
	msg, err := session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	})
	if err != nil {
		return fmt.Errorf("announce in channel %s: %w", channelID, err)
	}
	slog.Info("announced in discord channel", "channel", channelID, "chars", DiscordUTF16Len(content))

	if crosspost && msg != nil && msg.ID != "" {
		if err := maybeCrosspost(session, channelID, msg.ID); err != nil {
			// Message is already in the channel — don't fail the send.
			slog.Warn("announcement crosspost failed", "channel", channelID, "err", err)
		}
	}
	return nil
}

// maybeCrosspost publishes to announcement followers when the channel is news-type.
func maybeCrosspost(session *discordgo.Session, channelID, messageID string) error {
	ch, err := session.Channel(channelID)
	if err != nil {
		return fmt.Errorf("posted but could not inspect channel %s for crosspost: %w", channelID, err)
	}
	if ch.Type != discordgo.ChannelTypeGuildNews {
		return nil
	}
	if _, err := session.ChannelMessageCrosspost(channelID, messageID); err != nil {
		return fmt.Errorf("posted in %s but crosspost failed: %w", channelID, err)
	}
	slog.Info("crossposted announcement", "channel", channelID, "message", messageID)
	return nil
}

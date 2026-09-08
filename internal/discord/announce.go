package discord

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
)

// FormatListenMessage fills {{name}}, {{playlist_id}}, {{channel_id}} in a template.
// Meat Bag configures these globally in Admin — same notices everywhere.
func FormatListenMessage(tmpl string, listen *db.ChannelMapping) string {
	if listen == nil {
		return tmpl
	}
	out := tmpl
	out = strings.ReplaceAll(out, "{{name}}", listen.Name)
	out = strings.ReplaceAll(out, "{{playlist_id}}", listen.YouTubePlaylistID)
	out = strings.ReplaceAll(out, "{{channel_id}}", listen.DiscordChannelID)
	return out
}

// FormatAirMessage is a legacy alias for FormatListenMessage.
func FormatAirMessage(tmpl string, listen *db.ChannelMapping) string {
	return FormatListenMessage(tmpl, listen)
}

// Announce posts a plain message to a Discord channel using the bot token (REST).
func Announce(token, channelID, content string) error {
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
		return nil // nothing to say — Meat Bag cleared the template
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("discord session: %w", err)
	}
	if _, err := session.ChannelMessageSend(channelID, content); err != nil {
		return fmt.Errorf("announce in channel %s: %w", channelID, err)
	}
	slog.Info("announced in discord channel", "channel", channelID, "bytes", len(content))
	return nil
}

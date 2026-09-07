package discord

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// GuildInfo is a server the bot is currently in (Admin UI dropdown).
type GuildInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChannelInfo is a text-like channel the bot can see in a guild.
type ChannelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type int    `json:"type"` // discordgo channel type int
}

// ListGuilds returns guilds from the gateway state (servers the bot is on).
// If DISCORD_GUILD_ID is set in config, only that guild is returned.
func (b *Bot) ListGuilds() ([]GuildInfo, error) {
	if b == nil || b.session == nil {
		return nil, fmt.Errorf("discord bot is not ready")
	}
	if !b.ready.Load() {
		return nil, fmt.Errorf("discord is not connected yet — wait a moment and retry")
	}

	guilds := b.session.State.Guilds
	out := make([]GuildInfo, 0, len(guilds))
	for _, g := range guilds {
		if g == nil {
			continue
		}
		if b.guildID != "" && g.ID != b.guildID {
			continue
		}
		name := g.Name
		if name == "" {
			name = g.ID
		}
		out = append(out, GuildInfo{ID: g.ID, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ListTextChannels returns text/announcement channels in a guild.
// Prefers gateway state; falls back to REST if state has no channels yet.
func (b *Bot) ListTextChannels(guildID string) ([]ChannelInfo, error) {
	if b == nil || b.session == nil {
		return nil, fmt.Errorf("discord bot is not ready")
	}
	if !b.ready.Load() {
		return nil, fmt.Errorf("discord is not connected yet — wait a moment and retry")
	}
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return nil, fmt.Errorf("guild id is required")
	}
	if b.guildID != "" && guildID != b.guildID {
		return nil, fmt.Errorf("guild %s is outside DISCORD_GUILD_ID filter", guildID)
	}

	channels, err := b.channelsForGuild(guildID)
	if err != nil {
		return nil, err
	}

	out := make([]ChannelInfo, 0, len(channels))
	for _, ch := range channels {
		if ch == nil || !isMappableChannel(ch.Type) {
			continue
		}
		name := ch.Name
		if name == "" {
			name = ch.ID
		}
		out = append(out, ChannelInfo{ID: ch.ID, Name: name, Type: int(ch.Type)})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (b *Bot) channelsForGuild(guildID string) ([]*discordgo.Channel, error) {
	if g, err := b.session.State.Guild(guildID); err == nil && g != nil && len(g.Channels) > 0 {
		return g.Channels, nil
	}
	// State can be empty right after connect — REST fills the gap.
	chs, err := b.session.GuildChannels(guildID)
	if err != nil {
		return nil, fmt.Errorf("list guild channels: %w", err)
	}
	return chs, nil
}

func isMappableChannel(t discordgo.ChannelType) bool {
	switch t {
	case discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews:
		return true
	default:
		return false
	}
}

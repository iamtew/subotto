package discord

import (
	"bytes"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// How many previous messages the Admin Chat tab loads.
const defaultChatHistory = 10

// Discord's ChannelMessages cap is 100; we keep Chat tiny on purpose.
const maxChatHistory = 10

// Discord allows 10 files per message. Regular bots are capped around 10 MiB each.
const MaxChatFiles = 10
const MaxChatFileBytes = 10 << 20
const MaxChatUploadBytes = MaxChatFiles*MaxChatFileBytes + 1<<20

// GuildInfo is a server the bot is currently in (Admin UI dropdown).
type GuildInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChannelInfo is a text-like channel the bot can see in a guild.
type ChannelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`     // discordgo channel type int
	CanSend bool   `json:"can_send"` // bot has Send Messages in this channel
}

// ChatMessage is one Discord message for the Admin Chat tab.
type ChatMessage struct {
	ID          string           `json:"id"`
	Author      string           `json:"author"`
	AuthorID    string           `json:"author_id,omitempty"`
	Content     string           `json:"content"`
	Timestamp   string           `json:"timestamp"`
	Bot         bool             `json:"bot"`
	Self        bool             `json:"self"` // posted by this Subotto bot
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

// ChatAttachment is a file on a Discord message (history + send).
type ChatAttachment struct {
	Filename    string `json:"filename"`
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// ChatFile is an upload from the Admin Chat tab.
type ChatFile struct {
	Name        string
	ContentType string
	Data        []byte
}

func (b *Bot) requireReady() error {
	if b == nil || b.session == nil {
		return fmt.Errorf("discord bot is not ready")
	}
	if !b.ready.Load() {
		return fmt.Errorf("discord is not connected yet — wait a moment and retry")
	}
	return nil
}

// ListGuilds returns guilds from the gateway state (servers the bot is on).
// If DISCORD_GUILD_ID is set in config, only that guild is returned.
func (b *Bot) ListGuilds() ([]GuildInfo, error) {
	if err := b.requireReady(); err != nil {
		return nil, err
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

// ListTextChannels returns text/announcement channels in a guild that the
// bot can actually View. Discord's guild channel list includes every channel
// the gateway knows about — including ones with a View Channel deny overwrite
// — so Admin dropdowns used to offer rooms the bot cannot use.
func (b *Bot) ListTextChannels(guildID string) ([]ChannelInfo, error) {
	if err := b.requireReady(); err != nil {
		return nil, err
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

	// One REST member lookup if the bot is missing from gateway state.
	if err := b.ensureBotMember(guildID); err != nil {
		slog.Debug("bot member missing; channel prune may be incomplete", "guild", guildID, "err", err)
	}

	out := make([]ChannelInfo, 0, len(channels))
	for _, ch := range channels {
		if ch == nil || !isMappableChannel(ch.Type) {
			continue
		}
		if ch.GuildID == "" {
			ch.GuildID = guildID
		}
		canSend := true
		if perms, ok := b.channelPerms(ch); ok {
			if perms&discordgo.PermissionViewChannel == 0 {
				continue
			}
			canSend = perms&discordgo.PermissionSendMessages != 0
		}
		name := ch.Name
		if name == "" {
			name = ch.ID
		}
		out = append(out, ChannelInfo{ID: ch.ID, Name: name, Type: int(ch.Type), CanSend: canSend})
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

// ensureBotMember puts this bot's guild member into gateway state so
// permission math can run locally (no REST per channel).
func (b *Bot) ensureBotMember(guildID string) error {
	if b.session.State == nil || b.session.State.User == nil {
		return fmt.Errorf("bot user not in session state")
	}
	uid := b.session.State.User.ID
	if _, err := b.session.State.Member(guildID, uid); err == nil {
		return nil
	}
	m, err := b.session.GuildMember(guildID, uid)
	if err != nil {
		return fmt.Errorf("fetch bot member: %w", err)
	}
	return b.session.State.MemberAdd(m)
}

// channelPerms returns the bot's permission bitfield for a channel.
// ok=false means we could not compute it (keep the channel in the list).
func (b *Bot) channelPerms(ch *discordgo.Channel) (int64, bool) {
	if b.session == nil || b.session.State == nil || b.session.State.User == nil || ch == nil {
		return 0, false
	}
	uid := b.session.State.User.ID
	if _, err := b.session.State.Channel(ch.ID); err != nil {
		_ = b.session.State.ChannelAdd(ch)
	}
	perms, err := b.session.State.UserChannelPermissions(uid, ch.ID)
	if err != nil {
		perms, err = b.session.UserChannelPermissions(uid, ch.ID)
		if err != nil {
			slog.Debug("channel permission unknown", "channel", ch.ID, "err", err)
			return 0, false
		}
	}
	return perms, true
}

func clampChatHistory(n int) int {
	if n <= 0 {
		return defaultChatHistory
	}
	if n > maxChatHistory {
		return maxChatHistory
	}
	return n
}

// ListRecentMessages fetches the last N messages in a channel (oldest first).
func (b *Bot) ListRecentMessages(channelID string, limit int) ([]ChatMessage, error) {
	if err := b.requireReady(); err != nil {
		return nil, err
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	limit = clampChatHistory(limit)

	msgs, err := b.session.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("fetch channel messages: %w", err)
	}

	botID := ""
	if b.session.State != nil && b.session.State.User != nil {
		botID = b.session.State.User.ID
	}

	// Discord returns newest first. Chat UI wants oldest at the top.
	out := make([]ChatMessage, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i] == nil {
			continue
		}
		out = append(out, toChatMessage(msgs[i], botID))
	}
	return out, nil
}

// SendChannelMessage posts as the bot (text and/or files). Mentions work —
// this is operator chat, not a template notice.
func (b *Bot) SendChannelMessage(channelID, content string, files []ChatFile) (*ChatMessage, error) {
	if err := b.requireReady(); err != nil {
		return nil, err
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	content, files, err := normalizeChatUpload(content, files)
	if err != nil {
		return nil, err
	}

	payload := &discordgo.MessageSend{Content: content}
	for _, f := range files {
		payload.Files = append(payload.Files, &discordgo.File{
			Name:        f.Name,
			ContentType: f.ContentType,
			Reader:      bytes.NewReader(f.Data),
		})
	}

	msg, err := b.session.ChannelMessageSendComplex(channelID, payload)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	botID := ""
	if b.session.State != nil && b.session.State.User != nil {
		botID = b.session.State.User.ID
	}
	out := toChatMessage(msg, botID)
	return &out, nil
}

func normalizeChatUpload(content string, files []ChatFile) (string, []ChatFile, error) {
	content = strings.TrimSpace(content)
	if DiscordUTF16Len(content) > DiscordMaxMessageLength {
		return "", nil, fmt.Errorf("message exceeds Discord %d-character limit (%d)", DiscordMaxMessageLength, DiscordUTF16Len(content))
	}
	if len(files) > MaxChatFiles {
		return "", nil, fmt.Errorf("too many attachments (max %d)", MaxChatFiles)
	}
	out := make([]ChatFile, 0, len(files))
	for _, f := range files {
		name := sanitizeChatFilename(f.Name)
		if name == "" {
			return "", nil, fmt.Errorf("attachment is missing a filename")
		}
		if len(f.Data) == 0 {
			return "", nil, fmt.Errorf("%s is empty", name)
		}
		if len(f.Data) > MaxChatFileBytes {
			return "", nil, fmt.Errorf("%s exceeds Discord %d MiB cap", name, MaxChatFileBytes>>20)
		}
		out = append(out, ChatFile{Name: name, ContentType: strings.TrimSpace(f.ContentType), Data: f.Data})
	}
	if content == "" && len(out) == 0 {
		return "", nil, fmt.Errorf("message is empty")
	}
	return content, out, nil
}

func sanitizeChatFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

func toChatMessage(m *discordgo.Message, botID string) ChatMessage {
	if m == nil {
		return ChatMessage{}
	}
	author := displayNameFromMessage(m)
	authorID := ""
	isBot := false
	self := false
	if m.Author != nil {
		authorID = m.Author.ID
		isBot = m.Author.Bot
		self = botID != "" && m.Author.ID == botID
	}
	ts := ""
	if !m.Timestamp.IsZero() {
		ts = m.Timestamp.UTC().Format(time.RFC3339)
	}
	return ChatMessage{
		ID:          m.ID,
		Author:      author,
		AuthorID:    authorID,
		Content:     strings.TrimSpace(m.Content),
		Timestamp:   ts,
		Bot:         isBot,
		Self:        self,
		Attachments: chatAttachments(m),
	}
}

func chatAttachments(m *discordgo.Message) []ChatAttachment {
	if m == nil || len(m.Attachments) == 0 {
		return nil
	}
	out := make([]ChatAttachment, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.Filename)
		if name == "" {
			name = a.ID
		}
		out = append(out, ChatAttachment{
			Filename:    name,
			URL:         a.URL,
			ContentType: a.ContentType,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

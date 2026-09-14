package discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// formatSlideshowMessage turns Discord message content into slideshow caption
// text: user/role/channel mentions become @name / #name. Custom emoji tokens
// (<:name:id> / <a:name:id>) stay so the overlay can render CDN images.
func formatSlideshowMessage(s *discordgo.Session, m *discordgo.Message) string {
	if m == nil {
		return ""
	}
	text := m.Content
	if text == "" {
		return ""
	}

	for _, u := range m.Mentions {
		if u == nil || u.ID == "" {
			continue
		}
		name := "@" + displayNameFromUser(u)
		text = strings.ReplaceAll(text, "<@!"+u.ID+">", name)
		text = strings.ReplaceAll(text, "<@"+u.ID+">", name)
	}

	guildID := strings.TrimSpace(m.GuildID)
	for _, rid := range m.MentionRoles {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		label := "@" + rid
		if s != nil && s.State != nil && guildID != "" {
			if role, err := s.State.Role(guildID, rid); err == nil && role != nil && strings.TrimSpace(role.Name) != "" {
				label = "@" + role.Name
			}
		}
		text = strings.ReplaceAll(text, "<@&"+rid+">", label)
	}

	for _, ch := range m.MentionChannels {
		if ch == nil || ch.ID == "" {
			continue
		}
		name := ch.Name
		if strings.TrimSpace(name) == "" {
			name = ch.ID
		}
		text = strings.ReplaceAll(text, "<#"+ch.ID+">", "#"+name)
	}

	return strings.TrimSpace(text)
}

func displayNameFromUser(u *discordgo.User) string {
	if u == nil {
		return "unknown"
	}
	if gn := strings.TrimSpace(u.GlobalName); gn != "" {
		return gn
	}
	if name := strings.TrimSpace(u.Username); name != "" {
		return name
	}
	if u.ID != "" {
		return u.ID
	}
	return "unknown"
}

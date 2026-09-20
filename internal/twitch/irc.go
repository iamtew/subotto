package twitch

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ircHost = "irc.chat.twitch.tv:6697"

// Twitch chat cap. Truncate model output rather than fail the send.
const heshMaxReplyRunes = 500

type ircLine struct {
	Tags    map[string]string
	Nick    string
	Command string
	Params  []string
}

func parseIRCLine(raw string) (ircLine, bool) {
	raw = strings.TrimRight(raw, "\r\n")
	if raw == "" {
		return ircLine{}, false
	}
	out := ircLine{}
	rest := raw
	if strings.HasPrefix(rest, "@") {
		tags, after, ok := strings.Cut(rest[1:], " ")
		if !ok {
			return ircLine{}, false
		}
		out.Tags = parseTags(tags)
		rest = after
	}
	if strings.HasPrefix(rest, ":") {
		prefix, after, ok := strings.Cut(rest[1:], " ")
		if !ok {
			return ircLine{}, false
		}
		out.Nick, _, _ = strings.Cut(prefix, "!")
		rest = after
	}
	cmd, after, found := strings.Cut(rest, " ")
	out.Command = cmd
	if !found {
		return out, out.Command != ""
	}
	rest = after
	for rest != "" {
		if strings.HasPrefix(rest, ":") {
			out.Params = append(out.Params, rest[1:])
			break
		}
		p, after, found := strings.Cut(rest, " ")
		out.Params = append(out.Params, p)
		if !found {
			break
		}
		rest = after
	}
	return out, out.Command != ""
}

func parseTags(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		out[k] = unescapeTag(v)
	}
	return out
}

func unescapeTag(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case ':':
			b.WriteByte(';')
		case 's':
			b.WriteByte(' ')
		case '\\':
			b.WriteByte('\\')
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (l ircLine) text() string {
	if len(l.Params) == 0 {
		return ""
	}
	return l.Params[len(l.Params)-1]
}

func (l ircLine) tag(key string) string {
	if l.Tags == nil {
		return ""
	}
	return l.Tags[key]
}

func privmsgOut(channel, text, parentMsgID string) string {
	channel = NormalizeChannel(channel)
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	text = truncateRunes(text, heshMaxReplyRunes)
	line := fmt.Sprintf("PRIVMSG #%s :%s", channel, text)
	if parentMsgID != "" {
		return "@reply-parent-msg-id=" + parentMsgID + " " + line
	}
	return line
}

// NormalizeChannel lowercases a Twitch login and strips a leading #.
// Empty is allowed (means “do not join”). Invalid non-empty values return "".
func NormalizeChannel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "#")
	if s == "" {
		return ""
	}
	if len(s) > 25 {
		return ""
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return s
}

// ParseChannel validates an Admin-typed join target. Empty is OK.
func ParseChannel(s string) (string, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return "", nil
	}
	out := NormalizeChannel(raw)
	if out == "" {
		return "", fmt.Errorf("twitch channel must be a login (letters, digits, underscore)")
	}
	return out, nil
}

func hasAtName(text, name string) bool {
	name = strings.TrimSpace(strings.TrimPrefix(name, "@"))
	if name == "" {
		return false
	}
	lower := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower("@" + name))
	if len(needle) > len(lower) {
		return false
	}
	for i := 0; i <= len(lower)-len(needle); i++ {
		if !runesEqual(lower[i:i+len(needle)], needle) {
			continue
		}
		end := i + len(needle)
		if end < len(lower) && isNameRune(lower[end]) {
			continue
		}
		return true
	}
	return false
}

func stripAtName(text, name string) string {
	name = strings.TrimSpace(strings.TrimPrefix(name, "@"))
	if name == "" {
		return strings.TrimSpace(text)
	}
	lower := []rune(strings.ToLower(text))
	orig := []rune(text)
	needle := []rune(strings.ToLower("@" + name))
	var out []rune
	for i := 0; i < len(orig); {
		if i+len(needle) <= len(lower) && runesEqual(lower[i:i+len(needle)], needle) {
			end := i + len(needle)
			if end >= len(lower) || !isNameRune(lower[end]) {
				i = end
				continue
			}
		}
		out = append(out, orig[i])
		i++
	}
	return strings.TrimSpace(string(out))
}

func userText(content, login, display string) string {
	s := stripAtName(content, login)
	if !strings.EqualFold(login, display) {
		s = stripAtName(s, display)
	}
	if s == "" {
		return "hey"
	}
	return s
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max < 4 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

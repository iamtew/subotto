// Package parser finds YouTube video IDs inside messy human text.
//
// Meat Bag: Discord messages are rarely a clean URL. People paste links with
// timestamps, tracking junk, shorts, youtu.be, etc. This package pulls out
// the 11-character video IDs Subotto needs.
package parser

import (
	"net/url"
	"regexp"
	"strings"
)

// YouTube video IDs are 11 characters from this alphabet.
var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Matches common YouTube URL shapes inside larger strings.
// We intentionally keep this a bit greedy and then validate each candidate.
var urlFinder = regexp.MustCompile(`(?i)https?://(?:(?:www|m|music)\.)?(?:youtube\.com|youtu\.be)/[^\s<>\]\)\"]+`)

// ExtractVideoIDs returns unique video IDs found in text, in first-seen order.
func ExtractVideoIDs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var out []string

	add := func(id string) {
		id = strings.TrimSpace(id)
		if !videoIDPattern.MatchString(id) {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	for _, raw := range urlFinder.FindAllString(text, -1) {
		cleaned := strings.TrimRight(raw, ".,;:!?")
		add(idFromURL(cleaned))
	}

	// Also catch bare youtu.be / watch links without scheme (less common, but humans).
	for _, raw := range bareLinkFinder.FindAllString(text, -1) {
		cleaned := strings.TrimRight(raw, ".,;:!?")
		if !strings.Contains(strings.ToLower(cleaned), "http") {
			cleaned = "https://" + cleaned
		}
		add(idFromURL(cleaned))
	}

	return out
}

var bareLinkFinder = regexp.MustCompile(`(?i)(?:(?:www|m|music)\.)?(?:youtube\.com|youtu\.be)/[^\s<>\]\)\"]+`)

func idFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	path := u.Path

	switch {
	case host == "youtu.be":
		// https://youtu.be/VIDEOID?t=30
		id := strings.TrimPrefix(path, "/")
		if slash := strings.Index(id, "/"); slash >= 0 {
			id = id[:slash]
		}
		return id

	case strings.HasSuffix(host, "youtube.com"):
		// /watch?v=VIDEOID
		if v := u.Query().Get("v"); v != "" {
			return v
		}
		// /shorts/VIDEOID, /embed/VIDEOID, /live/VIDEOID, /v/VIDEOID
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 2 {
			switch strings.ToLower(parts[0]) {
			case "shorts", "embed", "live", "v":
				return parts[1]
			}
		}
	}
	return ""
}

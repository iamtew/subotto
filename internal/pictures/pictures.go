// Package pictures downloads Discord image attachments for picture listeners.
//
// Meat Bag: when someone posts a PNG/JPEG/WebP/GIF in a channel with a live
// picture listener, Subotto saves the file under data/pictures/{slug}/ and
// records it for the public OBS slideshow.
package pictures

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"subotto/internal/db"
)

// Result summarizes one message's picture ingest.
// Skipped = SkippedSame + SkippedOld (kept for resync summaries / String()).
// SkippedSame = already saved for this listener epoch (DUPE react).
// SkippedOld  = already filed under a previous picture-listener epoch (OLD react).
// OriginHits  = this Discord message is the one already on file (💾 chrome on resync).
type Result struct {
	Saved       int
	Skipped     int
	SkippedSame int
	SkippedOld  int
	Failed      int
	OriginHits  int
}

// Merge adds another Result into r (used by picture resync to accumulate totals).
func (r *Result) Merge(other Result) {
	r.Saved += other.Saved
	r.Skipped += other.Skipped
	r.SkippedSame += other.SkippedSame
	r.SkippedOld += other.SkippedOld
	r.Failed += other.Failed
	r.OriginHits += other.OriginHits
}

func (r Result) String() string {
	return fmt.Sprintf("saved=%d skipped=%d failed=%d", r.Saved, r.Skipped, r.Failed)
}

var imageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// IsImageContentType reports whether Discord's ContentType is a supported image.
func IsImageContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	_, ok := imageTypes[ct]
	return ok
}

func extForContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ext, ok := imageTypes[ct]; ok {
		return ext
	}
	return ".bin"
}

// Attachment is the minimal Discord attachment info we need (keeps package free of discordgo).
type Attachment struct {
	ID          string
	URL         string
	Filename    string
	ContentType string
	Width       int
	Height      int
}

// ProcessAttachments downloads image attachments for a picture listener.
func ProcessAttachments(
	ctx context.Context,
	store *db.DB,
	listener *db.PictureListener,
	messageID, authorID, authorDisplayName string,
	atts []Attachment,
	reactions []db.ReactionCount,
) Result {
	var res Result
	if store == nil || listener == nil {
		return res
	}
	if err := store.EnsurePictureListenerDir(listener.Slug); err != nil {
		slog.Error("ensure picture dir failed", "slug", listener.Slug, "err", err)
		res.Failed = len(atts)
		return res
	}

	client := &http.Client{Timeout: 60 * time.Second}

	for _, att := range atts {
		if !IsImageContentType(att.ContentType) {
			// Discord sometimes omits ContentType; fall back to filename extension.
			if !looksLikeImageFilename(att.Filename) {
				continue
			}
			if att.ContentType == "" {
				att.ContentType = guessContentType(att.Filename)
			}
			if !IsImageContentType(att.ContentType) {
				continue
			}
		}

		prev, already, err := store.CollectedAttachmentOnChannel(ctx, listener.DiscordChannelID, att.ID)
		if err != nil {
			slog.Error("picture dedupe lookup failed", "err", err)
			res.Failed++
			continue
		}
		if already {
			sameListener := prev.ListenerID == listener.ID
			origin := sameListener && prev.MessageID != "" && prev.MessageID == messageID
			if origin {
				res.OriginHits++
			}
			reason := "duplicate"
			if origin {
				reason = "already_this_message"
				res.SkippedSame++
			} else if sameListener {
				res.SkippedSame++
			} else {
				res.SkippedOld++
				reason = "duplicate_old"
			}
			res.Skipped++
			slog.Info("skip duplicate picture",
				"attachment", att.ID,
				"channel", listener.DiscordChannelID,
				"listener", listener.ID,
				"prev_listener", prev.ListenerID,
				"prev_message", prev.MessageID,
				"reason", reason,
			)
			_ = store.LogActivity(ctx, "picture_skipped", map[string]any{
				"reason":           reason,
				"attachment_id":    att.ID,
				"listener_id":      listener.ID,
				"prev_listener_id": prev.ListenerID,
				"channel_id":       listener.DiscordChannelID,
				"message_id":       messageID,
			}, true)
			continue
		}

		ext := extForContentType(att.ContentType)
		rel := filepath.ToSlash(filepath.Join(listener.Slug, messageID+"_"+att.ID+ext))
		abs, err := store.AbsolutePicturePath(rel)
		if err != nil {
			slog.Error("picture path rejected", "rel", rel, "err", err)
			res.Failed++
			continue
		}

		if err := downloadToFile(ctx, client, att.URL, abs); err != nil {
			slog.Error("download picture failed", "url", att.URL, "err", err)
			_ = os.Remove(abs)
			res.Failed++
			continue
		}

		_, err = store.InsertCollectedPicture(ctx, db.CollectedPicture{
			ListenerID:          listener.ID,
			DiscordMessageID:    messageID,
			DiscordAttachmentID: att.ID,
			AuthorID:            authorID,
			AuthorDisplayName:   authorDisplayName,
			StoredPath:          rel,
			ContentType:         att.ContentType,
			Reactions:           reactions,
		})
		if err != nil {
			slog.Error("insert collected picture failed", "err", err)
			_ = os.Remove(abs)
			res.Failed++
			continue
		}

		_ = store.LogActivity(ctx, "picture_saved", map[string]any{
			"listener_id":   listener.ID,
			"slug":          listener.Slug,
			"message_id":    messageID,
			"attachment_id": att.ID,
			"author":        authorDisplayName,
			"path":          rel,
		}, true)
		res.Saved++
	}
	return res
}

func looksLikeImageFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func guessContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func downloadToFile(ctx context.Context, client *http.Client, url, dest string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("empty attachment url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, 40<<20)) // 40 MiB cap
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

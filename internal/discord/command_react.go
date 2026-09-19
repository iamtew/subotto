package discord

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
)

type playlistRemover interface {
	RemoveVideoFromPlaylist(ctx context.Context, playlistID, videoID string) error
}

func (b *Bot) isSuperadminSkipReact(r *discordgo.MessageReactionAdd) bool {
	if b == nil || b.superadminID == "" || r == nil || r.UserID != b.superadminID {
		return false
	}
	if r.Emoji.Name == "" {
		return false
	}
	return normalizeEmoji(r.Emoji.Name) == normalizeEmoji(chromeFailed[0])
}

func (b *Bot) handleSuperadminSkip(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	ctx := context.Background()
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		slog.Warn("skip command: fetch message failed", "channel", r.ChannelID, "message", r.MessageID, "err", err)
		return
	}
	if !messageHasSubottoChrome(msg) {
		return
	}

	hid, err := b.store.SetCollectedPicturesIgnoredByMessage(ctx, r.ChannelID, r.MessageID, true)
	if err != nil {
		slog.Error("skip command: hide pictures failed", "err", err)
		return
	}

	skipped, skipErr := skipVideosOnMessage(ctx, b.store, b.yt, r.ChannelID, r.MessageID)
	if skipErr != nil {
		slog.Error("skip command: playlist skip failed", "err", skipErr)
	}
	if hid == 0 && skipped == 0 {
		return
	}

	fresh, ferr := s.ChannelMessage(r.ChannelID, r.MessageID)
	if ferr != nil {
		fresh = msg
	}
	if skipped > 0 {
		syncStatusChrome(ctx, s, r.ChannelID, r.MessageID, fresh, chromeSkip)
	} else {
		pics, _ := b.store.ListCollectedPicturesByMessage(ctx, r.ChannelID, r.MessageID)
		syncStatusChrome(ctx, s, r.ChannelID, r.MessageID, fresh, PictureIgnoreChrome(pics))
	}
	_ = b.store.LogActivity(ctx, "listener_skip", map[string]any{
		"channel_id": r.ChannelID,
		"message_id": r.MessageID,
		"user_id":    r.UserID,
		"pictures":   hid,
		"videos":     skipped,
	}, true)
	slog.Info("superadmin skip", "channel", r.ChannelID, "message", r.MessageID, "pictures", hid, "videos", skipped)
}

func messageHasSubottoChrome(msg *discordgo.Message) bool {
	if msg == nil {
		return false
	}
	for _, r := range msg.Reactions {
		if r == nil || r.Emoji == nil || !r.Me || r.Emoji.Name == "" {
			continue
		}
		if isManagedStatusChrome(normalizeEmoji(r.Emoji.Name)) {
			return true
		}
	}
	return false
}

func skipVideosOnMessage(ctx context.Context, store *db.DB, yt playlistRemover, channelID, messageID string) (int, error) {
	rows, err := store.ListProcessedVideosByMessage(ctx, channelID, messageID)
	if err != nil {
		return 0, err
	}
	n := 0
	needYT := false
	for _, row := range rows {
		if !row.Skipped {
			needYT = true
			break
		}
	}
	if needYT && yt == nil {
		return 0, errors.New("youtube client is nil")
	}
	for _, row := range rows {
		if row.Skipped {
			continue
		}
		if yt != nil {
			if err := yt.RemoveVideoFromPlaylist(ctx, row.PlaylistID, row.VideoID); err != nil {
				return n, err
			}
		}
		if err := store.MarkVideoSkipped(ctx, row.VideoID, channelID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

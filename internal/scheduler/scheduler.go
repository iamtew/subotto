// Package scheduler runs optional background channel re-scans.
//
// Meat Bag: the Admin Scheduler tab owns interval, message amount, and which
// listeners to walk. RESYNC_INTERVAL_HOURS in .env is only used until the first
// Admin save. Interval 0 = off. Live Discord posts still work either way.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/youtube"
)

// resyncFn is the content history-scan. Production uses discord.ResyncChannel.
type resyncFn func(
	ctx context.Context,
	token string,
	store *db.DB,
	yt *youtube.Client,
	channelID string,
	limit int,
) (*discord.ResyncSummary, error)

// pictureResyncFn is the picture history-scan.
type pictureResyncFn func(
	ctx context.Context,
	token string,
	store *db.DB,
	channelID string,
	limit int,
) (*discord.PictureResyncSummary, error)

// Scheduler ticks every N hours and re-scans selected listeners.
type Scheduler struct {
	store        *db.DB
	yt           *youtube.Client
	discordToken string
	envHours     int // .env fallback until Admin saves a row
	resync       resyncFn
	picture      pictureResyncFn
	reload       chan struct{}

	mu        sync.Mutex
	lastRunAt time.Time
	lastErr   string
}

// Options configures a Scheduler.
type Options struct {
	Store               *db.DB
	YouTube             *youtube.Client
	DiscordToken        string
	ResyncIntervalHours int // env bootstrap when no DB row
}

// Info is a snapshot for the Admin /api/status endpoint.
type Info struct {
	Enabled       bool   `json:"scheduler_enabled"`
	IntervalHours int    `json:"resync_interval_hours"`
	Limit         int    `json:"resync_limit,omitempty"`
	LastRunAt     string `json:"scheduler_last_run_at,omitempty"`
	LastError     string `json:"scheduler_last_error,omitempty"`
}

// New builds a scheduler. Run always blocks until ctx cancel so Admin can
// enable the ticker later without a process restart.
func New(opts Options) *Scheduler {
	return &Scheduler{
		store:        opts.Store,
		yt:           opts.YouTube,
		discordToken: opts.DiscordToken,
		envHours:     opts.ResyncIntervalHours,
		resync:       discord.ResyncChannel,
		picture:      discord.ResyncPictureChannel,
		reload:       make(chan struct{}, 1),
	}
}

func (s *Scheduler) loadCfg(ctx context.Context) db.ResyncScheduler {
	if s == nil || s.store == nil {
		return db.DefaultResyncScheduler(0)
	}
	cfg, err := s.store.LoadResyncScheduler(ctx, s.envHours)
	if err != nil {
		slog.Error("scheduler could not load settings", "err", err)
		return db.DefaultResyncScheduler(s.envHours)
	}
	return cfg
}

// Enabled reports whether background resync is on (interval > 0).
func (s *Scheduler) Enabled() bool {
	if s == nil {
		return false
	}
	return s.loadCfg(context.Background()).IntervalHours > 0
}

// Info returns a thread-safe status snapshot for the Admin UI.
func (s *Scheduler) Info() Info {
	if s == nil {
		return Info{Enabled: false, IntervalHours: 0}
	}
	cfg := s.loadCfg(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	info := Info{
		Enabled:       cfg.IntervalHours > 0,
		IntervalHours: cfg.IntervalHours,
		Limit:         cfg.Limit,
		LastError:     s.lastErr,
	}
	if !s.lastRunAt.IsZero() {
		info.LastRunAt = s.lastRunAt.UTC().Format(time.RFC3339)
	}
	return info
}

// Reload wakes the wait loop so a settings save does not sit out the old interval.
func (s *Scheduler) Reload() {
	if s == nil {
		return
	}
	select {
	case s.reload <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is cancelled. Interval 0 waits for Reload (or cancel).
func (s *Scheduler) Run(ctx context.Context) {
	if s == nil {
		return
	}
	slog.Info("scheduler loop started (Admin Scheduler tab; interval 0 = idle)")

	for {
		if err := ctx.Err(); err != nil {
			slog.Info("scheduler stopped")
			return
		}
		cfg := s.loadCfg(ctx)
		if cfg.IntervalHours <= 0 {
			slog.Info("scheduler idle (interval 0)")
			select {
			case <-ctx.Done():
				slog.Info("scheduler stopped")
				return
			case <-s.reload:
				continue
			}
		}

		interval := time.Duration(cfg.IntervalHours) * time.Hour
		slog.Info("scheduler waiting",
			"interval_hours", cfg.IntervalHours,
			"resync_limit", cfg.Limit,
			"scope", cfg.Scope,
		)
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-s.reload:
			continue
		case <-time.After(interval):
			s.runOnceCfg(ctx, cfg)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.runOnceCfg(ctx, s.loadCfg(ctx))
}

func (s *Scheduler) runOnceCfg(ctx context.Context, cfg db.ResyncScheduler) {
	if err := ctx.Err(); err != nil {
		return
	}

	_ = s.store.LogActivity(ctx, "resync_scheduled_started", map[string]any{
		"source":         "scheduler",
		"interval_hours": cfg.IntervalHours,
		"limit":          cfg.Limit,
		"scope":          cfg.Scope,
	}, true)

	var ran, failed int
	var firstErr error

	list, err := s.store.ListMappings(ctx)
	if err != nil {
		s.record(err)
		slog.Error("scheduler could not list content listeners", "err", err)
		_ = s.store.LogActivity(ctx, "resync_scheduled_finished", map[string]any{
			"source": "scheduler",
			"error":  err.Error(),
		}, false)
		return
	}
	for _, m := range list {
		if !m.Enabled || !cfg.Wants(db.ResyncKindContent, m.DiscordChannelID) {
			continue
		}
		if err := ctx.Err(); err != nil {
			s.record(err)
			return
		}
		ran++
		slog.Info("scheduler content resync starting",
			"channel", m.DiscordChannelID,
			"playlist", m.YouTubePlaylistID,
			"name", m.Name,
		)
		_, err := s.resync(ctx, s.discordToken, s.store, s.yt, m.DiscordChannelID, cfg.Limit)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("scheduler content resync failed",
				"channel", m.DiscordChannelID,
				"err", err,
			)
		}
	}

	pics, err := s.store.ListPictureListeners(ctx)
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
		s.record(firstErr)
		slog.Error("scheduler could not list picture listeners", "err", err)
		_ = s.store.LogActivity(ctx, "resync_scheduled_finished", map[string]any{
			"source":           "scheduler",
			"channels_scanned": ran,
			"channels_failed":  failed,
			"error":            err.Error(),
		}, false)
		return
	}
	for _, p := range pics {
		if !p.Enabled || !cfg.Wants(db.ResyncKindPicture, p.DiscordChannelID) {
			continue
		}
		if err := ctx.Err(); err != nil {
			s.record(err)
			return
		}
		ran++
		slog.Info("scheduler picture resync starting",
			"channel", p.DiscordChannelID,
			"slug", p.Slug,
			"name", p.Name,
		)
		_, err := s.picture(ctx, s.discordToken, s.store, p.DiscordChannelID, cfg.Limit)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("scheduler picture resync failed",
				"channel", p.DiscordChannelID,
				"err", err,
			)
		}
	}

	if firstErr != nil {
		s.record(firstErr)
	} else {
		s.record(nil)
	}

	_ = s.store.LogActivity(ctx, "resync_scheduled_finished", map[string]any{
		"source":           "scheduler",
		"channels_scanned": ran,
		"channels_failed":  failed,
	}, firstErr == nil)

	slog.Info("scheduler tick finished",
		"channels_scanned", ran,
		"channels_failed", failed,
	)
}

func (s *Scheduler) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRunAt = time.Now().UTC()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
}

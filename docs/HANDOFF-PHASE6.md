# Handoff — Phase 6 (Scheduling & Polish)

**For:** the next Cursor agent / maintainer  
**From:** Phase 0–5 arc + live DEV verification  
**Date:** 2026-09-07  
**Branch:** `master` (verified good)

**Subotto works in DEV** (Discord → playlist, Admin UI, reactions).  
This handoff was to **prepare and implement Phase 6** from [PLAN.md](../PLAN.md). Do **not** jump to Phase 7 unless asked. Phase 6 has since shipped — see [PROGRESS.md](PROGRESS.md) and [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md).

---

## 1. Read these first (in order)

1. [AGENTS.md](../AGENTS.md) — conventions, Just/no Docker, git message style  
2. [JUMPBACK.md](JUMPBACK.md) — current resume snapshot  
3. [PROGRESS.md](PROGRESS.md) — what shipped + verify status  
4. [DEV-VERIFY.md](DEV-VERIFY.md) — how operators run DEV (do not break this path)  
5. This file  
6. [PLAN.md](../PLAN.md) § Phase 6  

Suggested opener:  
> Read docs/HANDOFF-PHASE6.md and continue Phase 6.

---

## 2. Verification status (do not re-litigate)

Live DEV confirmed:

- Discord connects (Message Content Intent on)
- YouTube OAuth + playlist writes work
- Mapping via Admin UI (and/or CLI) works
- Fresh YouTube link → video on playlist → reaction **💾**
- Same-listener duplicate → **♻️** + letter reacts **DUPE**; previous-listener → **🛑** + **OLD**; failures → **❌**
- Reaction failures log at **Warn** (permissions issues are visible)
- Admin UI Basic Auth: user `admin` / `ADMIN_PASSWORD`

Code checks already green historically: `just test`, `just build`. Re-run after your changes.

---

## 3. What Phase 6 must deliver (PLAN.md)

| Item | Intent |
|------|--------|
| Optional background re-scans | Honor `RESYNC_INTERVAL_HOURS` (already in config; **0 = disabled**, unused today) |
| Rate-limit handling | Graceful retries / clearer handling when Discord or YouTube throttle |
| Graceful shutdown | Largely done in Phase 5 (signal → admin Shutdown + Discord Close) — polish if gaps remain |
| Health / status endpoint | `/api/status` exists; may extend (uptime, last resync, scheduler state) — keep Basic Auth unless a public `/healthz` is requested |
| Final justfile / README polish | DEV recipes stay clear; light deploy notes OK, but **full VPS/systemd guide is Phase 7** |

Empty scaffold waiting: `internal/scheduler/` (was `.gitkeep` only; package may need creating).

---

## 4. Architecture you inherit

```text
just run
  → Discord gateway
       MessageCreate → enabled mapping? → ingest.ProcessContent → react 💾 / ♻️DUPE / 🛑OLD / ❌
  → HTTP Admin (webroot + /api/*) Basic Auth
       GET  /api/status
       GET/POST /api/mappings
       PATCH/DELETE /api/mappings/{channel}
       GET  /api/activity
       POST /api/resync   → discord.ResyncChannel (REST history, no reactions)

Shared pipeline: internal/ingest (live + CLI/UI resync + future scheduled resync)
```

### Key paths

| Path | Role |
|------|------|
| `cmd/subotto/main.go` | Flags, `runBot`, wiring |
| `internal/discord/bot.go` | Live handler + reactions |
| `internal/discord/resync.go` | History scan (cap 500) |
| `internal/ingest/` | Dedup + YouTube add + activity |
| `internal/web/` | Admin server + API |
| `internal/config/config.go` | Includes `ResyncIntervalHours` |
| `internal/scheduler/` | **Phase 6 home** |

### Hard constraints

- **No Docker.** Just + native Go. Pure Go SQLite (`modernc.org/sqlite`), no CGO.  
- Plain, beginner-friendly comments.  
- Do not commit `.env` or `data/*.db`.  
- Commit only when the maintainer asks; title + bullet body; prefer `-F` file on Windows PowerShell.  
- OAuth one-shot and Admin both default to port **50770** — auth exits; bot holds the port. Don’t break that.  
- `DeleteMapping` leaves `processed_videos` (intentional).  
- Scheduled resync should **reuse** `discord.ResyncChannel` / ingest — no second pipeline.  
- Scheduled jobs must respect **enabled** mappings only; mind YouTube **quota** (playlist insert ~50 units). Prefer conservative defaults and clear logs.

---

## 5. Suggested Phase 6 implementation shape

Not mandatory dogma — ask if trade-offs are big — but a sane default:

1. **`internal/scheduler`**  
   - If `ResyncIntervalHours <= 0`, no-op.  
   - Otherwise ticker/loop: for each enabled mapping, call `ResyncChannel` with a modest limit (e.g. same default 100, or a dedicated config later).  
   - Log `resync_scheduled_*` (or reuse started/finished) in activity.  
   - Cancel cleanly on process shutdown (context from `runBot`).

2. **Wire from `runBot`**  
   - Start scheduler goroutine after Discord + Admin are up.  
   - Shutdown path cancels scheduler before/with HTTP shutdown.

3. **Rate limits**  
   - YouTube: improve handling around quota / 403 / 429 if gaps remain in `internal/youtube`.  
   - Discord: backoff on REST resync failures where useful.  
   - Keep it simple — no huge framework.

4. **Status / health**  
   - Extend `/api/status` with scheduler enabled?, interval, last run time/error if easy.  
   - Optional unauthenticated `/healthz` returning 200 if process up — only if useful for Phase 7; ask if unsure.

5. **Docs**  
   - Update README / PROGRESS / JUMPBACK / DEV-VERIFY notes for `RESYNC_INTERVAL_HOURS`.  
   - Leave full systemd/VPS to Phase 7; a short “what the interval does” is enough.

6. **Verify**  
   - `just test` + `just build`.  
   - Manual: interval `0` = quiet; small interval in DEV with one mapping = scheduled activity rows (operator can confirm).

---

## 6. Out of scope for this handoff

- Phase 7: systemd unit, VPS sync, SQLite backup guide, reverse proxy how-to  
- Docker  
- Multi-Google-account YouTube  
- Rewriting Admin UI stack (stay plain HTML/CSS/JS)

---

## 7. Recent relevant commits (orientation)

```text
2ca2682 Use floppy emoji for successful playlist adds.
…       DEV-VERIFY guide + verify pause docs
031e910 Add Phase 5 Admin Web UI and JSON API.
ccf91ec Add Phase 4 mapping CRUD and history resync.
```

Use `git log` for the full list.

---

## 8. Definition of done (Phase 6)

- [x] `RESYNC_INTERVAL_HOURS` actually drives background resync when > 0  
- [x] Interval 0 keeps today’s behavior (no scheduled scans)  
- [x] Shutdown stops scheduler without hanging  
- [x] Rate-limit / quota errors are clearer or retried sensibly  
- [x] Status (or health) reflects scheduler state enough for operators  
- [x] Docs updated; `just test` / `just build` green  
- [ ] Operator can re-check with [DEV-VERIFY.md](DEV-VERIFY.md) smoke (live link still 💾)

Then hand off toward **Phase 7** (deploy guide) — see [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md).

---

**Note:** Phase 6 shipped (scheduler + Admin polish). Default port is **50770**. Scheduler stays off until `RESYNC_INTERVAL_HOURS` is set. Next stop: native VPS deploy.

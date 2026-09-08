# Subotto Progress Report

**Date:** 2026-09-08  
**Branch:** `master`  
**Status:** Phase 6 **complete enough** for deploy planning. Phase 7 next — [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md).  
**Later:** Native Linux VPS / systemd / backup / optional reverse proxy  

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0–5 | Core bot + Admin | Verified live DEV |
| 6a | Scheduler + port 50770 | Shipped |
| 6b | Listeners + ops desk UI | Dark digicam Admin, public playlists, epochs, notices, DUPE/OLD reacts, channel names, START confirm |

## Key paths

```
cmd/subotto/main.go          Entry: run (bot + admin + scheduler) / auth / mapping CLI / resync
internal/scheduler/          Optional interval re-scans of enabled mappings
internal/web/                HTTP server, Basic Auth, JSON API
webroot/                     Admin UI (index.html, css/, js/)
internal/db/                 SQLite + mappings + activity + notice settings
internal/ingest/             Shared live + resync pipeline (SkippedSame / SkippedOld)
internal/discord/            Bot + REST resync + catalog + notices
internal/youtube/            OAuth client; CreatePlaylist → public
justfile                     run, build, build-linux, auth-youtube, listener CRUD, resync
docs/DEV-VERIFY.md           DEV startup guide
docs/HANDOFF-PHASE7.md       Next agent: Phase 7 deploy
```

## Verified — code

- `just test` — parser, youtube errors, db mappings, ingest, web API, scheduler, announce  
- `just build` — Windows binary builds  
- `just build-linux` — recipe ready for PROD binary  

## Verified — live DEV (Meat Bag sign-off, 2026-09-07+)

Acceptance from [DEV-VERIFY.md](DEV-VERIFY.md) for Phases 0–5, plus Phase 6 product use:

- [x] Discord bot token + **Message Content Intent** + bot invited  
- [x] Google Cloud: YouTube Data API v3 + OAuth client + redirect + test user  
- [x] Real `ADMIN_PASSWORD` in `.env`  
- [x] `just auth-youtube` once  
- [x] `just run` → Admin UI → listener → paste YouTube link → **💾** + playlist update  
- [x] Reactions work (Add Reactions permission)  
- [x] Phase 6 Admin polish (listeners, notices, digicam, public playlists) in daily use  

Phase 6 live check (optional): set a small `RESYNC_INTERVAL_HOURS` in DEV and confirm scheduled activity rows; leave at `0` for normal use.

Default Admin/OAuth port is **50770**.

## Not done yet (PLAN.md)

- **Phase 7** — Linux VPS / systemd deploy guide → [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)  
- Optional leftover: Admin notice **preview** before save (only if Meat Bag asks)  

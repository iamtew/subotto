# Subotto Progress Report

**Date:** 2026-09-08  
**Branch:** `master`  
**Status:** Phase 7 **shipped** — Linux package + deploy guide.  
**Guide:** [DEPLOY.md](DEPLOY.md) · prior brief [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)

---

## What shipped

| Phase | Title | Result |
|--------|--------|--------|
| 0–5 | Core bot + Admin | Verified live DEV |
| 6a | Scheduler + port 50770 | Shipped |
| 6b | Listeners + ops desk UI | Dark digicam Admin, public playlists, epochs, notices, DUPE/OLD reacts, channel names, START confirm |
| 7 | Linux package / deploy | `just package-linux`, sample systemd, headless + SSH-tunnel docs |

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
justfile                     run, build, build-linux, package-linux, auth-youtube, listener CRUD, resync
scripts/package-linux.ps1    Zip staging for dist/subotto-linux.zip
deploy/subotto.service       Sample systemd unit
docs/DEPLOY.md               PROD cutover, SSH-tunnel YouTube auth, backup
docs/DEV-VERIFY.md           DEV startup guide
```

## Verified — code

- `just test` — parser, youtube errors, db mappings, ingest, web API, scheduler, announce  
- `just build` — Windows binary builds  
- `just build-linux` / `just package-linux` — Linux binary + `dist/subotto-linux.zip`  

## Verified — live DEV (operator sign-off, 2026-09-07+)

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

## PROD checklist

Follow [DEPLOY.md](DEPLOY.md): package → copy `.env` + `data/subotto.db` → stop DEV → run on VPS → optional systemd/Caddy.

## Optional leftovers (not blocking)

- Admin notice **preview** before save (only if requested)  
- Public `/healthz` without Basic Auth  

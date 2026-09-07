# Subotto Progress Report

**Date:** 2026-09-07  
**Branch:** `master`  
**Status:** Phases **0–5 complete** and **Meat Bag verified good** (live Discord + YouTube DEV).  
**Next:** Phase 6 — [HANDOFF-PHASE6.md](HANDOFF-PHASE6.md)  
**Later:** Phase 7 Linux VPS / systemd deploy guide

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog` |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist` |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions |
| 4 | Mapping CRUD + resync | List/enable/disable/delete CLI, shared ingest, history resync |
| 5 | Admin Web UI | stdlib HTTP, Basic Auth, `/api/*`, plain HTML/CSS/JS dashboard |

## Key paths

```
cmd/subotto/main.go          Entry: run (bot + admin) / auth / mapping CLI / resync
internal/web/                HTTP server, Basic Auth, JSON API
webroot/                     Admin UI (index.html, css/, js/)
internal/db/                 SQLite + mappings + activity list helpers
internal/ingest/             Shared live + resync pipeline
internal/discord/            Bot + REST resync (success react = 💾)
justfile                     run, build, auth-youtube, mapping CRUD, resync
docs/DEV-VERIFY.md           DEV startup guide
docs/HANDOFF-PHASE6.md       Next-agent brief for Phase 6
```

## Verified — code

- `just test` — parser, youtube errors, db mappings, ingest, web API  
- `just build` — Windows binary builds  

## Verified — live DEV (Meat Bag sign-off, 2026-09-07)

Acceptance from [DEV-VERIFY.md](DEV-VERIFY.md):

- [x] Discord bot token + **Message Content Intent** + bot invited  
- [x] Google Cloud: YouTube Data API v3 + OAuth client + redirect + test user  
- [x] Real `ADMIN_PASSWORD` in `.env`  
- [x] `just auth-youtube` once  
- [x] `just run` → Admin UI → mapping → paste YouTube link → **💾** + playlist update  
- [x] Reactions work (Add Reactions permission); failures visible at Warn if they recur  

Known DEV gotchas learned this arc (documented in DEV-VERIFY):

- Gateway **4014** → Message Content Intent not enabled/saved  
- OAuth “testing” mode → add yourself as a **test user**  
- Playlist OK but no emoji → Add Reactions permission; restart after fixes  

## Not done yet (PLAN.md)

- **Phase 6** — scheduler (`RESYNC_INTERVAL_HOURS`), rate-limit handling, polish → [HANDOFF-PHASE6.md](HANDOFF-PHASE6.md)  
- **Phase 7** — Linux VPS / systemd deploy guide  

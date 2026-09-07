# Subotto Progress Report

**Date:** 2026-09-07  
**Branch:** `master`  
**Status:** Phases **0–5 complete** in code. **Phase work paused** — Meat Bag verifying DEV with live APIs.  
**How to verify:** [DEV-VERIFY.md](DEV-VERIFY.md)  
**Later (not now):** Phase 6 scheduler/polish · Phase 7 Linux deploy guide

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot placeholder |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog`, startup activity row |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist`, quota/auth errors |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions, `just add-mapping` |
| 4 | Mapping CRUD + resync | List/enable/disable/delete CLI, shared ingest, history resync |
| 5 | Admin Web UI | stdlib HTTP, Basic Auth, `/api/*`, plain HTML/CSS/JS dashboard |

## Key paths

```
cmd/subotto/main.go          Entry: run (bot + admin) / auth / mapping CLI / resync
internal/web/                HTTP server, Basic Auth, JSON API
webroot/                     Admin UI (index.html, css/, js/)
internal/db/                 SQLite + mappings + activity list helpers
internal/ingest/             Shared live + resync pipeline
internal/discord/            Bot + REST resync
justfile                     run, build, auth-youtube, mapping CRUD, resync
docs/DEV-VERIFY.md           Meat Bag DEV startup + acceptance checklist
```

## Verified in CI-ish / local code checks

- `just test` — parser, youtube errors, db mappings, ingest, web API
- `just build` — Windows binary builds

## Live DEV verification (Meat Bag — in progress)

Follow [DEV-VERIFY.md](DEV-VERIFY.md). Acceptance = one ✅ on a fresh link + video on the playlist + activity visible in Admin UI.

- [ ] Discord bot token + **Message Content Intent** + bot invited  
- [ ] Google Cloud: YouTube Data API v3 + OAuth client + redirect  
- [ ] Real `ADMIN_PASSWORD` in `.env`  
- [ ] `just auth-youtube` once  
- [ ] `just run` → Admin UI → mapping → paste YouTube link → ✅  

## Paused (PLAN.md)

- **Phase 6** — scheduler (`RESYNC_INTERVAL_HOURS`), rate-limit handling, polish  
- **Phase 7** — Linux VPS / systemd deploy guide  

Resume those only after DEV verification (or if Meat Bag explicitly redirects).

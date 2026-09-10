# Subotto Progress Report

**Date:** 2026-09-10  
**Branch:** `master`  
**Status:** Phase 8 **code complete + polished locally** — not pushed. See [HANDOFF-PHASE8.md](HANDOFF-PHASE8.md).  
**Guide:** [DEPLOY.md](DEPLOY.md) · jump-back [JUMPBACK.md](JUMPBACK.md)

---

## What shipped

| Phase | Title | Result |
|--------|--------|--------|
| 0–5 | Core bot + Admin | Verified live DEV |
| 6a | Scheduler + port 50770 | Shipped |
| 6b | Content listeners + ops desk UI | Dark digicam Admin, public playlists, epochs, notices, DUPE/OLD reacts |
| 7 | Linux package / deploy | `just package-linux`, sample systemd, headless + SSH-tunnel docs |
| 8 | Picture listeners + OBS slideshow | Parallel picture epochs, `data/pictures/`, public `/slideshow/{slug\|latest}`, Admin tabs |

**Also:** public Streamer.bot GETs — `/api/get/content/{channel}` and `/api/get/picture/{channel}` (no auth; open epoch only).

## Key paths

```
cmd/subotto/main.go          Entry: run / auth / content + picture CLI / resync
internal/pictures/           Discord attachment download + save
internal/db/pictures.go      picture_listeners + collected_pictures
internal/web/slideshow.go    Public slideshow + media (no Basic Auth)
internal/web/get_api.go      Public /api/get/{content|picture}/{channel} (Streamer.bot)
internal/web/picture_api.go  Admin /api/picture-listens
webroot/slideshow/           OBS overlay HTML/CSS/JS
webroot/                     Admin UI (tabs: content | pictures)
data/pictures/{slug}/        Saved images (next to DATABASE_PATH)
```

## Verified — code

- `just test` — parser, youtube, db (mappings + pictures), ingest, pictures package, web API (incl. public slideshow + `/api/get`), scheduler, announce  
- `just build` — Windows binary builds  

## Operator sign-off still needed (Phase 8 live)

- [ ] Admin tabs: Content listeners + Picture listeners
- [ ] Start picture listener → post image in Discord → **💾** + file under `data/pictures/{slug}/`
- [ ] Open `/slideshow/{slug}` (and `/slideshow/latest`) in browser / OBS — transparent bg, credit corner, floaters (no bot 💾/DUPE/OLD/❌)
- [ ] Same channel: content + picture listeners both live
- [ ] Picture DUPE / OLD chrome matches content when re-ingest / previous epoch hits

## Parked (not Phase 8)

- **Shows** — bi-weekly listeners, Discord announce-as-show-runner, richer scheduling
- Deeper overlay polish (fonts, transitions, emoji filters)

## PROD checklist

Follow [DEPLOY.md](DEPLOY.md): package → copy `.env` + `data/` (DB **and** `pictures/`) → run on VPS → optional Caddy for public slideshow TLS.

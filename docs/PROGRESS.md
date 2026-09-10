# Subotto Progress Report

**Date:** 2026-09-10  
**Branch:** `master`  
**Status:** Phase 8 + **Show Episodes (medium)** in code — not pushed. See [HANDOFF-PHASE8.md](HANDOFF-PHASE8.md).  
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
| Episodes | Show episodes (medium) | Templates → start/cease group; public `/api/get/episode/{show}`; Admin Episodes tab |

**Also:** public Streamer.bot GETs — `/api/get/content/{channel}`, `/api/get/picture/{channel}`, `/api/get/episode/{show}` (no auth; live episode / open epoch only).

## Key paths

```
cmd/subotto/main.go          Entry: run / auth / content + picture CLI / resync
internal/pictures/           Discord attachment download + save
internal/db/pictures.go      picture_listeners + collected_pictures
internal/db/episodes.go      episodes + episode_templates
internal/web/slideshow.go    Public slideshow + media (no Basic Auth)
internal/web/get_api.go      Public /api/get/{content|picture}/{channel} + /api/get/episode/{show}
internal/web/picture_api.go  Admin /api/picture-listens
internal/web/episode_api.go  Admin /api/episodes + /api/episode-templates
webroot/slideshow/           OBS overlay HTML/CSS/JS
webroot/                     Admin UI (tabs: content | pictures | episodes)
data/pictures/{slug}/        Saved images (next to DATABASE_PATH)
```

## Verified — code

- `just test` — parser, youtube, db (mappings + pictures + episodes), ingest, pictures package, web API (incl. public slideshow + `/api/get` + episodes), scheduler, announce  
- `just build` — Windows binary builds  

## Operator sign-off still needed (Phase 8 live)

- [ ] Admin tabs: Content listeners + Picture listeners
- [ ] Start picture listener → post image in Discord → **💾** + file under `data/pictures/{slug}/`
- [ ] Open `/slideshow/{slug}` (and `/slideshow/latest`) in browser / OBS — transparent bg, credit corner, floaters (no bot 💾/DUPE/OLD/❌)
- [ ] Same channel: content + picture listeners both live
- [ ] Picture DUPE / OLD chrome matches content when re-ingest / previous epoch hits

## Episodes — operator check

- [ ] Admin **Episodes** tab: save a template (show + listener stubs JSON)
- [ ] Start episode from template → linked content/picture listeners online
- [ ] **Absorb:** after PROD deploy/restart with live orphan listeners → ABSORB LIVE LISTENERS (keeps epochs, sets episode_id)
- [ ] `GET /api/get/episode/{show_slug}` returns fields + listeners (Streamer.bot)
- [ ] Cease episode → all linked listeners cease

## Parked

- Bi-weekly show-runner / Discord announce-as-show-runner / richer scheduling
- Deeper overlay polish (fonts, transitions, emoji filters)

## PROD checklist

Follow [DEPLOY.md](DEPLOY.md): package → copy `.env` + `data/` (DB **and** `pictures/`) → run on VPS → optional Caddy for public slideshow TLS.

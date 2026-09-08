# Handoff — Phase 8 picture listeners + OBS slideshow polish

**Status:** Phase 8 **shipped in code**, polished locally, **not pushed**.  
**For:** next Cursor agent / Meat Bag  
**Date:** 2026-09-08  
**Branch:** `master` — **ahead of `origin/master` by 2 commits** (do not push unless asked)

Suggested opener for the new chat:

> Read `docs/HANDOFF-PHASE8.md`, `docs/JUMPBACK.md`, and `AGENTS.md`. Continue slideshow / picture-listener polish with me.

Also feed: [AGENTS.md](../AGENTS.md) · [PLAN.md](../PLAN.md) · [PROGRESS.md](PROGRESS.md)

---

## 1. Git state (important)

| Commit | Title |
|--------|--------|
| `ffec62a` | Ship Phase 8 content and picture listeners with OBS slideshow |
| `86a66f3` | Polish picture slideshow overlay and Admin settings |

Working tree should be **clean** after `86a66f3`. Nothing pushed yet.

**Do not force-push. Commit only when asked. Push only when Meat Bag says so (or “ship it”).**

---

## 2. What Phase 8 is

### Product language
- **Content listener** — Discord channel → YouTube playlist (old “Listener”; DB table still `channel_mappings`)
- **Picture listener** — Discord channel → `data/pictures/{slug}/` + public OBS slideshow
- **Both may be live on the same channel at once** (one of each type)

### Public vs Admin HTTP
Same Go server (default port **50770**):

| Route | Auth |
|-------|------|
| Admin `/`, `/api/*` (listens, picture-listens, resync, …) | Basic Auth `admin` / `ADMIN_PASSWORD` |
| `GET /slideshow/latest`, `/slideshow/{slug}` | **Public** |
| `GET /api/slideshow/{slug}` | **Public** feed JSON |
| `GET /media/pictures/{slug}/{file}` | **Public** images |

### Key paths
```
internal/db/pictures.go          picture_listeners + collected_pictures CRUD
internal/pictures/               download Discord attachments to disk
internal/discord/bot.go          content + picture MessageCreate; reaction sync
internal/discord/picture_resync.go
internal/web/slideshow.go        public routes
internal/web/picture_api.go      /api/picture-listens + /api/picture-resync
webroot/index.html + js/admin.js Admin tabs (Content | Picture)
webroot/slideshow/               OBS overlay HTML/CSS/JS
data/pictures/{slug}/            saved images (next to DATABASE_PATH)
```

### Admin UX
- Expandable tabs via `TAB_REGISTRY` in `webroot/js/admin.js`
- Picture **start form** is bare minimum: name, server, channel, enabled
- **Settings** panel (per live picture listener): corner, advance, shuffle, author credit, reactions, reaction render 1–25x, author size, emoji size
- Picture **resync** (REST history; stamps missing 💾 / DUPE / OLD / ❌) — Admin + `just resync-pictures`

### Slideshow behavior (current)
- Transparent bg; image `object-fit: contain`
- Frosted Author card: `Author: NAME`
- Floaters = reaction sprites (Twemoji unicode/flags; Discord CDN for custom `name:id` / `a:name:id`)
- Pop-in + fade-out lifecycle; stage z-index **above** author card
- Floater playground size scales with reaction multiplier (1x local zone → 25x full viewport)
- Top corners: floaters above + below author; bottom corners: mostly above
- Default author/emoji size multipliers: **1.5**
- Feed poll every 5s; advance timer must **not** reset on poll (`ensureAdvanceTimer`)

### Discord reacts
- Content + Picture (same chrome): 💾 / ♻️ DUPE / 🛑 OLD / ❌  
- **Stamping is fundamental** — live ingest **and** content/picture resync share one policy. Resync fills in missing chrome (original saved post → 💾, not DUPE).  
- Floaters (slideshow): other users’ reacts only — Subotto’s status chrome is excluded via Discord `Me`  
- Intents: Guilds + GuildMessages + MessageContent + **GuildMessageReactions**

### Ops note
Stop **PROD** if DEV uses the **same Discord bot token** (both would ingest the same messages). Keys can be copied; concurrent gateway sessions cannot.

---

## 3. What was just polished (commit `86a66f3`)

- Empty “waiting for pictures…” stuck over images — CSS `[hidden]` vs `display:grid`
- Slideshow never advancing — feed poll reset the advance interval
- Settings prompts → real panel
- Frosted credit; Twemoji + Discord CDN; no count digits on floaters
- Reaction render multiplier; credit_scale / reaction_scale columns
- Slim start form; floater stage layout; z-index floaters above card

---

## 4. Likely next polish (operator returning)

Pick up with Meat Bag — do not invent a huge redesign:

- Floater orbit feel / density / corner bias still may need tweaks after live OBS eyeballing
- Animated custom emoji keys need fresh reacts after `a:name:id` storage change (old rows may be `name:id` only — still works as static CDN)
- Picture history resync of **floater** reactions is snapshot-at-scan, not live gateway (status chrome is stamped)
- Package/deploy docs already mention `data/pictures/` — verify on next PROD cutover
- **Parked (not this arc):** **shows** (bi-weekly schedules, richer Discord announce / show-runner)

---

## 5. Verify before more UI churn

```text
just test
just build
just run   # stop PROD first if same bot token
```

Live checklist:
1. Admin tabs: Content | Picture  
2. Start picture listener → post image → 💾 → file under `data/pictures/{slug}/`  
3. `/slideshow/{slug}` in browser/OBS — advance, Author credit, floaters on top (no bot status chrome)  
4. Settings save → feed picks up scales / multiplier without full restart (hard-refresh overlay)  
5. Optional: `just resync-pictures CHANNEL`

---

## 6. Conventions reminder

- Beginner-friendly comments; Just, no Docker  
- Commit style: clean title + bullet body; only when asked  
- “ship it” = commit + push (no force-push unless explicit)  
- Individual agency / SIGINT vibe in copy — operator owns the wire  

Welcome back, Meat Bag. Clanker stands ready in the next tab.

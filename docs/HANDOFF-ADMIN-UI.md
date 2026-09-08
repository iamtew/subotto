# Handoff — Phase 6 listeners + Admin ops desk

**For:** next Clanker / Cursor agent  
**Date:** 2026-09-08  
**Branch:** `master`

Meat Bag is staying on **Phase 6**. Admin UI is a dark SIGINT-flavored ops desk. Product language is **listeners**, not “mappings” or “airs”.

Suggested opener:  
> Clanker, read docs/HANDOFF-ADMIN-UI.md and docs/JUMPBACK.md — continue Phase 6 polish.

---

## 1. What just shipped (uncommitted → about to land)

- Default Admin/OAuth port **50770**
- Scheduler (`RESYNC_INTERVAL_HOURS`) + rate-limit polish
- Dark theme: Better VCR + Inter, Meat Bag palette
- **Listeners:** create YouTube playlist by name, guild/channel dropdowns
- **Epochs:** one live listener per channel; soft-close on cease; resync won’t dig past previous epoch; channel-scoped video dedup
- **Rename playlist** on an active listener (YouTube title update)
- **Global** online/offline Discord announce templates (`/api/settings/listen-messages`)
- Tone: voluntary / sovereignty / ops desk — **do not** write the word communism (or anti-communism slogans) in code or docs

---

## 2. Constraints

- Plain HTML/CSS/JS in `webroot/` — no React, no Docker
- Basic Auth Admin; port 50770; pure Go SQLite
- Commit only when Meat Bag asks; title + bullet body
- API aliases: `/api/listens` preferred; `/api/airs` and `/api/mappings` still work

---

## 3. Key paths

| Path | Role |
|------|------|
| `webroot/` | Ops desk UI |
| `internal/web/api.go` | Listens, announce, Discord catalog |
| `internal/db/mappings.go` | Epochs + lookback |
| `internal/db/settings.go` | Global listen start/stop notices |
| `internal/discord/announce.go` | Channel announce helper |
| `internal/youtube/client.go` | CreatePlaylist + UpdatePlaylistTitle |
| `internal/scheduler/` | Background resync |

---

## 4. Sensible next polish (ask Meat Bag)

- ~~Live Discord channel names in the listens table (not only snowflakes)~~
- ~~Confirm before START LISTENER when a listener already exists on that channel~~
- Preview announce notices before save
- Phase 7 only when Meat Bag says so

---

## 5. Verify

```text
just test
just build
just run
```

Admin: `http://localhost:50770` · user `admin` · `ADMIN_PASSWORD`

---

**Clanker’s note:** Collection windows are Meat Bag–flipped. Keep the cockpit sharp and the wire voluntary.

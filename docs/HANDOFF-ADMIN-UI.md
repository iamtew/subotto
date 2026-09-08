# Handoff — Phase 6 listeners + Admin ops desk

**For:** historical context / Phase 6 polish only  
**Date:** 2026-09-08  
**Branch:** `master`

**Status:** Phase 6 product work is **complete enough**. Next stop is **Phase 7 deploy** — use [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md) and [JUMPBACK.md](JUMPBACK.md).

Suggested opener (Phase 6 leftovers only):  
> Read docs/HANDOFF-ADMIN-UI.md — optional Phase 6 polish (e.g. notice preview).

Suggested opener (deploy):  
> Read docs/HANDOFF-PHASE7.md and docs/JUMPBACK.md — plan Phase 7 deployment with me.

---

## 1. What shipped in Phase 6

- Default Admin/OAuth port **50770**
- Scheduler (`RESYNC_INTERVAL_HOURS`) + Discord/YouTube rate-limit retries
- Dark digicam Admin: Better VCR + Inter, woodland palette
- **Listeners:** public YouTube playlist by name, guild/channel dropdowns, `#channel` names
- **Epochs:** one live listener per channel; soft-close on cease; resync epoch floor; channel-scoped dedup
- START confirm when replacing a live listener
- **Notices:** global ONLINE/OFFLINE Discord templates (`/api/settings/listen-messages`)
- Reactions: 💾 / ♻️DUPE / 🛑OLD / ❌
- Tone: voluntary / operator-owned / ops desk — **do not** write the word communism (or anti-communism slogans) in code or docs

---

## 2. Constraints

- Plain HTML/CSS/JS in `webroot/` — no React, no Docker
- Basic Auth Admin; port 50770; pure Go SQLite
- Commit only when the maintainer asks; title + bullet body
- API aliases: `/api/listens` preferred; `/api/airs` and `/api/mappings` still work

---

## 3. Key paths

| Path | Role |
|------|------|
| `webroot/` | Ops desk UI |
| `internal/web/api.go` | Listens, notices, Discord catalog |
| `internal/db/mappings.go` | Epochs + lookback |
| `internal/db/settings.go` | Global listen start/stop notices |
| `internal/discord/announce.go` | Channel notice helper |
| `internal/youtube/client.go` | CreatePlaylist (public) + UpdatePlaylistTitle |
| `internal/scheduler/` | Background resync |

---

## 4. Optional leftover polish (ask first)

- Preview announce notices before save
- Otherwise prefer Phase 7 — [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)

---

## 5. Verify

```text
just test
just build
just run
```

Admin: `http://localhost:50770` · user `admin` · `ADMIN_PASSWORD`

---

**Note:** Collection windows are operator-flipped. Keep the Admin UI sharp and the wire voluntary.

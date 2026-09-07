# Subotto

**Discord channels → YouTube playlists, automatically.**

Written in Go. Designed for a human (Meat Bag) by an AI (Clanker).

See **PLAN.md** for the full architecture and phased implementation plan.  
See **AGENTS.md** for how Clanker and Meat Bag are supposed to talk to each other.

---

## Quick Status

**Phases 0–6a done** (scheduler + port 50770). **Phase 6 continuing:** Admin UI polish — [docs/HANDOFF-ADMIN-UI.md](docs/HANDOFF-ADMIN-UI.md).  
**Later:** Phase 7 (native Linux VPS / systemd).

DEV recipe: [docs/DEV-VERIFY.md](docs/DEV-VERIFY.md).

Short path once secrets exist:

```
just auth-youtube
just run
```

Open `http://localhost:50770` — username `admin`, password = `ADMIN_PASSWORD`.  
Paste a YouTube link in a mapped channel → expect 💾 (or ♻️ / ❌).

Optional: set `RESYNC_INTERVAL_HOURS` (e.g. `6`) for background history re-scans of enabled mappings. `0` keeps scheduler off.

Session notes: [docs/PROGRESS.md](docs/PROGRESS.md) · jump-back: [docs/JUMPBACK.md](docs/JUMPBACK.md)

**No Docker.** We use **Just** + native Go binaries.

---

## What Subotto Does

- Watches Discord text channels you configure.
- Detects YouTube video links.
- Adds those videos to the YouTube playlist mapped to that channel.
- Different channels can point to different playlists.
- Integrated Admin Web UI (plain HTML/CSS/JS served from `webroot/`).
- Optional re-scan / update of a channel’s history.

---

## Prerequisites (Meat Bag Checklist)

- [ ] Go 1.22+ installed
- [ ] Just installed (https://github.com/casey/just)
- [ ] Discord Application + Bot Token (Message Content Intent enabled)
- [ ] Google Cloud project with YouTube Data API v3 enabled
- [ ] OAuth 2.0 Client ID + Secret
- [ ] A Discord server where you can invite the bot
- [ ] YouTube playlists that the Google account you authorize owns (or let Admin UI create them)

Details and click-by-click steps: [docs/DEV-VERIFY.md](docs/DEV-VERIFY.md).

---

## High-Level Setup Flow

1. Copy `.env.example` → `.env` and fill in tokens (set a real `ADMIN_PASSWORD`).
2. Google Cloud: enable YouTube Data API v3, create OAuth client, add redirect `http://localhost:50770/oauth/callback`.
3. `just auth-youtube` once (browser login; refresh token saved in SQLite).
4. `just run` (or `just build` then run the binary).
5. Open the Admin UI (`http://localhost:50770`, user `admin` / `ADMIN_PASSWORD`), add a mapping.
6. Drop a YouTube link in that channel and confirm 💾 + playlist update.

Full walkthrough: [docs/DEV-VERIFY.md](docs/DEV-VERIFY.md).

---

## Development vs Production

|                  | DEV – Windows 10                  | PROD – Linux VPS (EU)                  |
|------------------|-----------------------------------|----------------------------------------|
| Build            | `just build` → `bin/subotto.exe`  | `just build-linux` → `bin/subotto-linux` |
| Run              | `just run`                        | Run the Linux binary (or systemd)      |
| Admin UI         | http://localhost:50770             | Same binary, optionally behind reverse proxy |
| Database         | SQLite in `./data`                | Same, keep the `data/` folder          |
| Web UI files     | `webroot/`                        | Copy `webroot/` next to the binary     |
| Secrets          | `.env` file                       | `.env` or environment variables        |

PROD deploy docs are **Phase 7** (paused until DEV verify is done).

---

## Philosophy

- No Docker. Native binaries only.
- **Just** as the classic build system.
- Plain HTML/CSS/JS in `webroot/` for the Admin UI.
- Prefer simple, readable Go over clever abstractions.
- Heavy comments written for a beginner.
- Configuration over code changes.

Clanker is ready. Verify Subotto the classic way, Meat Bag.

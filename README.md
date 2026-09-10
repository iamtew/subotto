# Subotto

**Discord channels → YouTube playlists, picture folders, and show episodes.**

A Go bot with a plain HTML/CSS/JS Admin UI. **No Docker** — Just + native binaries.

Conventions for Cursor agents: **[AGENTS.md](AGENTS.md)**.  
PROD cutover: **[docs/DEPLOY.md](docs/DEPLOY.md)**.

---

## Status

Shipped in code:

- **Content listeners** — Discord channel → YouTube playlist (start / cease)
- **Picture listeners** — Discord channel → on-disk images + public OBS `/slideshow/...`
- **Show episodes** — templates + start/cease + absorb live listeners + public `/api/get/episode/{show}`

Parked: bi-weekly schedules, richer show-runner Discord announce.

---

## What Subotto Does

- Watches Discord text channels you configure.
- **Content:** detects YouTube links and adds them to that channel’s playlist.
- **Pictures:** saves image attachments under `data/pictures/` and serves an OBS slideshow.
- Both listener types may be live on the **same channel** at once.
- **Episodes** group content + picture listeners for a show (Admin tab + Streamer.bot GET).
- Admin UI from `webroot/`; optional background history re-scan via `RESYNC_INTERVAL_HOURS`.

Reactions on ingest: **💾** added · **♻️** DUPE · **🛑** OLD · **❌** failed.

---

## Prerequisites

- Go 1.22+ and [Just](https://github.com/casey/just)
- Discord Application + Bot Token (**Message Content Intent** enabled)
- Google Cloud project with YouTube Data API v3 + OAuth 2.0 Client ID/Secret
- A Discord server where you can invite the bot
- YouTube playlists owned by the Google account you authorize (or create them in Admin)

---

## DEV setup (Windows)

1. Copy `.env.example` → `.env` and fill in tokens (set a real `ADMIN_PASSWORD`).
2. Google Cloud: enable YouTube Data API v3, create OAuth client, add redirect  
   `http://localhost:50770/oauth/callback`.
3. `just auth-youtube` once (browser login; refresh token saved in SQLite).
4. `just run` (do **not** run auth and the bot on port **50770** at the same time).
5. Open `http://localhost:50770` — user `admin`, password = `ADMIN_PASSWORD`.
6. Start a content listener, paste a YouTube link → expect **💾** + playlist update.

Short path once secrets exist:

```
just auth-youtube
just run
```

Optional: `RESYNC_INTERVAL_HOURS=6` for background re-scans of enabled listeners (`0` = off).

---

## Development vs Production

|                  | DEV – Windows 10                  | PROD – Linux VPS (EU)                  |
|------------------|-----------------------------------|----------------------------------------|
| Build            | `just build` → `bin/subotto.exe`  | `just build-linux` → `bin/subotto-linux` |
| Package          | —                                 | `just package-linux` → `dist/subotto-linux.zip` |
| Run              | `just run`                        | Linux binary or systemd                |
| Admin UI         | http://localhost:50770            | Same binary, optionally behind Caddy   |
| Database         | SQLite in `./data`                | Copy `data/` (DB **and** `pictures/`)  |
| Web UI files     | `webroot/`                        | Next to the binary                     |
| Secrets          | `.env`                            | `.env` or environment variables        |

Full PROD walkthrough (headless cutover, SSH-tunnel re-auth, backup): [docs/DEPLOY.md](docs/DEPLOY.md).

---

## Philosophy

- No Docker. Native binaries only.
- **Just** as the classic build system.
- Plain HTML/CSS/JS in `webroot/` for the Admin UI.
- Prefer simple, readable Go over clever abstractions.
- Heavy comments written for a beginner.
- Configuration over code changes.

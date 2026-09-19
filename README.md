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
- **Show episodes** — templates + start/cease + absorb live listeners + public `/api/get/episode/{show}` (`air_datetime`, `spot_image`, `stream_background` + host URL)
- **Broadcasts** — named multi-channel Discord sends (Admin Fire or `GET /api/broadcasts/{slug}/fire` with Basic Auth `api` / `API_PASSWORD`)
- **Admin login** — public landing at `/`; Discord OAuth (needs `SUPERADMIN_DISCORD_ID` + Discord app client id/secret) and optional Basic `admin` / `ADMIN_PASSWORD`
- **Hesh Helper** — Discord mention / reply-to-bot chat via OpenRouter (Admin **AI** tab for the model, system prompt, sampling sliders, optional channel memory, and test chat)

Parked: bi-weekly schedules, richer show-runner Discord announce.

---

## What Subotto Does

- Watches Discord text channels you configure.
- **Content:** detects YouTube links and adds them to that channel’s playlist.
- **Pictures:** saves image attachments under `data/pictures/` and serves an OBS slideshow.
- Both listener types may be live on the **same channel** at once.
- **Episodes** group content + picture listeners for a show (Admin tab + Streamer.bot GET). Optional **air datetime** (RFC3339 with offset, e.g. `2026-09-20T20:00:00+02:00`) and **spot image** live on the episode (`data/episode-spots/`, public `/media/episodes/{id}/…`). **Stream background** is one global still (`data/stream-background/latest`, 15 MB). OBS Browser Source: `/stream-background` (picks up a new upload in a couple of seconds). Raw image: `/media/stream-background/latest`.
- Admin UI from `webroot/`; background history re-scan from the **Scheduler** tab (`RESYNC_INTERVAL_HOURS` is only a bootstrap until you save there).

Reactions on ingest: **💾** added · **♻️** DUPE · **🛑** OLD · **❌** failed.

---

## Prerequisites

- Go 1.22+ and [Just](https://github.com/casey/just)
- Discord Application + Bot Token (**Message Content Intent** enabled). For Admin Discord login: same app’s **OAuth2** client id/secret, redirect `http://localhost:50770/auth/discord/callback`, scope **identify**, and `SUPERADMIN_DISCORD_ID` (your Discord user id)
- Google Cloud project with YouTube Data API v3 + OAuth 2.0 Client ID/Secret
- A Discord server where you can invite the bot
- YouTube playlists owned by the Google account you authorize (or create them in Admin)

---

## DEV setup (Windows)

1. Copy `.env.example` → `.env` and fill in tokens. Set `SUPERADMIN_DISCORD_ID` plus `DISCORD_CLIENT_ID` / `DISCORD_CLIENT_SECRET` for Discord login. Set a real `ADMIN_PASSWORD` for Basic fallback, or `ADMIN_PASSWORD=` to turn Basic off. Optional `API_PASSWORD` for Streamer.bot broadcast fire; optional `OPENROUTER_API_KEY` for Hesh Helper.
2. Google Cloud: enable YouTube Data API v3, create OAuth client, add redirect  
   `http://localhost:50770/oauth/callback`.
3. Discord Developer Portal: add redirect `http://localhost:50770/auth/discord/callback` (OAuth2, identify).
4. `just auth-youtube` once (browser login; refresh token saved in SQLite). After that, Admin **Status → Authorize YouTube** can re-auth without stopping the bot.
5. `just run` (do **not** run auth and the bot on port **50770** at the same time).
6. Open `http://localhost:50770` — Continue with Discord, or Password login (`admin` / `ADMIN_PASSWORD`) if Basic is on. Admin UI is `/admin`.
7. Start a content listener, paste a YouTube link → expect **💾** + playlist update.

Short path once secrets exist:

```
just auth-youtube
just run
```

Optional: `RESYNC_INTERVAL_HOURS=6` until you save the Admin **Scheduler** tab (`0` = off). After the first save, interval / amount / listener list live in SQLite.

---

## Development vs Production

|                  | DEV – Windows 10                  | PROD – Linux VPS (EU)                  |
|------------------|-----------------------------------|----------------------------------------|
| Build            | `just build` → `bin/subotto.exe`  | `just build-linux` → `bin/subotto-linux` |
| Package          | —                                 | `just package-linux` → `dist/subotto-linux.zip` |
| Run              | `just run`                        | Linux binary or systemd                |
| Admin UI         | http://localhost:50770/admin      | Same binary, optionally behind Caddy   |
| Database         | SQLite in `./data`                | Copy `data/` (DB **and** `pictures/` / `episode-spots/` / `stream-background/`)  |
| Web UI files     | `webroot/`                        | Next to the binary                     |
| Secrets          | `.env`                            | `.env` or environment variables        |

Full PROD walkthrough (headless cutover, Admin YouTube re-auth, backup): [docs/DEPLOY.md](docs/DEPLOY.md).

---

## Philosophy

- No Docker. Native binaries only.
- **Just** as the classic build system.
- Plain HTML/CSS/JS in `webroot/` for the Admin UI.
- Prefer simple, readable Go over clever abstractions.
- Heavy comments written for a beginner.
- Configuration over code changes.

# Get Subotto Running (DEV on Windows)

**Audience:** Meat Bag (you)  
**Goal:** Run Subotto in DEV and confirm Phases 0–5 still work.  
**Status:** Meat Bag verified good (2026-09-07). Use this anytime you need to re-check.  
**Machine:** Windows 10, repo root, **no Docker** — Go + Just only.

Clanker wrote this so you can follow it like a recipe. When something goes boom, jump to [Troubleshooting](#troubleshooting).

---

## What you are verifying

Subotto does this loop:

```text
Someone posts a YouTube link in a mapped Discord channel
  → Subotto reads the message
  → pulls out the video ID
  → adds it to that channel’s YouTube playlist
  → remembers it (so it does not add twice)
  → reacts 💾 (added), ♻️ + DUPE (same listener), 🛑 + OLD (previous listener), or ❌ (failed)
```

You also get:

- An **Admin UI** in the browser (manage mappings, see activity, resync history)
- **CLI** helpers via `just …` if you prefer the terminal

Phases **6–7** come after this guide. **Meat Bag verified DEV good (2026-09-07)** — keep this recipe for re-checks and regressions.

---

## 0. Mental model (plain English)

| Piece          | What it is                               | Where it lives                          |
| -------------- | ---------------------------------------- | --------------------------------------- |
| Config         | Secrets and settings                     | `.env` (never commit this)              |
| Database       | Mappings, dedup, activity, YouTube token | `./data/subotto.db`                     |
| Discord bot    | Listens for new messages                 | Same process as `just run`              |
| YouTube client | Adds videos using *your* Google login    | OAuth refresh token in the DB           |
| Admin UI       | Browser dashboard                        | `http://localhost:50770` from `webroot/` |

**Important:** Adding to a playlist needs **OAuth**, not just a Google API key. That is why you run `just auth-youtube` once.

**Port 50770:** used by `just auth-youtube` for a short callback, then by the Admin UI while the bot runs. Do one at a time; do not run auth and the bot on 50770 together.

---

## 1. Install tools (once)

1. **Go 1.22+** — [https://go.dev/dl/](https://go.dev/dl/)  
   Check: `go version`
2. **Just** — [https://github.com/casey/just#installation](https://github.com/casey/just#installation)  
   Check: `just --list` (run from the Subotto repo folder)

Optional sanity (no Discord needed):

```text
just test
just build
```

Both should succeed. That proves the code Clanker shipped compiles and unit-tests pass.

---

## 2. Create your `.env`

From the repo root:

```text
copy .env.example .env
```

Open `.env` in an editor. You will fill tokens in the steps below. At minimum change:

```env
ADMIN_PASSWORD=pick-something-you-will-remember
```

Leave `ADMIN_PORT=50770` and `ADMIN_HOST=0.0.0.0` unless you know you need otherwise.  
`DATABASE_PATH=./data/subotto.db` is fine for DEV.

`RESYNC_INTERVAL_HOURS=0` (default) means **no** background history scans — live Discord posts still work. Set a positive number (hours) only if you want Subotto to periodically re-scan enabled channels the same way Admin “Resync” does. Keep it conservative; YouTube playlist inserts cost quota.

`.env` is gitignored. Do not paste secrets into Discord, commits, or chat logs.

---

## 3. Discord bot setup

### 3.1 Create the application

1. Open [Discord Developer Portal](https://discord.com/developers/applications) → **New Application** → name it (e.g. Subotto).
2. Left sidebar → **Bot** → **Add Bot** (if needed).
3. Under **Token** → **Reset Token** / **Copy** → paste into `.env` as `DISCORD_BOT_TOKEN=...`.

### 3.2 Message Content Intent (required)

Still on the **Bot** page:

- Enable **Message Content Intent**
- Save changes

Without this, Discord will not send Subotto the text of messages, so YouTube links are invisible.

### 3.3 Invite the bot to your server

1. Left sidebar → **OAuth2** → **URL Generator**
2. Scopes: **`bot`**
3. Bot permissions (minimum that works):
   - **View Channels**
   - **Read Message History** (needed for resync)
   - **Add Reactions** (for 💾 ♻️ 🛑 letter-spells ❌)
   - **Send Messages** is nice-to-have; Subotto mainly reacts
4. Copy the generated URL, open it, pick your test server, authorize.

### 3.4 Optional guild filter

If the bot is in many servers and you only want one, set in `.env`:

```env
DISCORD_GUILD_ID=your-server-id
```

Leave empty to allow every server the bot is in.

### 3.5 Get a channel ID

1. Discord → User Settings → **Advanced** → enable **Developer Mode**
2. Right-click the text channel you want → **Copy Channel ID**  
   Keep this for the mapping step.

---

## 4. Google / YouTube setup

You need a Google account that **owns** (or can edit) the playlists Subotto will write to.

### 4.1 Project + API

1. [Google Cloud Console](https://console.cloud.google.com/) → create/select a project.
2. **APIs & Services** → **Enable APIs** → enable **YouTube Data API v3**.

### 4.2 OAuth client

1. **APIs & Services** → **Credentials** → **Create credentials** → **OAuth client ID**
2. If asked, configure the OAuth consent screen (External is fine for personal DEV; add yourself as a test user).
3. Application type: **Web application** (works with our localhost callback).
4. **Authorized redirect URIs** — add exactly:

   ```text
   http://localhost:50770/oauth/callback
   ```

5. Copy **Client ID** and **Client secret** into `.env`:

   ```env
   YOUTUBE_CLIENT_ID=....apps.googleusercontent.com
   YOUTUBE_CLIENT_SECRET=...
   YOUTUBE_REDIRECT_URL=http://localhost:50770/oauth/callback
   ```

### 4.3 Playlist

Admin UI can **create** a public playlist from a name when you save a mapping.  
You no longer need to invent an empty playlist by hand on YouTube.

(Optional) If you already have a playlist ID (`list=` in the URL, looks like `PLxxxxxxxx…`), CLI `just add-mapping` still accepts it.

---

## 5. One-time YouTube login

From the **repo root**, with nothing else using port 50770:

```text
just auth-youtube
```

What happens:

1. Subotto starts a tiny temporary HTTP listener on 50770.
2. Your browser opens Google’s consent screen.
3. Sign in with the Google account that owns the playlist.
4. Approve access to manage YouTube (playlist writes).
5. Google redirects to `http://localhost:50770/oauth/callback`.
6. Subotto saves a **refresh token** into SQLite and prints your channel name.
7. The auth process exits.

If the browser never opens, check the terminal for a URL and paste it yourself.

Re-run `just auth-youtube` if you revoke access, wipe `data/`, or see auth errors later.

---

## 6. Start Subotto

Still from the repo root (so it finds `.env` and `webroot/`):

```text
just run
```

You should see logs roughly like:

- config loaded
- YouTube client ready
- discord connected
- **Admin UI ready** with a URL
- listening for YouTube links…

Leave this terminal open. Ctrl+C stops Discord **and** the Admin UI.

---

## 7. Open the Admin UI

1. Browser → [http://localhost:50770](http://localhost:50770)
2. When asked for credentials:
   - **Username:** `admin` (always)
   - **Password:** whatever you set as `ADMIN_PASSWORD` in `.env`

You should see status pills (Discord / YouTube / mapping counts), a mappings form, resync, and recent activity.

---

## 8. Create a channel → playlist mapping

### Option A — Admin UI (recommended for this verify pass)

1. Pick **Server** (guilds the bot is in) and **Discord channel** from the dropdowns  
2. Enter a **New playlist name** — Subotto creates a **private** YouTube playlist for you  
3. Optional **Label** (defaults to playlist name) · leave **Enabled** checked · **SAVE + CREATE**  
4. Confirm the row appears with a generated playlist ID (`PL…`)

### Option B — CLI (another terminal; bot can keep running)

```text
just add-mapping DISCORD_CHANNEL_ID YOUTUBE_PLAYLIST_ID "my-label"
just list-mappings
```

CLI still wants an **existing** playlist ID. Prefer the Admin UI when you need Subotto to create the playlist.

Only **enabled** mappings are watched. Disable = pause; delete = **close the epoch** (history kept). A new mapping on the same channel will not resync messages older than the previous epoch, and videos already saved for that channel stay logged so they are not re-added.

---

## 9. The real test — paste a link

1. In Discord, open the **mapped** channel.
2. Post a normal YouTube URL, for example:
   - `https://www.youtube.com/watch?v=…`
   - `https://youtu.be/…`
   - Shorts / music links also work when they contain a video ID
3. Within a few seconds Subotto should react:
   - 💾 — added to the playlist (floppy = saved)
   - ♻️ + letter reacts **DUPE** — already processed on this listener (try a second paste of the same video)
   - 🛑 + letter reacts **OLD** — already filed under a previous listener epoch on this channel
   - ❌ — something failed (check the `just run` terminal + Admin activity log)

4. Open the playlist on YouTube — the video should be there.
5. Refresh the Admin UI activity table — you should see `video_added` (or skip/fail events).

**Pass criteria for “current work is good”:** one 💾 on a fresh video, playlist updated, activity logged. That is the Phase 0–5 acceptance test.

---

## 10. Optional checks (still DEV)

### Resync history

Backfills recent messages (no emoji spam on old posts):

- Admin UI → Resync form, or
- `just resync DISCORD_CHANNEL_ID` (default 100 messages, max 500)

YouTube quota is limited (~50 units per playlist insert). Do not resync huge channels for fun on day one.

### CLI mapping controls

```text
just disable-mapping CHANNEL_ID   # bot ignores the channel
just enable-mapping CHANNEL_ID
just delete-mapping CHANNEL_ID
just list-mappings
```

### Automated tests (no live APIs)

```text
just test
```

---

## Troubleshooting

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| Bot ignores links | Channel not mapped, or mapping disabled | Admin UI / `just list-mappings` |
| Bot ignores everything | Message Content Intent off | Developer Portal → Bot → enable + save |
| Bot not in channel | Missing invite / permissions | Re-invite with View + Read History + Add Reactions |
| Browser 401 on Admin | Wrong password or not `admin` | Username must be `admin` |
| Admin page ugly / 404 assets | Wrong working directory | `just run` from repo root |
| `webroot folder missing` | Same | Run from repo root; keep `webroot/` next to where you start |
| Auth or Admin “address already in use” | Port 50770 busy | Stop the other Subotto / process; do not run auth + bot together |
| YouTube auth / permission errors | Wrong Google account or revoked token | `just auth-youtube` again with the playlist owner |
| ❌ on every link | Playlist not owned / API not enabled / quota | Check Cloud Console + playlist ownership; wait if quota exhausted |
| “no YouTube token yet” | Skipped auth | `just auth-youtube` then `just run` |

---

## Commands cheat sheet

```text
just test                         # unit tests
just build                        # bin/subotto.exe
just auth-youtube                 # one-time Google login
just run                          # Discord bot + Admin UI
just add-mapping CHANNEL PLAYLIST [name]
just list-mappings
just enable-mapping CHANNEL
just disable-mapping CHANNEL
just delete-mapping CHANNEL
just resync CHANNEL [limit]
```

---

## What “done verifying” looks like

Check these off for yourself:

- [x] `just test` and `just build` OK *(code checks; Meat Bag / Clanker)*  
- [x] `.env` filled; Discord intent on; bot invited  
- [x] `just auth-youtube` printed your YouTube channel name  
- [x] `just run` shows Discord + Admin UI ready  
- [x] Admin login works (`admin` / your password)  
- [x] Mapping saved (UI or CLI)  
- [x] Fresh YouTube link → 💾 and video appears on the playlist  
- [x] Same link again → ♻️ + DUPE  
- [x] Activity shows up in the Admin UI  

**Sign-off:** Meat Bag verified good (2026-09-07). Phase 6 Admin polish followed; **next is Phase 7** — see [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md).

---

## Related docs

| File | Role |
|------|------|
| [PROGRESS.md](PROGRESS.md) | What shipped + verify sign-off |
| [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md) | Next agent: Phase 7 deploy brief |
| [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md) | Phase 6 Admin context |
| [JUMPBACK.md](JUMPBACK.md) | Resume notes for the next session |
| [PLAN.md](../PLAN.md) | Full roadmap |
| [AGENTS.md](../AGENTS.md) | How Clanker ↔ Meat Bag work |
| [README.md](../README.md) | Project overview |

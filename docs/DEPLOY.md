# Deploy Subotto to a Linux VPS

**No Docker.** Cross-compile on Windows, zip, copy to the VPS, run the native binary (or systemd).  
Caddy / TLS / firewall stay with the operator — this guide only covers Subotto’s side.

Default Admin port: **50770**.

---

## 1. Package on Windows (DEV)

```text
just package-linux
```

Produces `dist/subotto-linux.zip` with:

- `subotto-linux` — amd64 Linux binary  
- `webroot/` — Admin UI **and** `webroot/slideshow/` (OBS overlay)  
- `.env.example` — secret template  
- `deploy/subotto.service` — sample systemd unit  
- `docs/DEPLOY.md` — this guide  

**Not in the zip:** live `.env` or `data/` (copy those yourself — DB **and** `data/pictures/` / `data/episode-spots/` if you already collected images).

Public slideshow URLs (no Basic Auth): `/slideshow/latest`, `/slideshow/{slug}`, `/media/pictures/...`, `/media/episodes/{id}/...`.  
Streamer.bot GETs (no Basic Auth): `GET /api/get/content/{channel-name}`, `GET /api/get/picture/{channel-name}`, and `GET /api/get/episode/{show}` — live listener / episode JSON (`air_datetime` RFC3339 with offset, `spot_image` on the episode). Channel name is the Discord name without `#`; Discord must be connected.  
Broadcast fire (Basic Auth): `GET /api/broadcasts/{slug}/fire` — user `api` / `API_PASSWORD` (or `admin` / `ADMIN_PASSWORD`). Sends every message of that named broadcast to its mapped channels.  
YouTube re-auth (Admin): `GET /api/youtube/auth` (Basic Auth) → Google → public `GET /oauth/callback` (no password). Caddy must proxy `/oauth/callback` without extra auth.  
If you put Caddy in front, proxy the whole port (Admin + public slideshow + OAuth callback) or expose slideshow paths publicly and keep Admin locked down as you prefer — **except** `/oauth/callback`, which Google must reach unauthenticated.

---

## 2. First cutover (headless) — copy DB once

YouTube login is a **browser** OAuth dance. After Subotto is running, re-auth from Admin (Status → **Authorize YouTube**) — no desktop on the VPS required. The refresh token still lives in SQLite, not in `.env`.

### What’s in `data/subotto.db`

| Table | Role |
|-------|------|
| `oauth_tokens` | YouTube **refresh** token (must travel somehow) |
| `channel_mappings` | Content listeners / epochs |
| `processed_videos` | Content dedup history |
| `picture_listeners` | Picture listeners / epochs |
| `collected_pictures` | Saved image metadata (+ reactions) |
| `activity_log` / `app_settings` | Ops / settings (including Scheduler tab: interval, amount, listener targets) |
| `episodes` / `episode_templates` | Show episodes |
| `broadcasts` | Named Discord broadcasts |

Also copy **`data/pictures/`** and **`data/episode-spots/`** if present — picture-listener galleries and episode spot stills live next to the DB.

`.env` still holds Discord bot token + YouTube **client** ID/secret. Same Discord/Google apps — no re-registration.

### Steps

1. Upload and unzip `subotto-linux.zip` to your app dir (example: `/opt/subotto`).
2. Copy **`.env`** from DEV into that dir.
3. Copy **`data/subotto.db`** (or the whole `data/` folder) from DEV.
4. Tweaks in PROD `.env` (not new tokens):
   - `ADMIN_HOST=127.0.0.1` when Caddy (or another proxy) terminates TLS in front of Admin  
   - Optional stronger `ADMIN_PASSWORD`  
   - Optional `API_PASSWORD` for Streamer.bot broadcast fire (`api` user)  
   - `YOUTUBE_REDIRECT_URL` = the public HTTPS callback registered in Google Console (must match **exactly**, path `/oauth/callback`). Leave localhost only if you will use the SSH-tunnel fallback in §3.  
5. `chmod +x subotto-linux`
6. **Stop Windows `just run`** (one Discord gateway per bot token).
7. Smoke in foreground: `./subotto-linux` — Discord up, Admin login, paste a link → 💾.
8. Install systemd if you want (see §5), then put Caddy in front (§6).

Do **not** delete PROD `data/` later just to “feel fresh” — that throws away YouTube auth. Cease/start listeners in Admin if you only want a clean collection window.

---

## 3. YouTube re-auth (Admin UI)

Use this when the token is expired/revoked (`invalid_grant`), the DB is empty, or you switch Google accounts. Subotto stays running — no `-youtube-auth`, no service stop.

### Prerequisites

- `YOUTUBE_REDIRECT_URL` in PROD `.env` is the **public** HTTPS URL Google will call, path `/oauth/callback`.
- That same URI is listed on the OAuth client in Google Cloud Console (Web application type).
- Caddy (or Nginx) proxies `/oauth/callback` to Subotto **without** Basic Auth / extra login. Google cannot send `admin` credentials.

### Cookbook

1. Open Admin in a normal browser (already logged in as `admin`).
2. Status tab → **Authorize YouTube**. The tab goes to Google.
3. Sign in with the Google account that owns the playlists.
4. Google redirects to your public callback; Subotto saves the refresh token and hot-reloads the YouTube client (no restart).
5. You land back on Admin with a toast. Status should show **AUTHORIZED** and the channel name.

### Pitfalls

| Problem | Fix |
|---------|-----|
| Redirect URI mismatch | Console URI must equal `YOUTUBE_REDIRECT_URL` character-for-character |
| Callback 401 from Caddy | Do not put edge auth on `/oauth/callback` |
| “invalid or expired OAuth state” | Start again from Admin (state lasts 10 minutes, one-shot) |
| Wrong Google account | Use the account that owns the target playlists |

Day-to-day playlist writes do **not** need a browser after the token is saved.

### Fallback: SSH tunnel + `-youtube-auth`

Only if the redirect URL is still `http://localhost:50770/oauth/callback` (no public callback).

1. Stop Subotto (it owns `:50770`).
2. Laptop: `ssh -L 50770:127.0.0.1:50770 user@your-vps`
3. VPS: `./subotto-linux -youtube-auth`
4. Paste the printed Google URL on the laptop; complete consent.
5. Restart the normal service.

---

## 4. Updates (keep `data/` intact)

When shipping a new build from Windows:

1. `just package-linux` again.
2. On the VPS: stop Subotto.
3. Replace **`subotto-linux`** and **`webroot/`** only.
4. Leave **`data/`** and **`.env`** alone (unless you intentionally change config).
5. Start Subotto again.

Never unzip over a live `data/` folder blindly.

---

## 5. Sample systemd

See `deploy/subotto.service` in the zip (or this repo).

```text
sudo cp deploy/subotto.service /etc/systemd/system/subotto.service
# Edit WorkingDirectory / ExecStart if your path is not /opt/subotto
sudo systemctl daemon-reload
sudo systemctl enable --now subotto
sudo systemctl status subotto
journalctl -u subotto -f
```

Subotto loads `.env` from `WorkingDirectory`. Keep `.env`, the binary, `webroot/`, and `data/` together.

---

## 6. Caddy (edge is operator-owned)

Bind Subotto to loopback so only the proxy is public:

```text
ADMIN_HOST=127.0.0.1
ADMIN_PORT=50770
```

Point Caddy (or Nginx) at `http://127.0.0.1:50770`. TLS, hostnames, and edge auth are up to the operator — Subotto still uses Basic Auth (`admin` / `ADMIN_PASSWORD`). Leave **`/oauth/callback` public** at the edge so Google’s redirect can complete Admin YouTube auth.

---

## 7. SQLite backup

While Subotto is **stopped** (simplest safe copy):

```text
cp data/subotto.db /path/to/backups/subotto-$(date +%Y%m%d).db
```

Restore: stop Subotto, replace `data/subotto.db`, start again.  
If you prefer online backups later, use SQLite’s `.backup` / `sqlite3` backup API — stop-and-copy is enough for a small ops DB.

---

## 8. Smoke checklist (PROD)

- [ ] `subotto-linux` runs; logs look healthy  
- [ ] Admin login works (via Caddy or direct)  
- [ ] YouTube token present (Admin Status **AUTHORIZED**, or re-auth via **Authorize YouTube**)  
- [ ] At least one enabled listener  
- [ ] Paste a YouTube link → **💾** (or expected ♻️ / 🛑)  
- [ ] Windows DEV bot is **not** running with the same token  

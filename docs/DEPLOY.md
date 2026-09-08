# Deploy Subotto to a Linux VPS (Phase 7)

**No Docker.** Cross-compile on Windows, zip, copy to your box, run the native binary (or systemd).  
You own Caddy / TLS / firewall — this guide only covers Subotto’s side.

Default Admin port: **50770**.

---

## 1. Package on Windows (DEV)

```text
just package-linux
```

Produces `dist/subotto-linux.zip` with:

- `subotto-linux` — amd64 Linux binary  
- `webroot/` — Admin UI  
- `.env.example` — secret template  
- `deploy/subotto.service` — sample systemd unit  
- `docs/DEPLOY.md` — this guide  

**Not in the zip:** live `.env` or `data/*.db` (copy those yourself).

---

## 2. First cutover (headless) — copy DB once

YouTube login is a **browser** OAuth dance. A VPS with no desktop cannot complete `auth-youtube` alone.  
You already authorized on Windows; the refresh token lives in SQLite, not in `.env`.

### What’s in `data/subotto.db`

| Table | Role |
|-------|------|
| `oauth_tokens` | YouTube **refresh** token (must travel somehow) |
| `channel_mappings` | Listeners / epochs |
| `processed_videos` | Dedup history |
| `activity_log` / `app_settings` | Ops / settings |

`.env` still holds Discord bot token + YouTube **client** ID/secret. Same Discord/Google apps — no re-registration.

### Steps

1. Upload and unzip `subotto-linux.zip` to your app dir (example: `/opt/subotto`).
2. Copy **`.env`** from DEV into that dir.
3. Copy **`data/subotto.db`** (or the whole `data/` folder) from DEV.
4. Tweaks in PROD `.env` (not new tokens):
   - `ADMIN_HOST=127.0.0.1` when Caddy (or another proxy) terminates TLS in front of Admin  
   - Optional stronger `ADMIN_PASSWORD`  
   - Leave `YOUTUBE_REDIRECT_URL=http://localhost:50770/oauth/callback` if you are not re-authing on the box  
5. `chmod +x subotto-linux`
6. **Stop Windows `just run`** (one Discord gateway per bot token).
7. Smoke in foreground: `./subotto-linux` — Discord up, Admin login, paste a link → 💾.
8. Install systemd if you want (see §5), then put Caddy in front (§6).

Do **not** delete PROD `data/` later just to “feel fresh” — that throws away YouTube auth. Cease/start listeners in Admin if you only want a clean collection window.

---

## 3. Headless YouTube re-auth (SSH tunnel)

Use this when the DB is empty, the token is lost, or you switch Google accounts.  
No desktop on the VPS required — the **laptop** browser does the consent; SSH forwards the callback.

### Prerequisites

- `YOUTUBE_REDIRECT_URL=http://localhost:50770/oauth/callback` in PROD `.env` (or change both the env value and the tunnel port together).
- Subotto **must not** already be listening on `:50770` (stop the service / kill the binary).
- Keep the SSH tunnel up until the success page appears.

### Cookbook

1. **On the laptop** (Windows OpenSSH or similar):

   ```text
   ssh -L 50770:127.0.0.1:50770 user@your-vps
   ```

   Leave this session open.

2. **On the VPS** (second SSH session), from the app directory:

   ```text
   ./subotto-linux -youtube-auth
   ```

   Opening a desktop browser on the server may fail — ignore that. The process still listens and **prints a Google URL** in the log.

3. **On the laptop**, paste that URL into a normal browser. Sign in with the Google account that owns the playlists.

4. Google redirects to `http://localhost:50770/oauth/callback` on the **laptop**. The tunnel forwards that hit to the VPS process, which saves the refresh token into `data/subotto.db`.

5. Confirm a log line that the YouTube token was saved. Stop the auth process (Ctrl+C). Start the normal Subotto service again.

### Pitfalls

| Problem | Fix |
|---------|-----|
| “listen on :50770 … already running?” | Stop systemd / the main binary before `-youtube-auth` |
| Redirect / connection refused on laptop | Tunnel not up, wrong port, or auth process exited early |
| Wrong Google account | Use the account that owns the target playlists |
| Tunnel closed mid-flow | Re-run from step 1 |

Day-to-day playlist writes do **not** need a browser after the token is saved.

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

## 6. Caddy (you own the rest)

Bind Subotto to loopback so only the proxy is public:

```text
ADMIN_HOST=127.0.0.1
ADMIN_PORT=50770
```

Point Caddy (or Nginx) at `http://127.0.0.1:50770`. TLS, hostnames, and auth at the edge are your call — Subotto still uses Basic Auth (`admin` / `ADMIN_PASSWORD`).

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
- [ ] YouTube token present (no “run auth-youtube” error on ingest)  
- [ ] At least one enabled listener  
- [ ] Paste a YouTube link → **💾** (or expected ♻️ / 🛑)  
- [ ] Windows DEV bot is **not** running with the same token  

Your keys, your box, your epochs.

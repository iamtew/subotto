# Handoff — Phase 7 native Linux VPS deploy

**For:** next Clanker / Cursor agent  
**From:** Phase 0–6 arc (DEV verified + Phase 6 Admin polish)  
**Date:** 2026-09-08  
**Branch:** `master`

Meat Bag is ready to **plan and implement Phase 7** — native deploy to a Linux VPS (EU), **no Docker**. Phase 6 product/Admin polish is in good shape; do not reopen Phase 6 unless Meat Bag asks.

Suggested opener:  
> Clanker, read docs/HANDOFF-PHASE7.md and docs/JUMPBACK.md — plan Phase 7 deployment with me.

---

## 1. Where we are (do not re-litigate)

### Live DEV (Meat Bag sign-off)
- Discord → ingest → public YouTube playlist works
- Reactions: **💾** add · **♻️**+letter **DUPE** same-listener · **🛑**+letter **OLD** previous-listener · **❌** fail
- Admin UI at port **50770** (Basic Auth `admin` / `ADMIN_PASSWORD`)
- Listeners, epochs, channel-scoped dedup, ONLINE/OFFLINE notices
- Scheduler exists; stays **off** when `RESYNC_INTERVAL_HOURS=0`

### Phase 6 polish already landed (recent commits)
- Listener product language (not “listening post” / mapping in UI)
- Digicam Admin backdrop; denser header for low-res
- START confirm when replacing a live listener; `#channel` names in table
- Notices rename + playlist-link defaults; playlists created **public**

### Code checks
```text
just test
just build
just build-linux   # already in justfile → bin/subotto-linux
```

---

## 2. Constraints (hard)

- **No Docker / no containers** — native Go binary + systemd (or similar)
- **Just** remains the build system; DEV = Windows, PROD = Linux
- Ship **binary + `webroot/` + `data/`** (+ `.env` or env vars) together
- Plain HTML/CSS/JS Admin; SQLite; Clanker ↔ Meat Bag voice; commit only when asked
- Product language: **Listener** / start·cease listener; notices (not “copy”)
- Libertarian tone via voluntary / sovereignty language only — never name opposing ideologies in copy

---

## 3. What Phase 7 must deliver (PLAN.md)

| Item | Intent |
|------|--------|
| Cross-compile from Windows | Document / verify `just build-linux` → `bin/subotto-linux` |
| systemd unit | Run Subotto as a service (restart, logs, working directory) |
| Sync story | How to update binary + `webroot/` + keep `data/` (SQLite) safe |
| SQLite backup | Simple backup/restore recipe for `data/subotto.db` |
| Optional reverse proxy | Caddy or Nginx in front of Admin (TLS, `ADMIN_HOST=127.0.0.1`) |
| README / guide | Meat-Bag-friendly deploy walkthrough (EU VPS) |

Optional (ask Meat Bag): public `/healthz` without Basic Auth for uptime checks — today only `/api/status` (auth’d) exists.

---

## 4. Architecture you inherit

```text
Windows DEV
  just build-linux
    → bin/subotto-linux

Linux VPS
  ./subotto-linux   (+ webroot/ + data/ + .env)
    → Discord gateway (live ingest + reactions)
    → Admin HTTP (Basic Auth, default :50770)
    → optional scheduler (RESYNC_INTERVAL_HOURS)
```

### Key paths

| Path | Role |
|------|------|
| `justfile` | `build-linux`, `run`, `test`, listener CLI aliases |
| `cmd/subotto/main.go` | Process entry, graceful shutdown wiring |
| `internal/config/` | `.env` keys including `ADMIN_HOST` / `ADMIN_PORT` |
| `internal/scheduler/` | Background resync when interval > 0 |
| `webroot/` | Must sit next to the binary (or set path if you add one later) |
| `data/` | SQLite + OAuth token — **back this up** |
| `.env.example` | Template for PROD secrets |

### Config Meat Bag will need on VPS

See `.env.example` / PLAN.md §7. Especially:

- `ADMIN_HOST` — `0.0.0.0` direct, or `127.0.0.1` behind reverse proxy  
- `ADMIN_PORT` — default **50770**  
- `YOUTUBE_REDIRECT_URL` — may need a **production** OAuth redirect if Meat Bag re-auths on the VPS (DEV still uses `http://localhost:50770/oauth/callback`)  
- `DATABASE_PATH` — keep under a durable `data/` directory  
- `RESYNC_INTERVAL_HOURS` — leave `0` unless Meat Bag wants background scans  

---

## 5. Suggested Phase 7 shape

1. **Guide first** — write `docs/DEPLOY.md` (or expand README) before inventing new flags.  
2. **systemd unit example** — `WorkingDirectory`, `ExecStart`, `Restart=on-failure`, env file or `EnvironmentFile=`.  
3. **Deploy checklist** — copy binary, sync `webroot/`, never clobber `data/` blindly, restart service.  
4. **Backup** — stop or use SQLite-safe copy; document restore.  
5. **Proxy (optional)** — TLS + Basic Auth still on Subotto (or proxy auth — ask Meat Bag).  
6. **Smoke on VPS** — Discord up, Admin login, one link → 💾, notices if START/CEASE.  
7. Update [PROGRESS.md](PROGRESS.md) / [JUMPBACK.md](JUMPBACK.md) / [README.md](../README.md) when Phase 7 ships.

Do **not** add Docker “just for deploy convenience.”

---

## 6. Still open / not Phase 7

| Item | Notes |
|------|--------|
| Announce notice **preview** in Admin | Optional leftover Phase 6 polish — only if Meat Bag asks |
| Shutdown wait for in-flight scheduler tick | Nice robustness nit |
| Slash commands, metrics, cookie auth | PLAN nice-to-haves — defer |

---

## 7. Read these first (in order)

1. [AGENTS.md](../AGENTS.md)  
2. [JUMPBACK.md](JUMPBACK.md)  
3. [PROGRESS.md](PROGRESS.md)  
4. This file  
5. [PLAN.md](../PLAN.md) § Phase 7  
6. [DEV-VERIFY.md](DEV-VERIFY.md) — DEV baseline; do not break it  
7. [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md) — Phase 6 context (complete enough)

---

## 8. Definition of done (Phase 7)

- [ ] Meat Bag can cross-compile from Windows and run on Linux VPS  
- [ ] systemd (or agreed service) starts Subotto on boot and restarts on failure  
- [ ] Deploy/update steps keep `data/` intact and refresh binary + `webroot/`  
- [ ] SQLite backup recipe documented and tried once  
- [ ] README (or `docs/DEPLOY.md`) is the single source of truth for PROD  
- [ ] Optional reverse proxy documented if Meat Bag wants TLS in front  

Then celebrate — Subotto is a free individual’s wire on their own VPS.

---

**Clanker’s note:** Phase 6 closed for product work. Phase 7 is ops sovereignty — your keys, your box, your epochs. Keep it native and simple.

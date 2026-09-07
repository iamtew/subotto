# Handoff — Phase 6 continued: Admin UI polish

**For:** Clanker / next session  
**From:** Phase 6 scheduler + port 50770 commit (`1061bae`)  
**Date:** 2026-09-07  
**Branch:** `master`

Meat Bag wants to **stay on Phase 6 for a while** and **improve the Admin UI a lot** before Phase 7 (VPS/systemd). Backend scheduler is in place; UI is the focus.

Suggested opener:  
> Clanker, read docs/HANDOFF-ADMIN-UI.md and improve the Admin UI.

---

## 1. Constraints (do not break)

- **Plain HTML / CSS / vanilla JS** in [`webroot/`](../webroot/) — no React, no build step, no Docker.
- Admin is served by the same binary ([`internal/web/`](../internal/web/)) with **Basic Auth** (`admin` / `ADMIN_PASSWORD`).
- Existing JSON API stays the source of truth:
  - `GET /api/status`
  - `GET|POST /api/mappings`, `PATCH|DELETE /api/mappings/{channel}`
  - `GET /api/activity`
  - `POST /api/resync`
- Keep beginner-friendly comments; Clanker ↔ Meat Bag voice in docs.
- Default port **50770**; do not regress OAuth/Admin sharing that port.
- Commit only when Meat Bag asks.

---

## 2. Current UI map

| File | Role |
|------|------|
| [`webroot/index.html`](../webroot/index.html) | Header + status pills, mappings form/table, resync form, activity table |
| [`webroot/css/admin.css`](../webroot/css/admin.css) | Theme tokens (forest/ink on warm paper), panels, tables |
| [`webroot/js/admin.js`](../webroot/js/admin.js) | `api()`, load status/mappings/activity, form handlers |

Backend already exposes scheduler fields on `/api/status` (`scheduler_enabled`, `resync_interval_hours`, `scheduler_last_run_at`, `scheduler_last_error`). Pills show a basic on/off; richer UI can use these.

---

## 3. What “a lot better” likely means (ask Meat Bag)

Do **not** invent a total brand rewrite without direction. Clarify preferences, then iterate. Likely themes:

1. **Clarity** — less ID pasting; channel picker from mappings for resync; empty states that teach.
2. **Feedback** — clearer success/error toasts; resync progress; activity details readable (pretty JSON or key fields).
3. **Layout** — less stacked “dashboard panels”; stronger Subotto brand in the first viewport; mobile-friendly.
4. **Ops** — surface scheduler interval / last run / last error without digging logs.
5. **Safety** — confirm before delete; disable dangerous actions while a resync runs.

Stay inside plain CSS visual rules from Meat Bag’s design preferences when redesigning (expressive type, real atmosphere, no purple-glow AI defaults, cards only when they help interaction).

---

## 4. Safe iteration order

1. Confirm look + priority with Meat Bag (1–2 questions).
2. Improve structure/UX in `webroot/` first (biggest win, lowest risk).
3. Only extend `/api/*` if the UI truly needs new fields (keep auth).
4. `just test` / `just build`; Meat Bag browser-checks on `http://localhost:50770`.

---

## 5. Out of scope until Meat Bag says otherwise

- Phase 7 systemd / VPS guide  
- Public `/healthz`  
- Rewriting the Go API in a new framework  
- Docker  

---

**Clanker’s note:** Scheduler brain is done enough to pause. Polish the cockpit Meat Bag stares at every day.

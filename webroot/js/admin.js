/* Subotto Admin UI — vanilla JS for a sovereign Meat Bag.
   Talks to /api/... with browser Basic Auth (collected on page load).
   Product language: LISTENERS (start listener / cease listener). */

/* ---------- Digital camo backdrop (noise → quantized pixels, no tile) ----------
   Meat Bag: woodland digicam like your refs — small pixels, irregular clusters,
   one big canvas so it does not wallpaper-repeat across the screen. */
function paintDigicam() {
  const el = document.querySelector(".bg-camo");
  if (!el) return;

  const cell = 7; // digicam block size in CSS px (bigger = chunkier camo)
  const w = Math.max(window.innerWidth, document.documentElement.clientWidth, 1280);
  const h = Math.max(window.innerHeight, document.documentElement.clientHeight, 800);
  const gw = Math.ceil(w / cell);
  const gh = Math.ceil(h / cell);

  // Weighted toward darks; sparse olive / violet so panels stay readable.
  const palette = [
    [12, 15, 11], // bg-input
    [16, 20, 15], // bg0
    [22, 27, 20], // between bg0/bg1
    [26, 33, 24], // bg-panel
    [45, 53, 40], // line
    [108, 117, 74], // dusty olive
    [154, 172, 98], // muted olive
    [102, 102, 102], // gray
    [134, 0, 223], // royal violet (rare)
  ];

  function hash2(x, y) {
    let n = Math.imul(x, 374761393) + Math.imul(y, 668265263);
    n = Math.imul(n ^ (n >>> 13), 1274126177);
    return ((n ^ (n >>> 16)) >>> 0) / 4294967296;
  }

  function valueNoise(x, y) {
    const x0 = Math.floor(x);
    const y0 = Math.floor(y);
    const fx = x - x0;
    const fy = y - y0;
    const sx = fx * fx * (3 - 2 * fx);
    const sy = fy * fy * (3 - 2 * fy);
    const a = hash2(x0, y0);
    const b = hash2(x0 + 1, y0);
    const c = hash2(x0, y0 + 1);
    const d = hash2(x0 + 1, y0 + 1);
    return a + (b - a) * sx + (c - a) * sy + (a - b - c + d) * sx * sy;
  }

  // Fractal noise → irregular blotches like military digicam.
  function fbm(x, y) {
    let v = 0;
    let amp = 0.55;
    let freq = 1;
    for (let i = 0; i < 5; i++) {
      v += amp * valueNoise(x * freq, y * freq);
      amp *= 0.5;
      freq *= 2.05;
    }
    return Math.min(1, Math.max(0, v));
  }

  function pickColor(n, accent) {
    if (accent > 0.965) return palette[8]; // rare violet fleck
    if (n < 0.18) return palette[0];
    if (n < 0.32) return palette[1];
    if (n < 0.46) return palette[2];
    if (n < 0.58) return palette[3];
    if (n < 0.7) return palette[4];
    if (n < 0.82) return palette[5];
    if (n < 0.92) return palette[6];
    return palette[7];
  }

  const low = document.createElement("canvas");
  low.width = gw;
  low.height = gh;
  const lctx = low.getContext("2d");
  const img = lctx.createImageData(gw, gh);
  const data = img.data;

  // Slightly stretch X so clusters lean horizontal (classic digicam vibe).
  const scaleX = 0.085;
  const scaleY = 0.11;
  for (let y = 0; y < gh; y++) {
    for (let x = 0; x < gw; x++) {
      const n = fbm(x * scaleX, y * scaleY);
      const accent = hash2(x * 17 + 3, y * 29 + 7);
      const rgb = pickColor(n, accent);
      const i = (y * gw + x) * 4;
      data[i] = rgb[0];
      data[i + 1] = rgb[1];
      data[i + 2] = rgb[2];
      data[i + 3] = 255;
    }
  }
  lctx.putImageData(img, 0, 0);

  const out = document.createElement("canvas");
  out.width = gw * cell;
  out.height = gh * cell;
  const octx = out.getContext("2d");
  octx.imageSmoothingEnabled = false;
  octx.drawImage(low, 0, 0, out.width, out.height);

  el.style.backgroundImage = `url(${out.toDataURL("image/png")})`;
  el.style.backgroundSize = `${out.width}px ${out.height}px`;
}

let camoResizeTimer = 0;
function scheduleDigicam() {
  clearTimeout(camoResizeTimer);
  camoResizeTimer = setTimeout(paintDigicam, 180);
}

paintDigicam();
window.addEventListener("resize", scheduleDigicam);

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: {
      Accept: "application/json",
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers || {}),
    },
    ...options,
  });
  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { error: text || "non-JSON response" };
  }
  if (!res.ok) {
    const msg = (data && data.error) || res.statusText || "request failed";
    throw new Error(msg);
  }
  return data;
}

function pill(label, kind) {
  const span = document.createElement("span");
  let cls = "muted";
  if (kind === true || kind === "ok") cls = "ok";
  else if (kind === false || kind === "bad") cls = "bad";
  else if (kind === "warn") cls = "warn";
  else if (kind === "muted") cls = "muted";
  span.className = "pill " + cls;
  span.textContent = label;
  return span;
}

function esc(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function toast(msg, isErr) {
  const el = document.getElementById("toast");
  el.hidden = false;
  el.classList.toggle("err", !!isErr);
  el.textContent = msg;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => {
    el.hidden = true;
  }, 3200);
}

function fmtDetails(details) {
  if (!details || typeof details !== "object") return "";
  try {
    return JSON.stringify(details, null, 2);
  } catch {
    return String(details);
  }
}

let lastResyncChannel = "";

function fillResyncSelect(listens) {
  const sel = document.getElementById("resync-channel");
  const prev = sel.value || lastResyncChannel;
  const options = ['<option value="">— select listener —</option>'];
  for (const m of listens || []) {
    const off = m.enabled ? "" : " (paused)";
    const label = (m.name ? m.name + " · " : "") + m.discord_channel_id + off;
    options.push(
      `<option value="${esc(m.discord_channel_id)}">${esc(label)}</option>`
    );
  }
  sel.innerHTML = options.join("");
  if (prev && [...sel.options].some((o) => o.value === prev)) {
    sel.value = prev;
  }
}

async function loadGuilds() {
  const sel = document.getElementById("guild-select");
  try {
    const data = await api("/api/discord/guilds");
    const guilds = data.guilds || [];
    if (!guilds.length) {
      sel.innerHTML = `<option value="">— no servers (is the bot invited?) —</option>`;
      return;
    }
    sel.innerHTML =
      `<option value="">— select server —</option>` +
      guilds
        .map((g) => `<option value="${esc(g.id)}">${esc(g.name)}</option>`)
        .join("");
  } catch (err) {
    sel.innerHTML = `<option value="">— ${esc(err.message)} —</option>`;
  }
}

async function loadChannelsForGuild(guildID) {
  const sel = document.getElementById("channel-select");
  if (!guildID) {
    sel.disabled = true;
    sel.innerHTML = `<option value="">— pick a server first —</option>`;
    return;
  }
  sel.disabled = true;
  sel.innerHTML = `<option value="">loading channels…</option>`;
  try {
    const data = await api("/api/discord/guilds/" + encodeURIComponent(guildID) + "/channels");
    const channels = data.channels || [];
    if (!channels.length) {
      sel.innerHTML = `<option value="">— no text channels visible —</option>`;
      return;
    }
    sel.innerHTML =
      `<option value="">— select channel —</option>` +
      channels
        .map((c) => `<option value="${esc(c.id)}">#${esc(c.name)}</option>`)
        .join("");
    sel.disabled = false;
  } catch (err) {
    sel.innerHTML = `<option value="">— ${esc(err.message)} —</option>`;
  }
}

async function loadStatus() {
  const host = document.getElementById("status-pills");
  try {
    const s = await api("/api/status");
    const on = s.listens_enabled ?? s.airs_enabled ?? s.mappings_enabled;
    const tot = s.listens_total ?? s.airs_total ?? s.mappings_total;
    host.replaceChildren(
      pill(s.discord_connected ? "discord up" : "discord down", !!s.discord_connected),
      pill(s.youtube_authorized ? "youtube ok" : "youtube missing", !!s.youtube_authorized),
      pill(`listen ${on}/${tot}`, true),
      pill(`log ${s.activity_total}`, "muted")
    );
    if (s.youtube_channel) {
      host.appendChild(pill(s.youtube_channel, "ok"));
    }
    if (s.scheduler_enabled) {
      const hours = s.resync_interval_hours || "?";
      host.appendChild(pill(`sched ${hours}h`, "ok"));
      if (s.scheduler_last_error) {
        host.appendChild(pill("sched err", "bad"));
      } else if (s.scheduler_last_run_at) {
        host.appendChild(pill("sched ran", "muted"));
      }
    } else {
      host.appendChild(pill("sched off", "muted"));
    }
  } catch (err) {
    host.replaceChildren(pill("status fail: " + err.message, "bad"));
  }
}

async function loadListens() {
  const tbody = document.querySelector("#listens-table tbody");
  try {
    const data = await api("/api/listens");
    const rows = data.listens || data.airs || data.mappings || [];
    fillResyncSelect(rows);
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="6" class="empty">no listeners — start listener above</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((m) => {
        const ch = esc(m.discord_channel_id);
        const state = m.enabled
          ? `<span class="state-on">LISTENING</span>`
          : `<span class="state-off">PAUSED</span>`;
        const since = m.active_from ? new Date(m.active_from).toLocaleString() : "—";
        return `<tr>
          <td>${esc(m.name) || "—"}</td>
          <td class="mono">${ch}</td>
          <td class="mono">${esc(m.youtube_playlist_id)}</td>
          <td class="mono">${esc(since)}</td>
          <td>${state}</td>
          <td class="actions">
            <button type="button" class="secondary" data-act="rename" data-channel="${ch}" data-name="${esc(m.name)}">Rename PL</button>
            <button type="button" class="secondary" data-act="toggle" data-channel="${ch}" data-enabled="${m.enabled}">
              ${m.enabled ? "Pause" : "Resume"}
            </button>
            <button type="button" class="secondary" data-act="resync" data-channel="${ch}">Resync</button>
            <button type="button" class="danger" data-act="delete" data-channel="${ch}">Cease</button>
          </td>
        </tr>`;
      })
      .join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6" class="empty">failed: ${esc(err.message)}</td></tr>`;
  }
}

async function loadActivity() {
  const tbody = document.querySelector("#activity-table tbody");
  try {
    const data = await api("/api/activity?limit=40");
    const rows = data.activity || [];
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="4" class="empty">no activity yet</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((e) => {
        const when = e.timestamp ? new Date(e.timestamp).toLocaleString() : "";
        const details = esc(fmtDetails(e.details));
        const ok = e.success
          ? `<span class="ok-yes">YES</span>`
          : `<span class="ok-no">NO</span>`;
        return `<tr>
          <td>${esc(when)}</td>
          <td class="mono">${esc(e.event_type)}</td>
          <td>${ok}</td>
          <td><pre class="details-pre">${details}</pre></td>
        </tr>`;
      })
      .join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="4" class="empty">failed: ${esc(err.message)}</td></tr>`;
  }
}

async function loadAnnounce() {
  try {
    const data = await api("/api/settings/listen-messages");
    document.getElementById("start-message").value = data.start_message || "";
    document.getElementById("stop-message").value = data.stop_message || "";
  } catch (err) {
    toast("announce load failed: " + err.message, true);
  }
}

async function refreshAll() {
  await Promise.all([loadStatus(), loadListens(), loadActivity()]);
}

document.getElementById("guild-select").addEventListener("change", (ev) => {
  loadChannelsForGuild(ev.target.value);
});

document.getElementById("listen-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const btn = ev.target.querySelector('button[type="submit"]');
  const body = {
    name: fd.get("name") || "",
    guild_id: fd.get("guild_id") || "",
    discord_channel_id: fd.get("discord_channel_id"),
    youtube_playlist_title: fd.get("youtube_playlist_title"),
    enabled: fd.get("enabled") === "on",
  };
  btn.disabled = true;
  try {
    const m = await api("/api/listens", { method: "POST", body: JSON.stringify(body) });
    ev.target.reset();
    ev.target.querySelector('[name="enabled"]').checked = true;
    document.getElementById("channel-select").disabled = true;
    document.getElementById("channel-select").innerHTML =
      `<option value="">— pick a server first —</option>`;
    await loadGuilds();
    toast("listener · " + (m.youtube_playlist_id || "ok"));
    await refreshAll();
  } catch (err) {
    toast("start listener failed: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("announce-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  try {
    await api("/api/settings/listen-messages", {
      method: "PUT",
      body: JSON.stringify({
        start_message: fd.get("start_message") || "",
        stop_message: fd.get("stop_message") || "",
      }),
    });
    toast("announce copy saved");
  } catch (err) {
    toast("save failed: " + err.message, true);
  }
});

document.getElementById("listens-table").addEventListener("click", async (ev) => {
  const btn = ev.target.closest("button[data-act]");
  if (!btn) return;
  const channel = btn.getAttribute("data-channel");
  const act = btn.getAttribute("data-act");
  try {
    if (act === "rename") {
      const current = btn.getAttribute("data-name") || "";
      const title = prompt("New YouTube playlist title:", current);
      if (title === null) return;
      const trimmed = title.trim();
      if (!trimmed) {
        toast("title required", true);
        return;
      }
      await api("/api/listens/" + encodeURIComponent(channel), {
        method: "PATCH",
        body: JSON.stringify({ youtube_playlist_title: trimmed }),
      });
      toast("playlist renamed");
    } else if (act === "toggle") {
      const enabled = btn.getAttribute("data-enabled") === "true";
      await api("/api/listens/" + encodeURIComponent(channel), {
        method: "PATCH",
        body: JSON.stringify({ enabled: !enabled }),
      });
      toast(enabled ? "listener paused" : "listener resumed");
    } else if (act === "delete") {
      if (!confirm("Cease listener on channel " + channel + "? (epoch closes; history kept)")) return;
      await api("/api/listens/" + encodeURIComponent(channel), { method: "DELETE" });
      toast("listener offline");
    } else if (act === "resync") {
      lastResyncChannel = channel;
      const sel = document.getElementById("resync-channel");
      if (![...sel.options].some((o) => o.value === channel)) {
        const opt = document.createElement("option");
        opt.value = channel;
        opt.textContent = channel;
        sel.appendChild(opt);
      }
      sel.value = channel;
      document.getElementById("resync-section").scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    await refreshAll();
  } catch (err) {
    toast(err.message, true);
  }
});

document.getElementById("resync-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const channel = fd.get("channel_id");
  if (!channel) {
    toast("pick a listener", true);
    return;
  }
  lastResyncChannel = String(channel);
  const out = document.getElementById("resync-result");
  const btn = ev.target.querySelector('button[type="submit"]');
  out.hidden = false;
  out.classList.remove("err");
  out.textContent = "running…";
  btn.disabled = true;
  try {
    const data = await api("/api/resync", {
      method: "POST",
      body: JSON.stringify({
        channel_id: channel,
        limit: Number(fd.get("limit") || 100),
      }),
    });
    out.textContent = `scanned=${data.messages_scanned} added=${data.added} skipped=${data.skipped} failed=${data.failed}`;
    toast("resync done");
    await refreshAll();
  } catch (err) {
    out.classList.add("err");
    out.textContent = err.message;
    toast(err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("resync-channel").addEventListener("change", (ev) => {
  lastResyncChannel = ev.target.value || "";
});

loadGuilds();
loadAnnounce();
refreshAll();
setInterval(refreshAll, 15000);

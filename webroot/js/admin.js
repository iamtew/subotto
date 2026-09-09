/* Subotto Admin UI — vanilla JS for a sovereign Meat Bag.
   Talks to /api/... with browser Basic Auth (collected on page load).
   Product language: content listeners + picture listeners (expandable tabs). */

/* ---------- Expandable tab registry ----------
   Meat Bag: to add a future tab, push { id, label } here and add a
   matching #tab-{id} panel in index.html. */
const TAB_REGISTRY = [
  { id: "content", label: "Content listeners" },
  { id: "pictures", label: "Picture listeners" },
];

/** Overlay size steps: 0.5x … 5x in 0.25 increments (default 1.5). */
function overlayScaleOptionsHTML(selected) {
  const sel = Number(selected) > 0 ? Number(selected) : 1.5;
  const parts = [];
  for (let v = 0.5; v <= 5.001; v += 0.25) {
    const rounded = Math.round(v * 100) / 100;
    const label = Number.isInteger(rounded) ? rounded + "x" : rounded.toFixed(2).replace(/0$/, "") + "x";
    const isSel = Math.abs(rounded - sel) < 0.001;
    parts.push(
      `<option value="${rounded}"${isSel ? " selected" : ""}>${label}</option>`
    );
  }
  return parts.join("");
}

function fillOverlayScaleSelects() {
  const ids = ["picture-settings-credit-scale", "picture-settings-reaction-scale"];
  for (const id of ids) {
    const el = document.getElementById(id);
    if (el && !el.options.length) {
      el.innerHTML = overlayScaleOptionsHTML(1.5);
    }
  }
  const multEl = document.getElementById("picture-settings-multiplier");
  if (multEl && !multEl.options.length) {
    multEl.innerHTML = reactionMultiplierOptionsHTML(1);
  }
}

function reactionMultiplierOptionsHTML(selected) {
  const sel = Math.max(1, Math.min(25, Number(selected) || 1));
  const parts = [];
  for (let v = 1; v <= 25; v++) {
    parts.push(
      `<option value="${v}"${v === sel ? " selected" : ""}>${v}x</option>`
    );
  }
  return parts.join("");
}

function syncAnimatedMultiplierVisibility() {
  const animated = document.getElementById("picture-settings-animated");
  const wrap = document.getElementById("picture-settings-multiplier-wrap");
  if (!animated || !wrap) return;
  wrap.hidden = !animated.checked;
}

document.getElementById("picture-settings-animated").addEventListener("change", syncAnimatedMultiplierVisibility);

function initTabs() {
  const bar = document.getElementById("tab-bar");
  const saved = localStorage.getItem("subotto_admin_tab") || "content";
  bar.innerHTML = TAB_REGISTRY.map((t) => {
    const sel = t.id === saved ? "true" : "false";
    return `<button type="button" class="tab-btn" role="tab" id="tabbtn-${t.id}"
      data-tab="${t.id}" aria-selected="${sel}" aria-controls="tab-${t.id}">${t.label}</button>`;
  }).join("");

  bar.addEventListener("click", (ev) => {
    const btn = ev.target.closest("[data-tab]");
    if (!btn) return;
    activateTab(btn.getAttribute("data-tab"));
  });

  activateTab(TAB_REGISTRY.some((t) => t.id === saved) ? saved : TAB_REGISTRY[0].id);
}

function activateTab(id) {
  localStorage.setItem("subotto_admin_tab", id);
  document.querySelectorAll(".tab-btn").forEach((btn) => {
    btn.setAttribute("aria-selected", btn.getAttribute("data-tab") === id ? "true" : "false");
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.hidden = panel.getAttribute("data-tab") !== id;
  });
}

/* ---------- Digital camo backdrop (noise → quantized pixels, no tile) ---------- */
function paintDigicam() {
  const el = document.querySelector(".bg-camo");
  if (!el) return;

  const cell = 7;
  const w = Math.max(window.innerWidth, document.documentElement.clientWidth, 1280);
  const h = Math.max(window.innerHeight, document.documentElement.clientHeight, 800);
  const gw = Math.ceil(w / cell);
  const gh = Math.ceil(h / cell);

  const palette = [
    [12, 15, 11],
    [16, 20, 15],
    [22, 27, 20],
    [26, 33, 24],
    [45, 53, 40],
    [108, 117, 74],
    [154, 172, 98],
    [102, 102, 102],
    [134, 0, 223],
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
    if (accent > 0.965) return palette[8];
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

/** Discord-down pill: clickable button that POSTs /api/discord/reconnect. */
function discordStatusPill(connected) {
  if (connected) {
    return pill("discord up", true);
  }
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "pill bad pill-action";
  btn.textContent = "discord down · reconnect";
  btn.title = "Click to force a Discord gateway reconnect";
  btn.addEventListener("click", async () => {
    if (btn.disabled) return;
    btn.disabled = true;
    btn.textContent = "discord reconnecting…";
    try {
      const data = await api("/api/discord/reconnect", { method: "POST" });
      toast(data.message || "discord reconnect requested", false);
    } catch (err) {
      toast(err.message || "discord reconnect failed", true);
    }
    await loadStatus();
  });
  return btn;
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
let lastPictureResyncChannel = "";
let cachedListens = [];
let cachedPictureListens = [];

function channelLabel(m) {
  if (!m) return "";
  if (m.discord_channel_name) return "#" + m.discord_channel_name;
  return m.discord_channel_id || "";
}

function fillResyncSelect(listens) {
  const sel = document.getElementById("resync-channel");
  const prev = sel.value || lastResyncChannel;
  const options = ['<option value="">— select listener —</option>'];
  for (const m of listens || []) {
    const off = m.enabled ? "" : " (paused)";
    const ch = channelLabel(m);
    const label = (m.name ? m.name + " · " : "") + ch + off;
    options.push(
      `<option value="${esc(m.discord_channel_id)}">${esc(label)}</option>`
    );
  }
  sel.innerHTML = options.join("");
  if (prev && [...sel.options].some((o) => o.value === prev)) {
    sel.value = prev;
  }
}

function fillPictureResyncSelect(listens) {
  const sel = document.getElementById("picture-resync-channel");
  if (!sel) return;
  const prev = sel.value || lastPictureResyncChannel;
  const options = ['<option value="">— select picture listener —</option>'];
  for (const p of listens || []) {
    const off = p.enabled ? "" : " (paused)";
    const ch = channelLabel(p);
    const label = (p.name ? p.name + " · " : "") + ch + " · " + (p.slug || "") + off;
    options.push(
      `<option value="${esc(p.discord_channel_id)}">${esc(label)}</option>`
    );
  }
  sel.innerHTML = options.join("");
  if (prev && [...sel.options].some((o) => o.value === prev)) {
    sel.value = prev;
  }
}

async function fillGuildSelect(sel) {
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

async function loadGuilds() {
  await Promise.all([
    fillGuildSelect(document.getElementById("guild-select")),
    fillGuildSelect(document.getElementById("pic-guild-select")),
  ]);
}

async function loadChannelsForGuild(guildID, channelSelectId) {
  const sel = document.getElementById(channelSelectId);
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
    const picOn = s.picture_listens_enabled ?? 0;
    const picTot = s.picture_listens_total ?? 0;
    host.replaceChildren(
      discordStatusPill(!!s.discord_connected),
      pill(s.youtube_authorized ? "youtube ok" : "youtube missing", !!s.youtube_authorized),
      pill(`content ${on}/${tot}`, true),
      pill(`pics ${picOn}/${picTot}`, true),
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
    cachedListens = rows;
    fillResyncSelect(rows);
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="6" class="empty">no content listeners — start one above</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((m) => {
        const ch = esc(m.discord_channel_id);
        const chLabel = m.discord_channel_name
          ? `#${esc(m.discord_channel_name)}`
          : ch;
        const state = m.enabled
          ? `<span class="state-on">LISTENING</span>`
          : `<span class="state-off">PAUSED</span>`;
        const since = m.active_from ? new Date(m.active_from).toLocaleString() : "—";
        return `<tr>
          <td>${esc(m.name) || "—"}</td>
          <td class="mono" title="${ch}">${chLabel}</td>
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
    cachedListens = [];
    tbody.innerHTML = `<tr><td colspan="6" class="empty">failed: ${esc(err.message)}</td></tr>`;
  }
}

async function loadPictureListens() {
  const tbody = document.querySelector("#picture-listens-table tbody");
  try {
    const data = await api("/api/picture-listens");
    const rows = data.picture_listens || [];
    cachedPictureListens = rows;
    fillPictureResyncSelect(rows);
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="7" class="empty">no picture listeners — start one above</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((p) => {
        const ch = esc(p.discord_channel_id);
        const chLabel = p.discord_channel_name
          ? `#${esc(p.discord_channel_name)}`
          : ch;
        const state = p.enabled
          ? `<span class="state-on">LISTENING</span>`
          : `<span class="state-off">PAUSED</span>`;
        const since = p.active_from ? new Date(p.active_from).toLocaleString() : "—";
        const url = p.slideshow_url || ("/slideshow/" + p.slug);
        return `<tr>
          <td>${esc(p.name) || "—"}<br><span class="mono">${esc(p.slug)}</span></td>
          <td class="mono" title="${ch}">${chLabel}</td>
          <td class="mono">${esc(p.picture_count)}</td>
          <td><a href="${esc(url)}" target="_blank" rel="noopener">${esc(url)}</a></td>
          <td class="mono">${esc(since)}</td>
          <td>${state}</td>
          <td class="actions">
            <button type="button" class="secondary" data-pact="settings" data-channel="${ch}">Settings</button>
            <button type="button" class="secondary" data-pact="toggle" data-channel="${ch}" data-enabled="${p.enabled}">
              ${p.enabled ? "Pause" : "Resume"}
            </button>
            <button type="button" class="secondary" data-pact="resync" data-channel="${ch}">Resync</button>
            <button type="button" class="danger" data-pact="delete" data-channel="${ch}">Cease</button>
          </td>
        </tr>`;
      })
      .join("");
  } catch (err) {
    cachedPictureListens = [];
    fillPictureResyncSelect([]);
    tbody.innerHTML = `<tr><td colspan="7" class="empty">failed: ${esc(err.message)}</td></tr>`;
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

async function loadPictureAnnounce() {
  try {
    const data = await api("/api/settings/picture-listen-messages");
    document.getElementById("picture-start-message").value = data.start_message || "";
    document.getElementById("picture-stop-message").value = data.stop_message || "";
  } catch (err) {
    toast("picture notices load failed: " + err.message, true);
  }
}

async function refreshAll() {
  await Promise.all([loadStatus(), loadListens(), loadPictureListens(), loadActivity()]);
}

document.getElementById("guild-select").addEventListener("change", (ev) => {
  loadChannelsForGuild(ev.target.value, "channel-select");
});
document.getElementById("pic-guild-select").addEventListener("change", (ev) => {
  loadChannelsForGuild(ev.target.value, "pic-channel-select");
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

  const existing = cachedListens.find(
    (m) => m.discord_channel_id === body.discord_channel_id
  );
  if (existing) {
    const ch = channelLabel(existing);
    const codename = existing.name ? ` (“${existing.name}”)` : "";
    const ok = confirm(
      `Channel ${ch} already has a live content listener${codename}. ` +
        "Starting again closes that epoch and creates a new YouTube playlist. Continue?"
    );
    if (!ok) return;
  }

  btn.disabled = true;
  try {
    const m = await api("/api/listens", { method: "POST", body: JSON.stringify(body) });
    ev.target.reset();
    ev.target.querySelector('[name="enabled"]').checked = true;
    document.getElementById("channel-select").disabled = true;
    document.getElementById("channel-select").innerHTML =
      `<option value="">— pick a server first —</option>`;
    await loadGuilds();
    toast("content listener · " + (m.youtube_playlist_id || "ok"));
    await refreshAll();
  } catch (err) {
    toast("start content listener failed: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("picture-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const btn = ev.target.querySelector('button[type="submit"]');
  const body = {
    name: fd.get("name") || "",
    guild_id: fd.get("guild_id") || "",
    discord_channel_id: fd.get("discord_channel_id"),
    enabled: fd.get("enabled") === "on",
  };

  const existing = cachedPictureListens.find(
    (m) => m.discord_channel_id === body.discord_channel_id
  );
  if (existing) {
    const ch = channelLabel(existing);
    const ok = confirm(
      `Channel ${ch} already has a live picture listener (“${existing.name}” / ${existing.slug}). ` +
        "Starting with a new name/slug closes that epoch. Continue?"
    );
    if (!ok) return;
  }

  btn.disabled = true;
  try {
    const data = await api("/api/picture-listens", {
      method: "POST",
      body: JSON.stringify(body),
    });
    const p = data.picture_listen || {};
    ev.target.reset();
    ev.target.querySelector('[name="enabled"]').checked = true;
    document.getElementById("pic-channel-select").disabled = true;
    document.getElementById("pic-channel-select").innerHTML =
      `<option value="">— pick a server first —</option>`;
    await loadGuilds();
    toast("picture listener · /slideshow/" + (p.slug || "ok"));
    await refreshAll();
  } catch (err) {
    toast("start picture listener failed: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("announce-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const form = ev.target;
  const btn = form.querySelector('button[type="submit"]');
  const fd = new FormData(form);
  btn.disabled = true;
  try {
    await api("/api/settings/listen-messages", {
      method: "PUT",
      body: JSON.stringify({
        start_message: fd.get("start_message") || "",
        stop_message: fd.get("stop_message") || "",
      }),
    });
    toast("notices saved");
  } catch (err) {
    toast("save failed: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("picture-announce-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const form = ev.target;
  const btn = form.querySelector('button[type="submit"]');
  const fd = new FormData(form);
  btn.disabled = true;
  try {
    await api("/api/settings/picture-listen-messages", {
      method: "PUT",
      body: JSON.stringify({
        start_message: fd.get("start_message") || "",
        stop_message: fd.get("stop_message") || "",
      }),
    });
    toast("picture notices saved");
  } catch (err) {
    toast("save failed: " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("listens-table").addEventListener("click", async (ev) => {
  const btn = ev.target.closest("button[data-act]");
  if (!btn || btn.disabled) return;
  const channel = btn.getAttribute("data-channel");
  const act = btn.getAttribute("data-act");
  btn.disabled = true;
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
      toast(enabled ? "content listener paused" : "content listener resumed");
    } else if (act === "delete") {
      if (!confirm("Cease content listener on channel " + channel + "? (epoch closes; history kept)")) return;
      await api("/api/listens/" + encodeURIComponent(channel), { method: "DELETE" });
      toast("content listener offline");
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
  } finally {
    // refreshAll rebuilds the table; if the button still exists, re-enable it.
    if (btn.isConnected) btn.disabled = false;
  }
});

document.getElementById("picture-listens-table").addEventListener("click", async (ev) => {
  const btn = ev.target.closest("button[data-pact]");
  if (!btn || btn.disabled) return;
  const channel = btn.getAttribute("data-channel");
  const act = btn.getAttribute("data-pact");
  if (act === "settings" || act === "resync") {
    // Navigation-only — no network mutate yet.
  } else {
    btn.disabled = true;
  }
  try {
    if (act === "toggle") {
      const enabled = btn.getAttribute("data-enabled") === "true";
      await api("/api/picture-listens/" + encodeURIComponent(channel), {
        method: "PATCH",
        body: JSON.stringify({ enabled: !enabled }),
      });
      toast(enabled ? "picture listener paused" : "picture listener resumed");
    } else if (act === "delete") {
      if (!confirm("Cease picture listener on channel " + channel + "? (epoch closes; saved pics stay on disk)")) return;
      await api("/api/picture-listens/" + encodeURIComponent(channel), { method: "DELETE" });
      toast("picture listener offline");
    } else if (act === "resync") {
      lastPictureResyncChannel = channel;
      const sel = document.getElementById("picture-resync-channel");
      if (sel) {
        if (![...sel.options].some((o) => o.value === channel)) {
          const opt = document.createElement("option");
          opt.value = channel;
          opt.textContent = channel;
          sel.appendChild(opt);
        }
        sel.value = channel;
      }
      document.getElementById("picture-resync-section").scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    } else if (act === "settings") {
      openPictureSettings(channel);
      return;
    }
    await refreshAll();
  } catch (err) {
    toast(err.message, true);
  } finally {
    if (btn.isConnected) btn.disabled = false;
  }
});

function openPictureSettings(channelID) {
  const p = cachedPictureListens.find((row) => row.discord_channel_id === channelID);
  if (!p) {
    toast("picture listener not found — refresh and try again", true);
    return;
  }
  const panel = document.getElementById("picture-settings-panel");
  document.getElementById("picture-settings-channel").value = p.discord_channel_id;
  document.getElementById("picture-settings-corner").value = p.credit_corner || "br";
  document.getElementById("picture-settings-interval").value = String(p.interval_seconds || 8);
  document.getElementById("picture-settings-transition").value = p.transition || "fade";
  document.getElementById("picture-settings-shuffle").checked = !!p.shuffle;
  document.getElementById("picture-settings-credit").checked = p.show_credit !== false;
  document.getElementById("picture-settings-reactions").checked = p.show_reactions !== false;
  document.getElementById("picture-settings-animated").checked = p.reactions_animated !== false;
  document.getElementById("picture-settings-multiplier").innerHTML =
    reactionMultiplierOptionsHTML(p.reaction_multiplier || 1);
  syncAnimatedMultiplierVisibility();
  const creditScale = Number(p.credit_scale) > 0 ? Number(p.credit_scale) : 1.5;
  const reactionScale = Number(p.reaction_scale) > 0 ? Number(p.reaction_scale) : 1.5;
  document.getElementById("picture-settings-credit-scale").innerHTML =
    overlayScaleOptionsHTML(creditScale);
  document.getElementById("picture-settings-reaction-scale").innerHTML =
    overlayScaleOptionsHTML(reactionScale);
  const ch = channelLabel(p);
  document.getElementById("picture-settings-target").textContent =
    (p.name || "unnamed") + " · " + (p.slug || "") + " · " + ch;
  panel.hidden = false;
  panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

function closePictureSettings() {
  document.getElementById("picture-settings-panel").hidden = true;
  document.getElementById("picture-settings-channel").value = "";
}

document.getElementById("picture-settings-cancel").addEventListener("click", () => {
  closePictureSettings();
});

document.getElementById("picture-settings-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const channel = String(fd.get("discord_channel_id") || "").trim();
  if (!channel) {
    toast("no picture listener selected", true);
    return;
  }
  const btn = ev.target.querySelector('button[type="submit"]');
  btn.disabled = true;
  try {
    await api("/api/picture-listens/" + encodeURIComponent(channel), {
      method: "PATCH",
      body: JSON.stringify({
        credit_corner: fd.get("credit_corner") || "br",
        interval_seconds: Number(fd.get("interval_seconds") || 8),
        transition: fd.get("transition") || "fade",
        shuffle: fd.get("shuffle") === "on",
        show_credit: fd.get("show_credit") === "on",
        show_reactions: fd.get("show_reactions") === "on",
        reactions_animated: fd.get("reactions_animated") === "on",
        reaction_multiplier: Number(fd.get("reaction_multiplier") || 1),
        credit_scale: Number(fd.get("credit_scale") || 1.5),
        reaction_scale: Number(fd.get("reaction_scale") || 1.5),
      }),
    });
    toast("slideshow settings saved");
    closePictureSettings();
    await refreshAll();
  } catch (err) {
    toast(err.message, true);
  } finally {
    btn.disabled = false;
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

document.getElementById("picture-resync-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const channel = fd.get("channel_id");
  if (!channel) {
    toast("pick a picture listener", true);
    return;
  }
  lastPictureResyncChannel = String(channel);
  const out = document.getElementById("picture-resync-result");
  const btn = ev.target.querySelector('button[type="submit"]');
  out.hidden = false;
  out.classList.remove("err");
  out.textContent = "running…";
  btn.disabled = true;
  try {
    const data = await api("/api/picture-resync", {
      method: "POST",
      body: JSON.stringify({
        channel_id: channel,
        limit: Number(fd.get("limit") || 100),
      }),
    });
    out.textContent = `scanned=${data.messages_scanned} saved=${data.saved} skipped=${data.skipped} failed=${data.failed}`;
    toast("picture resync done");
    await refreshAll();
  } catch (err) {
    out.classList.add("err");
    out.textContent = err.message;
    toast(err.message, true);
  } finally {
    btn.disabled = false;
  }
});

document.getElementById("picture-resync-channel").addEventListener("change", (ev) => {
  lastPictureResyncChannel = ev.target.value || "";
});

initTabs();
fillOverlayScaleSelects();
loadGuilds();
loadAnnounce();
loadPictureAnnounce();
refreshAll();
setInterval(refreshAll, 15000);

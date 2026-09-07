/* Subotto Admin UI — vanilla JS for a sovereign Meat Bag.
   Talks to /api/... with browser Basic Auth (collected on page load).
   Product language: LISTENING POSTS (start listen / cease listen). */

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
  const options = ['<option value="">— select listen —</option>'];
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
      tbody.innerHTML = `<tr><td colspan="6" class="empty">no listening posts — start listen above</td></tr>`;
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
    toast("listening · " + (m.youtube_playlist_id || "ok"));
    await refreshAll();
  } catch (err) {
    toast("start listen failed: " + err.message, true);
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
      toast(enabled ? "listen paused" : "listen resumed");
    } else if (act === "delete") {
      if (!confirm("Cease listen on channel " + channel + "? (epoch closes; history kept)")) return;
      await api("/api/listens/" + encodeURIComponent(channel), { method: "DELETE" });
      toast("listening post offline");
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
    toast("pick a listen", true);
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

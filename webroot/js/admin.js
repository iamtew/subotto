/* Subotto Admin UI — vanilla JS, talks to /api/... with browser Basic Auth
   (the browser already collected credentials when the page loaded). */

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

function pill(label, ok) {
  const span = document.createElement("span");
  span.className = "pill " + (ok ? "ok" : "bad");
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

async function loadStatus() {
  const host = document.getElementById("status-pills");
  try {
    const s = await api("/api/status");
    host.replaceChildren(
      pill(s.discord_connected ? "Discord online" : "Discord offline", !!s.discord_connected),
      pill(s.youtube_authorized ? "YouTube authorized" : "YouTube missing", !!s.youtube_authorized),
      pill(`${s.mappings_enabled}/${s.mappings_total} mappings on`, true),
      pill(`${s.activity_total} activity rows`, true)
    );
    if (s.youtube_channel) {
      host.appendChild(pill(s.youtube_channel, true));
    }
    if (s.scheduler_enabled) {
      const hours = s.resync_interval_hours || "?";
      host.appendChild(pill(`scheduler every ${hours}h`, true));
      if (s.scheduler_last_error) {
        host.appendChild(pill("scheduler error", false));
      }
    } else {
      host.appendChild(pill("scheduler off", true));
    }
  } catch (err) {
    host.replaceChildren(pill("status failed: " + err.message, false));
  }
}

async function loadMappings() {
  const tbody = document.querySelector("#mappings-table tbody");
  try {
    const data = await api("/api/mappings");
    const rows = data.mappings || [];
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="5" class="empty">No mappings yet — add one above.</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((m) => {
        const ch = esc(m.discord_channel_id);
        return `<tr>
          <td>${esc(m.name)}</td>
          <td class="mono">${ch}</td>
          <td class="mono">${esc(m.youtube_playlist_id)}</td>
          <td>${m.enabled ? "yes" : "no"}</td>
          <td class="actions">
            <button type="button" class="secondary" data-act="toggle" data-channel="${ch}" data-enabled="${m.enabled}">
              ${m.enabled ? "Disable" : "Enable"}
            </button>
            <button type="button" class="secondary" data-act="resync" data-channel="${ch}">Resync</button>
            <button type="button" class="danger" data-act="delete" data-channel="${ch}">Delete</button>
          </td>
        </tr>`;
      })
      .join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="5" class="empty">Failed: ${esc(err.message)}</td></tr>`;
  }
}

async function loadActivity() {
  const tbody = document.querySelector("#activity-table tbody");
  try {
    const data = await api("/api/activity?limit=40");
    const rows = data.activity || [];
    if (!rows.length) {
      tbody.innerHTML = `<tr><td colspan="4" class="empty">No activity yet.</td></tr>`;
      return;
    }
    tbody.innerHTML = rows
      .map((e) => {
        const when = e.timestamp ? new Date(e.timestamp).toLocaleString() : "";
        const details = esc(JSON.stringify(e.details || {}));
        return `<tr>
          <td>${esc(when)}</td>
          <td class="mono">${esc(e.event_type)}</td>
          <td>${e.success ? "yes" : "no"}</td>
          <td class="mono">${details}</td>
        </tr>`;
      })
      .join("");
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="4" class="empty">Failed: ${esc(err.message)}</td></tr>`;
  }
}

async function refreshAll() {
  await Promise.all([loadStatus(), loadMappings(), loadActivity()]);
}

document.getElementById("mapping-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const body = {
    name: fd.get("name") || "",
    discord_channel_id: fd.get("discord_channel_id"),
    youtube_playlist_id: fd.get("youtube_playlist_id"),
    guild_id: fd.get("guild_id") || "",
    enabled: fd.get("enabled") === "on",
  };
  try {
    await api("/api/mappings", { method: "POST", body: JSON.stringify(body) });
    ev.target.reset();
    ev.target.querySelector('[name="enabled"]').checked = true;
    await refreshAll();
  } catch (err) {
    alert("Save failed: " + err.message);
  }
});

document.getElementById("mappings-table").addEventListener("click", async (ev) => {
  const btn = ev.target.closest("button[data-act]");
  if (!btn) return;
  const channel = btn.getAttribute("data-channel");
  const act = btn.getAttribute("data-act");
  try {
    if (act === "toggle") {
      const enabled = btn.getAttribute("data-enabled") === "true";
      await api("/api/mappings/" + encodeURIComponent(channel), {
        method: "PATCH",
        body: JSON.stringify({ enabled: !enabled }),
      });
    } else if (act === "delete") {
      if (!confirm("Delete mapping for channel " + channel + "?")) return;
      await api("/api/mappings/" + encodeURIComponent(channel), { method: "DELETE" });
    } else if (act === "resync") {
      const form = document.getElementById("resync-form");
      form.channel_id.value = channel;
      form.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    await refreshAll();
  } catch (err) {
    alert(err.message);
  }
});

document.getElementById("resync-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const out = document.getElementById("resync-result");
  out.hidden = false;
  out.classList.remove("err");
  out.textContent = "Running resync…";
  try {
    const data = await api("/api/resync", {
      method: "POST",
      body: JSON.stringify({
        channel_id: fd.get("channel_id"),
        limit: Number(fd.get("limit") || 100),
      }),
    });
    out.textContent = `scanned=${data.messages_scanned} added=${data.added} skipped=${data.skipped} failed=${data.failed}`;
    await refreshAll();
  } catch (err) {
    out.classList.add("err");
    out.textContent = err.message;
  }
});

refreshAll();
setInterval(refreshAll, 15000);

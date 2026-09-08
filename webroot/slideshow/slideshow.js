/* Subotto public OBS slideshow — polls /api/slideshow/{slug}. */

(function () {
  const slideEl = document.getElementById("slide");
  const creditEl = document.getElementById("credit");
  const nameEl = document.getElementById("credit-name");
  const reactionsEl = document.getElementById("credit-reactions");
  const floaterStage = document.getElementById("floater-stage");
  const emptyEl = document.getElementById("empty");

  const slug = slugFromPath();
  if (!slug) {
    emptyEl.hidden = false;
    emptyEl.textContent = "missing slideshow slug";
    return;
  }

  let feed = null;
  let order = [];
  let index = 0;
  let timer = null;
  let armedIntervalSec = null;
  let currentId = null;
  let floaterRAF = 0;
  let floaters = [];
  let lastTick = 0;

  const STATIC_STACK_MAX = 5;

  function slugFromPath() {
    const parts = location.pathname.split("/").filter(Boolean);
    if (parts.length >= 2 && parts[0] === "slideshow") {
      return decodeURIComponent(parts[1]);
    }
    return "";
  }

  async function refreshFeed() {
    try {
      const res = await fetch("/api/slideshow/" + encodeURIComponent(slug), {
        cache: "no-store",
      });
      if (!res.ok) throw new Error("feed " + res.status);
      const data = await res.json();
      const prevLen = feed && feed.images ? feed.images.length : 0;
      feed = data;
      rebuildOrder(prevLen === 0 || !order.length);
      applyChrome();
      if (!order.length) {
        showEmpty(true);
        stopAdvanceTimer();
        clearFloaters();
        clearStaticReactions();
        return;
      }
      showEmpty(false);
      if (currentId == null) {
        showAt(0);
      } else {
        const still = order.findIndex(function (id) {
          return String(id) === String(currentId);
        });
        if (still >= 0) {
          index = still;
          paintCurrent();
        } else {
          showAt(0);
        }
      }
      ensureAdvanceTimer();
    } catch (err) {
      console.warn("slideshow feed error", err);
      if (!feed || !feed.images || !feed.images.length) {
        showEmpty(true);
        emptyEl.textContent = "slideshow unavailable";
        stopAdvanceTimer();
        clearFloaters();
        clearStaticReactions();
      }
    }
  }

  function rebuildOrder(force) {
    const ids = (feed.images || []).map(function (img) {
      return img.id;
    });
    if (!force && order.length && sameSet(order, ids)) return;
    order = ids.slice();
    if (feed.shuffle) shuffleInPlace(order);
    index = 0;
  }

  function sameSet(a, b) {
    if (a.length !== b.length) return false;
    const set = {};
    for (let i = 0; i < a.length; i++) set[String(a[i])] = true;
    for (let i = 0; i < b.length; i++) if (!set[String(b[i])]) return false;
    return true;
  }

  function shuffleInPlace(arr) {
    for (let i = arr.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      const t = arr[i];
      arr[i] = arr[j];
      arr[j] = t;
    }
  }

  function cornerCode() {
    const c = (feed && feed.credit_corner ? feed.credit_corner : "br").toLowerCase();
    return ["tl", "tr", "bl", "br"].indexOf(c) >= 0 ? c : "br";
  }

  function reactionsAnimated() {
    // Default on when the field is missing (older feeds / first deploy).
    return feed.reactions_animated !== false;
  }

  function normalizeReactions(raw) {
    if (!raw) return [];
    if (Array.isArray(raw)) {
      const out = [];
      for (let i = 0; i < raw.length; i++) {
        const item = raw[i];
        if (!item) continue;
        const emoji = String(item.emoji || "").trim();
        const count = Number(item.count) || 0;
        if (!emoji || count < 1) continue;
        out.push({ emoji: emoji, count: count });
      }
      return out;
    }
    // Legacy object map — order already lost; alphabetical fallback.
    const keys = Object.keys(raw).sort();
    const out = [];
    for (let i = 0; i < keys.length; i++) {
      const count = Number(raw[keys[i]]) || 0;
      if (count < 1) continue;
      out.push({ emoji: keys[i], count: count });
    }
    return out;
  }

  function applyChrome() {
    const corner = cornerCode();
    creditEl.classList.remove("tl", "tr", "bl", "br");
    creditEl.classList.add(corner);

    const cs = Number(feed.credit_scale);
    const rs = Number(feed.reaction_scale);
    const creditScale = cs > 0 ? cs : 1.5;
    const reactionScale = rs > 0 ? rs : 1.5;
    creditEl.style.setProperty("--credit-scale", String(creditScale));
    creditEl.style.setProperty("--reaction-scale", String(reactionScale));
    floaterStage.style.setProperty("--reaction-scale", String(reactionScale));
  }

  function imageById(id) {
    const key = String(id);
    for (let i = 0; i < feed.images.length; i++) {
      if (String(feed.images[i].id) === key) return feed.images[i];
    }
    return null;
  }

  function showAt(i) {
    if (!order.length) return;
    index = ((i % order.length) + order.length) % order.length;
    currentId = order[index];
    paintCurrent();
  }

  function paintCurrent() {
    const img = imageById(currentId);
    if (!img) return;
    if (slideEl.getAttribute("src") !== img.url) {
      slideEl.onload = function () {
        slideEl.hidden = false;
      };
      slideEl.src = img.url;
    } else {
      slideEl.hidden = false;
    }

    const showCredit = !!feed.show_credit;
    const showReact = !!feed.show_reactions;
    if (!showCredit && !showReact) {
      creditEl.hidden = true;
      clearFloaters();
      clearStaticReactions();
      return;
    }

    creditEl.hidden = false;

    if (showCredit) {
      nameEl.hidden = false;
      const author = (img.author || "").trim();
      nameEl.textContent = author ? "Author: " + author : "Author:";
    } else {
      nameEl.hidden = true;
      nameEl.textContent = "";
    }

    const reactions = normalizeReactions(img.reactions);
    if (!showReact || !reactions.length) {
      clearFloaters();
      clearStaticReactions();
      return;
    }

    if (reactionsAnimated()) {
      clearStaticReactions();
      floaterStage.hidden = false;
      rebuildFloaters(reactions);
    } else {
      clearFloaters();
      floaterStage.hidden = true;
      rebuildStaticStack(reactions);
    }
  }

  function parseReactionKey(key) {
    let m = /^a:([^:]+):(\d+)$/.exec(key);
    if (m) {
      return { kind: "discord", name: m[1], id: m[2], animated: true };
    }
    m = /^([^:]+):(\d+)$/.exec(key);
    if (m) {
      return { kind: "discord", name: m[1], id: m[2], animated: false };
    }
    return { kind: "unicode", emoji: key };
  }

  function discordEmojiUrl(id, animated) {
    if (animated) {
      return "https://cdn.discordapp.com/emojis/" + id + ".gif?size=64&quality=lossless";
    }
    return "https://cdn.discordapp.com/emojis/" + id + ".webp?size=64&quality=lossless";
  }

  function twemojiUrl(emoji) {
    const cps = [];
    for (const ch of emoji) {
      const cp = ch.codePointAt(0);
      if (cp === 0xfe0f) continue;
      cps.push(cp.toString(16));
    }
    return (
      "https://cdn.jsdelivr.net/gh/jdecked/twemoji@15.1.0/assets/72x72/" +
      cps.join("-") +
      ".png"
    );
  }

  function makeEmoteImg(parsed) {
    const img = document.createElement("img");
    img.alt = "";
    img.draggable = false;
    if (parsed.kind === "discord") {
      img.src = discordEmojiUrl(parsed.id, parsed.animated);
      img.onerror = function () {
        img.onerror = null;
        img.src = "https://cdn.discordapp.com/emojis/" + parsed.id + ".png?size=64";
      };
    } else {
      img.src = twemojiUrl(parsed.emoji);
      img.onerror = function () {
        img.style.opacity = "0";
      };
    }
    return img;
  }

  function clearStaticReactions() {
    reactionsEl.innerHTML = "";
    reactionsEl.hidden = true;
  }

  // Static mode: Discord-ordered stack after the author name (max 5).
  function rebuildStaticStack(reactions) {
    reactionsEl.innerHTML = "";
    const limit = Math.min(STATIC_STACK_MAX, reactions.length);
    if (limit < 1) {
      reactionsEl.hidden = true;
      return;
    }
    for (let i = 0; i < limit; i++) {
      const item = reactions[i];
      const wrap = document.createElement("span");
      wrap.className = "static-react";
      wrap.appendChild(makeEmoteImg(parseReactionKey(item.emoji)));
      // Discord-style: no digit when there's only one of that react.
      if (item.count > 1) {
        const countEl = document.createElement("span");
        countEl.className = "static-react-count";
        countEl.textContent = String(item.count);
        wrap.appendChild(countEl);
      }
      reactionsEl.appendChild(wrap);
    }
    reactionsEl.hidden = false;
  }

  function rebuildFloaters(reactions) {
    clearFloaters(false);
    const mult = Math.max(1, Math.min(25, Number(feed.reaction_multiplier) || 1));
    const nodes = [];
    for (let i = 0; i < reactions.length; i++) {
      const count = Number(reactions[i].count) || 0;
      if (count < 1) continue;
      // Soft cap grows with multiplier so 25x can flood the stage
      const cap = Math.min(280, 48 + mult * 10);
      const copies = Math.min(cap, count * mult);
      const parsed = parseReactionKey(reactions[i].emoji);
      for (let n = 0; n < copies; n++) {
        nodes.push(spawnFloater(parsed));
      }
    }
    floaters = nodes;
    lastTick = 0;
    if (!floaterRAF && floaters.length) {
      floaterRAF = requestAnimationFrame(tickFloaters);
    }
  }

  // Fountain: burst from viewport center with upward impulse, then gravity.
  function spawnFloater(parsed) {
    const el = document.createElement("div");
    el.className = "float-emote pop";
    el.appendChild(makeEmoteImg(parsed));
    floaterStage.appendChild(el);
    return resetFountainState({
      el: el,
      parsed: parsed,
      rot: Math.random() * 360,
      spin: (Math.random() - 0.5) * 2.2,
    });
  }

  function resetFountainState(f) {
    // Percent of stage; small jitter so they don't all stack on one pixel.
    f.x = 50 + (Math.random() - 0.5) * 6;
    f.y = 52 + (Math.random() - 0.5) * 4;
    // Upward launch + horizontal spray — higher mult looks wild via more particles.
    f.vx = (Math.random() - 0.5) * 28;
    f.vy = -(14 + Math.random() * 22);
    f.born = performance.now();
    f.life = (2.4 + Math.random() * 2.8) * 1000;
    f.el.classList.remove("pop");
    void f.el.offsetWidth;
    f.el.classList.add("pop");
    f.el.style.opacity = "1";
    return f;
  }

  function clearFloaters(cancelRAF) {
    if (cancelRAF !== false && floaterRAF) {
      cancelAnimationFrame(floaterRAF);
      floaterRAF = 0;
    }
    floaters = [];
    floaterStage.innerHTML = "";
  }

  function respawnFloater(f) {
    resetFountainState(f);
  }

  function tickFloaters(now) {
    const dt = lastTick ? Math.min(0.05, (now - lastTick) / 1000) : 0.016;
    lastTick = now;
    const gravity = 38; // % of stage height per second²

    for (let i = 0; i < floaters.length; i++) {
      const f = floaters[i];
      const age = now - f.born;

      f.vy += gravity * dt;
      f.x += f.vx * dt;
      f.y += f.vy * dt;
      f.rot += f.spin;

      const offscreen =
        f.y > 118 || f.y < -18 || f.x < -18 || f.x > 118 || age >= f.life;
      if (offscreen) {
        respawnFloater(f);
      }

      // Fade in the last stretch of life (respawn resets opacity).
      const fadeStart = f.life * 0.7;
      let opacity = 1;
      const curAge = now - f.born;
      if (curAge > fadeStart) {
        opacity = Math.max(0, 1 - (curAge - fadeStart) / (f.life - fadeStart));
      }

      f.el.style.left = f.x + "%";
      f.el.style.top = f.y + "%";
      f.el.style.opacity = String(opacity);
      f.el.style.transform = "translate(-50%, -50%) rotate(" + f.rot + "deg)";
    }
    floaterRAF = requestAnimationFrame(tickFloaters);
  }

  function next() {
    if (!order.length) return;
    showAt(index + 1);
  }

  function stopAdvanceTimer() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
    armedIntervalSec = null;
  }

  function ensureAdvanceTimer() {
    const sec = Math.max(1, Number(feed.interval_seconds) || 8);
    if (timer && armedIntervalSec === sec) return;
    if (timer) clearInterval(timer);
    armedIntervalSec = sec;
    timer = setInterval(next, sec * 1000);
  }

  function showEmpty(on) {
    emptyEl.hidden = !on;
    if (on) {
      slideEl.hidden = true;
      creditEl.hidden = true;
    }
  }

  refreshFeed();
  setInterval(refreshFeed, 5000);
})();

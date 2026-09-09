/* Subotto public OBS slideshow — polls /api/slideshow/{slug}.
   Third-pass potato build: atomic slide+overlay commits, floater DOM pool,
   quantized style writes, no rebuild unless slide/reactions/settings change. */

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
  let paintedKey = "";
  let pendingKey = "";
  let paintGen = 0;
  const emoteTemplates = {};
  const preloaded = {};

  const STATIC_STACK_MAX = 5;
  const FLOATER_HARD_CAP = 56;
  const FRAME_BUDGET_MS = 20;
  let skipNextFrame = false;

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
        clearFloaters(true);
        clearStaticReactions();
        paintedKey = "";
        pendingKey = "";
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
      preloadNearby();
    } catch (err) {
      console.warn("slideshow feed error", err);
      if (!feed || !feed.images || !feed.images.length) {
        showEmpty(true);
        emptyEl.textContent = "slideshow unavailable";
        stopAdvanceTimer();
        clearFloaters(true);
        clearStaticReactions();
        paintedKey = "";
        pendingKey = "";
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
    const keys = Object.keys(raw).sort();
    const out = [];
    for (let i = 0; i < keys.length; i++) {
      const count = Number(raw[keys[i]]) || 0;
      if (count < 1) continue;
      out.push({ emoji: keys[i], count: count });
    }
    return out;
  }

  function reactionsKey(reactions) {
    let s = "";
    for (let i = 0; i < reactions.length; i++) {
      if (i) s += ",";
      s += reactions[i].emoji + ":" + reactions[i].count;
    }
    return s;
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
    preloadNearby();
  }

  function preloadUrl(url) {
    if (!url || preloaded[url]) return;
    const img = new Image();
    img.decoding = "async";
    img.onload = function () {
      preloaded[url] = true;
    };
    img.onerror = function () {
      preloaded[url] = true;
    };
    img.src = url;
  }

  function preloadNearby() {
    if (!feed || !order.length) return;
    const cur = imageById(order[index]);
    if (cur) preloadUrl(cur.url);
    if (order.length > 1) {
      const nxt = imageById(order[(index + 1) % order.length]);
      if (nxt) preloadUrl(nxt.url);
    }
  }

  function paintKeyFor(img, reactions, showCredit, showReact, animated, mult) {
    return (
      String(img.id) +
      "|" +
      img.url +
      "|" +
      (showCredit ? "1" : "0") +
      "|" +
      (showReact ? "1" : "0") +
      "|" +
      (animated ? "a" : "s") +
      "|" +
      mult +
      "|" +
      reactionsKey(reactions) +
      "|" +
      ((img.author || "").trim())
    );
  }

  function paintCurrent() {
    const img = imageById(currentId);
    if (!img) return;

    const showCredit = !!feed.show_credit;
    const showReact = !!feed.show_reactions;
    const reactions = showReact ? normalizeReactions(img.reactions) : [];
    const animated = reactionsAnimated();
    const mult = Math.max(1, Math.min(25, Number(feed.reaction_multiplier) || 1));
    const key = paintKeyFor(img, reactions, showCredit, showReact, animated, mult);

    if (key === paintedKey) {
      if (slideEl.getAttribute("src") === img.url) slideEl.hidden = false;
      return;
    }
    // Already loading this exact paint — don't stack duplicate preloads.
    if (key === pendingKey) return;

    const gen = ++paintGen;
    pendingKey = key;
    const url = img.url;
    const sameSrc = slideEl.getAttribute("src") === url;

    function commit() {
      if (gen !== paintGen) return;
      paintedKey = key;
      pendingKey = "";

      if (slideEl.getAttribute("src") !== url) {
        slideEl.src = url;
      }
      slideEl.hidden = false;

      if (!showCredit && !showReact) {
        creditEl.hidden = true;
        clearFloaters(false);
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

      if (!showReact || !reactions.length) {
        clearFloaters(false);
        clearStaticReactions();
        return;
      }

      if (animated) {
        clearStaticReactions();
        floaterStage.hidden = false;
        rebuildFloaters(reactions, mult);
      } else {
        clearFloaters(false);
        floaterStage.hidden = true;
        rebuildStaticStack(reactions);
      }
    }

    // Image + credit + floaters commit together once the bitmap is ready.
    if (sameSrc && (slideEl.complete || slideEl.naturalWidth > 0)) {
      commit();
      return;
    }
    if (preloaded[url]) {
      commit();
      return;
    }

    const pre = new Image();
    pre.decoding = "async";
    pre.onload = function () {
      preloaded[url] = true;
      commit();
    };
    pre.onerror = function () {
      preloaded[url] = true;
      commit();
    };
    pre.src = url;
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

  function discordEmojiUrl(id) {
    return "https://cdn.discordapp.com/emojis/" + id + ".webp?size=32";
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

  function bindEmoteError(img, parsed) {
    if (parsed.kind === "discord") {
      img.onerror = function () {
        img.onerror = null;
        img.src = "https://cdn.discordapp.com/emojis/" + parsed.id + ".png?size=32";
      };
    } else {
      img.onerror = function () {
        img.style.opacity = "0";
      };
    }
  }

  function emoteTemplate(key, parsed) {
    if (emoteTemplates[key]) return emoteTemplates[key];
    const img = document.createElement("img");
    img.alt = "";
    img.draggable = false;
    img.decoding = "async";
    img.loading = "eager";
    if (parsed.kind === "discord") {
      img.src = discordEmojiUrl(parsed.id);
    } else {
      img.src = twemojiUrl(parsed.emoji);
    }
    bindEmoteError(img, parsed);
    emoteTemplates[key] = img;
    return img;
  }

  function makeEmoteImg(key, parsed) {
    const img = emoteTemplate(key, parsed).cloneNode(true);
    bindEmoteError(img, parsed);
    return img;
  }

  function clearStaticReactions() {
    reactionsEl.innerHTML = "";
    reactionsEl.hidden = true;
  }

  function rebuildStaticStack(reactions) {
    reactionsEl.innerHTML = "";
    const limit = Math.min(STATIC_STACK_MAX, reactions.length);
    if (limit < 1) {
      reactionsEl.hidden = true;
      return;
    }
    const frag = document.createDocumentFragment();
    for (let i = 0; i < limit; i++) {
      const item = reactions[i];
      const wrap = document.createElement("span");
      wrap.className = "static-react";
      const parsed = parseReactionKey(item.emoji);
      wrap.appendChild(makeEmoteImg(item.emoji, parsed));
      if (item.count > 1) {
        const countEl = document.createElement("span");
        countEl.className = "static-react-count";
        countEl.textContent = String(item.count);
        wrap.appendChild(countEl);
      }
      frag.appendChild(wrap);
    }
    reactionsEl.appendChild(frag);
    reactionsEl.hidden = false;
  }

  function planFloaterKeys(reactions, mult) {
    const weights = [];
    let weightSum = 0;
    for (let i = 0; i < reactions.length; i++) {
      const count = Number(reactions[i].count) || 0;
      if (count < 1) continue;
      const w = count * mult;
      weights.push({ emoji: reactions[i].emoji, weight: w });
      weightSum += w;
    }
    if (!weightSum) return [];

    const total = Math.min(FLOATER_HARD_CAP, Math.max(1, Math.round(weightSum)));
    const keys = [];
    let spawned = 0;
    for (let i = 0; i < weights.length; i++) {
      let n;
      if (i === weights.length - 1) {
        n = total - spawned;
      } else {
        n = Math.round((weights[i].weight / weightSum) * total);
        if (n < 1 && weights[i].weight > 0 && spawned < total) n = 1;
        if (spawned + n > total) n = total - spawned;
      }
      for (let c = 0; c < n; c++) keys.push(weights[i].emoji);
      spawned += n;
      if (spawned >= total) break;
    }
    return keys;
  }

  // Reuse floater DOM nodes across slides — potato PCs hate createElement storms.
  function rebuildFloaters(reactions, mult) {
    const keys = planFloaterKeys(reactions, mult);
    const now = performance.now();
    const next = [];
    const frag = document.createDocumentFragment();

    for (let i = 0; i < keys.length; i++) {
      const key = keys[i];
      const parsed = parseReactionKey(key);
      let f = floaters[i];
      if (f) {
        if (f.key !== key) {
          f.el.textContent = "";
          f.el.appendChild(makeEmoteImg(key, parsed));
          f.key = key;
        }
        resetFountainState(f, now);
        f.born = now + (i % 4) * 12;
        f.el.style.opacity = "0";
        f.opacity = 0;
        f.waiting = true;
        f.tx = -1;
        f.ty = -1;
        f.tr = -9999;
        next.push(f);
      } else {
        next.push(spawnFloater(key, parsed, frag, now, i));
      }
    }

    for (let i = keys.length; i < floaters.length; i++) {
      const el = floaters[i].el;
      if (el && el.parentNode) el.parentNode.removeChild(el);
    }

    if (frag.childNodes.length) {
      floaterStage.appendChild(frag);
    }

    floaters = next;
    lastTick = 0;
    skipNextFrame = false;
    if (floaters.length && !floaterRAF) {
      floaterRAF = requestAnimationFrame(tickFloaters);
    }
    if (!floaters.length && floaterRAF) {
      cancelAnimationFrame(floaterRAF);
      floaterRAF = 0;
    }
  }

  function spawnFloater(key, parsed, parent, now, i) {
    const el = document.createElement("div");
    el.className = "float-emote";
    el.appendChild(makeEmoteImg(key, parsed));
    parent.appendChild(el);
    const f = resetFountainState({
      el: el,
      key: key,
      rot: Math.random() * 360,
      spin: (Math.random() - 0.5) * 1.4,
      tx: -1,
      ty: -1,
      tr: -9999,
    }, now);
    f.born = now + (i % 4) * 12;
    f.el.style.opacity = "0";
    f.opacity = 0;
    f.waiting = true;
    return f;
  }

  function resetFountainState(f, now) {
    const t = now || performance.now();
    f.x = 50 + (Math.random() - 0.5) * 6;
    f.y = 52 + (Math.random() - 0.5) * 4;
    f.vx = (Math.random() - 0.5) * 24;
    f.vy = -(11 + Math.random() * 18);
    f.born = t;
    f.life = (2.0 + Math.random() * 2.2) * 1000;
    f.opacity = 1;
    f.waiting = false;
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

  function tickFloaters(now) {
    if (!floaters.length) {
      floaterRAF = 0;
      return;
    }

    if (skipNextFrame) {
      skipNextFrame = false;
      lastTick = now;
      floaterRAF = requestAnimationFrame(tickFloaters);
      return;
    }

    const frameStart = now;
    const dt = lastTick ? Math.min(0.033, (now - lastTick) / 1000) : 0.016;
    lastTick = now;
    const gravity = 34;
    const list = floaters;
    const n = list.length;

    for (let i = 0; i < n; i++) {
      const f = list[i];
      if (now < f.born) {
        if (!f.waiting) {
          f.waiting = true;
          f.opacity = 0;
          f.el.style.opacity = "0";
        }
        continue;
      }
      f.waiting = false;

      f.vy += gravity * dt;
      f.x += f.vx * dt;
      f.y += f.vy * dt;
      f.rot += f.spin;

      const age = now - f.born;
      if (f.y > 112 || f.y < -16 || f.x < -16 || f.x > 116 || age >= f.life) {
        resetFountainState(f, now);
      }

      const curAge = now - f.born;
      const fadeStart = f.life * 0.75;
      let opacity = 1;
      if (curAge > fadeStart) {
        opacity = Math.max(0, 1 - (curAge - fadeStart) / (f.life - fadeStart));
      }
      // Quantize opacity to cut style thrash.
      opacity = ((opacity * 20) | 0) / 20;

      // Quantize position (~0.1 unit) / rotation (1°) before touching the DOM.
      const qx = (f.x * 10 + 0.5) | 0;
      const qy = (f.y * 10 + 0.5) | 0;
      const qr = f.rot | 0;
      if (qx !== f.tx || qy !== f.ty || qr !== f.tr) {
        f.tx = qx;
        f.ty = qy;
        f.tr = qr;
        f.el.style.transform =
          "translate3d(" + qx / 10 + "vw," + qy / 10 + "vh,0) translate(-50%,-50%) rotate(" +
          qr +
          "deg)";
      }
      if (opacity !== f.opacity) {
        f.opacity = opacity;
        f.el.style.opacity = opacity === 1 ? "1" : String(opacity);
      }
    }

    if (performance.now() - frameStart > FRAME_BUDGET_MS) {
      skipNextFrame = true;
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
      floaterStage.hidden = true;
    }
  }

  slideEl.decoding = "async";

  refreshFeed();
  setInterval(refreshFeed, 5000);
})();

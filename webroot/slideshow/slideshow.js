/* Subotto public OBS slideshow — polls /api/slideshow/{slug}. */

(function () {
  const slideEl = document.getElementById("slide");
  const creditEl = document.getElementById("credit");
  const nameEl = document.getElementById("credit-name");
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

  function isTopCorner(c) {
    return c === "tl" || c === "tr";
  }

  function applyChrome() {
    const corner = cornerCode();
    creditEl.classList.remove("tl", "tr", "bl", "br");
    creditEl.classList.add(corner);
    floaterStage.classList.remove("tl", "tr", "bl", "br");
    floaterStage.classList.add(corner);

    const cs = Number(feed.credit_scale);
    const rs = Number(feed.reaction_scale);
    const creditScale = cs > 0 ? cs : 1.5;
    const reactionScale = rs > 0 ? rs : 1.5;
    creditEl.style.setProperty("--credit-scale", String(creditScale));
    creditEl.style.setProperty("--reaction-scale", String(reactionScale));
    floaterStage.style.setProperty("--reaction-scale", String(reactionScale));

    layoutFloaterStage();
  }

  // 1x = compact zone near credit; 25x = full viewport. Linear in between.
  function layoutFloaterStage() {
    const mult = Math.max(1, Math.min(25, Number(feed.reaction_multiplier) || 1));
    const t = (mult - 1) / 24; // 0 at 1x → 1 at 25x
    const baseW = 22; // vw at 1x
    const baseH = 18; // vh at 1x
    const w = baseW + (100 - baseW) * t;
    const h = baseH + (100 - baseH) * t;
    const corner = cornerCode();

    floaterStage.style.width = w + "vw";
    floaterStage.style.height = h + "vh";
    floaterStage.style.top = "";
    floaterStage.style.right = "";
    floaterStage.style.bottom = "";
    floaterStage.style.left = "";

    if (corner === "tl") {
      floaterStage.style.top = "0";
      floaterStage.style.left = "0";
    } else if (corner === "tr") {
      floaterStage.style.top = "0";
      floaterStage.style.right = "0";
    } else if (corner === "bl") {
      floaterStage.style.bottom = "0";
      floaterStage.style.left = "0";
    } else {
      floaterStage.style.bottom = "0";
      floaterStage.style.right = "0";
    }
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
      return;
    }

    if (showCredit) {
      creditEl.hidden = false;
      const author = (img.author || "").trim();
      nameEl.hidden = false;
      nameEl.textContent = author ? "Author: " + author : "Author:";
    } else {
      creditEl.hidden = true;
    }

    if (showReact) {
      rebuildFloaters(img.reactions || {});
    } else {
      clearFloaters();
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

  function rebuildFloaters(reactions) {
    clearFloaters(false);
    layoutFloaterStage();
    const mult = Math.max(1, Math.min(25, Number(feed.reaction_multiplier) || 1));
    const keys = Object.keys(reactions).sort();
    const nodes = [];
    for (let i = 0; i < keys.length; i++) {
      const key = keys[i];
      const count = Number(reactions[key]) || 0;
      if (count < 1) continue;
      // Soft cap grows with multiplier so 25x can fill the stage
      const cap = Math.min(220, 40 + mult * 8);
      const copies = Math.min(cap, count * mult);
      const parsed = parseReactionKey(key);
      for (let n = 0; n < copies; n++) {
        nodes.push(spawnFloater(parsed));
      }
    }
    floaters = nodes;
    if (!floaterRAF && floaters.length) {
      floaterRAF = requestAnimationFrame(tickFloaters);
    }
  }

  function spawnFloater(parsed) {
    const el = document.createElement("div");
    el.className = "float-emote pop";
    el.appendChild(makeEmoteImg(parsed));
    floaterStage.appendChild(el);

    const corner = cornerCode();
    const top = isTopCorner(corner);
    // Top corners: float above + below author. Bottom: mostly above (toward center).
    const angle = Math.random() * Math.PI * 2;
    const radius = 18 + Math.random() * (top ? 48 : 40);
    const life = 2.8 + Math.random() * 3.4; // seconds until fade-out respawn

    return {
      el: el,
      parsed: parsed,
      angle: angle,
      radius: radius,
      spin: (Math.random() - 0.5) * 1.4,
      wobble: 0.35 + Math.random() * 0.9,
      speed: 0.25 + Math.random() * 0.55,
      phase: Math.random() * Math.PI * 2,
      rot: Math.random() * 360,
      born: performance.now(),
      life: life * 1000,
      topBias: top,
    };
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
    f.born = performance.now();
    f.life = (2.8 + Math.random() * 3.4) * 1000;
    f.angle = Math.random() * Math.PI * 2;
    f.radius = 18 + Math.random() * (f.topBias ? 48 : 40);
    f.phase = Math.random() * Math.PI * 2;
    f.rot = Math.random() * 360;
    f.el.classList.remove("pop");
    // Force reflow so pop animation can replay
    void f.el.offsetWidth;
    f.el.classList.add("pop");
    f.el.style.opacity = "1";
  }

  function tickFloaters(now) {
    const t = now * 0.001;
    for (let i = 0; i < floaters.length; i++) {
      const f = floaters[i];
      const age = now - f.born;
      if (age >= f.life) {
        respawnFloater(f);
      }

      const a = f.angle + t * f.speed * 0.35;
      const r = f.radius + Math.sin(t * f.wobble + f.phase) * 6;
      let cx = 50;
      let cy;
      if (f.topBias) {
        // Top corner: orbit around the credit (near top of floater stage) — above + below
        cy = 22;
      } else {
        // Bottom corner: keep floaters above the author card (toward upper part of zone)
        cy = 38;
      }
      const x = cx + Math.cos(a) * r * 0.55;
      const y = cy + Math.sin(a) * r * (f.topBias ? 0.55 : 0.38);

      // Fade out in the last ~35% of life; pop-in handles appear
      const fadeStart = f.life * 0.65;
      let opacity = 1;
      if (age > fadeStart) {
        opacity = Math.max(0, 1 - (age - fadeStart) / (f.life - fadeStart));
      }

      f.rot += f.spin;
      const scale = 0.85 + Math.sin(t * f.wobble + f.phase) * 0.12;
      f.el.style.left = x + "%";
      f.el.style.top = y + "%";
      f.el.style.opacity = String(opacity);
      f.el.style.transform =
        "translate(-50%, -50%) rotate(" + f.rot + "deg) scale(" + scale + ")";
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

  window.addEventListener("resize", function () {
    if (feed) layoutFloaterStage();
  });

  refreshFeed();
  setInterval(refreshFeed, 5000);
})();

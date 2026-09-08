/* Subotto public OBS slideshow — polls /api/slideshow/{slug}. */

(function () {
  const slideEl = document.getElementById("slide");
  const creditEl = document.getElementById("credit");
  const nameEl = document.getElementById("credit-name");
  const reactEl = document.getElementById("credit-reactions");
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
  let currentId = null;

  function slugFromPath() {
    const parts = location.pathname.split("/").filter(Boolean);
    // /slideshow/{slug}
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
      if (!res.ok) {
        throw new Error("feed " + res.status);
      }
      const data = await res.json();
      const prevLen = feed && feed.images ? feed.images.length : 0;
      feed = data;
      rebuildOrder(prevLen === 0 || !order.length);
      applyChrome();
      if (!order.length) {
        showEmpty(true);
        return;
      }
      showEmpty(false);
      if (currentId == null) {
        showAt(0);
      } else {
        // Keep current image if still present; refresh credit/reactions.
        const still = order.indexOf(currentId);
        if (still >= 0) {
          index = still;
          paintCurrent();
        } else {
          showAt(0);
        }
      }
      armTimer();
    } catch (err) {
      console.warn("slideshow feed error", err);
      if (!feed || !feed.images || !feed.images.length) {
        showEmpty(true);
        emptyEl.textContent = "slideshow unavailable";
      }
    }
  }

  function rebuildOrder(force) {
    const ids = (feed.images || []).map(function (img) { return img.id; });
    if (!force && order.length && sameSet(order, ids)) {
      return;
    }
    order = ids.slice();
    if (feed.shuffle) {
      shuffleInPlace(order);
    }
    index = 0;
  }

  function sameSet(a, b) {
    if (a.length !== b.length) return false;
    const set = {};
    for (let i = 0; i < a.length; i++) set[a[i]] = true;
    for (let i = 0; i < b.length; i++) if (!set[b[i]]) return false;
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

  function applyChrome() {
    creditEl.classList.remove("tl", "tr", "bl", "br");
    const corner = (feed.credit_corner || "br").toLowerCase();
    creditEl.classList.add(["tl", "tr", "bl", "br"].indexOf(corner) >= 0 ? corner : "br");
  }

  function imageById(id) {
    for (let i = 0; i < feed.images.length; i++) {
      if (feed.images[i].id === id) return feed.images[i];
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
      return;
    }
    creditEl.hidden = false;
    nameEl.hidden = !showCredit;
    nameEl.textContent = showCredit ? (img.author || "") : "";
    reactEl.hidden = !showReact;
    reactEl.innerHTML = "";
    if (showReact && img.reactions) {
      const keys = Object.keys(img.reactions).sort();
      for (let i = 0; i < keys.length; i++) {
        const key = keys[i];
        const count = img.reactions[key];
        if (!count) continue;
        const chip = document.createElement("span");
        chip.className = "react-chip";
        // Custom emoji keys look like name:id — show :name: style text.
        const label = key.indexOf(":") > 0 ? ":" + key.split(":")[0] + ":" : key;
        chip.textContent = label + " " + count;
        reactEl.appendChild(chip);
      }
    }
  }

  function next() {
    if (!order.length) return;
    showAt(index + 1);
  }

  function armTimer() {
    if (timer) clearInterval(timer);
    const sec = Math.max(1, Number(feed.interval_seconds) || 8);
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

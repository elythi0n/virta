/*
 * Leaderboard — Virta plugin GUI.
 *
 * Session leaderboards for top chatters and top emotes, tallied from the live event stream.
 * Sandboxed: this page makes no network calls itself (CSP blocks them); messages arrive through
 * the window.__virta postMessage bridge (events.subscribe → events.message). Tallies live in
 * memory only and reset when the panel closes.
 */
(function () {
  "use strict";
  var v = window.__virta;

  // Caps keep the tally maps bounded on long sessions; entries past the cap are dropped.
  var MAX_AUTHORS = 5000;
  var MAX_EMOTES = 2000;
  var RENDER_MS = 500; // coalesce re-renders instead of rendering per message

  var PLAT = {
    twitch: { tag: "TW", color: "#9146FF" },
    kick: { tag: "KK", color: "#53FC18" },
    x: { tag: "X", color: "#8A93A6" },
  };

  // ── state ───────────────────────────────────────────────────────────────────
  // chatters: platform + ":" + (authorId || author) → { name, color, platform, total, perChannel }
  // emotes:   emote code → { total, perChannel }
  var chatters = {};
  var chatterCount = 0;
  var emotes = {};
  var emoteCount = 0;
  var channels = {}; // observed "platform:slug" channel keys
  var tab = "chatters";
  var paused = false;
  var topN = 25;
  var channelFilter = ""; // "" = all channels
  var dirty = false;
  var channelsDirty = false;
  var failed = false;

  // ── tiny DOM helpers (textContent only — never innerHTML with stream data) ──
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }
  function fill(listEl, items) {
    listEl.textContent = "";
    items.forEach(function (i) { listEl.appendChild(i); });
  }

  // Platform author colors are picked for light backgrounds; on our dark panel the darkest ones
  // vanish. If the perceived luminance is under a floor, blend the color toward white until it
  // clears the floor.
  function clampColor(hex) {
    if (!hex || !/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.test(hex)) return "#E6EAF2";
    var h = hex.slice(1);
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    var r = parseInt(h.slice(0, 2), 16);
    var g = parseInt(h.slice(2, 4), 16);
    var b = parseInt(h.slice(4, 6), 16);
    var lum = 0.2126 * r + 0.7152 * g + 0.0722 * b; // perceived luminance, 0–255
    var FLOOR = 112;
    if (lum >= FLOOR) return hex;
    var t = (FLOOR - lum) / (255 - lum || 1);
    function lift(c) {
      var n = Math.round(c + (255 - c) * t);
      return (n < 16 ? "0" : "") + n.toString(16);
    }
    return "#" + lift(r) + lift(g) + lift(b);
  }

  // ── tallying ────────────────────────────────────────────────────────────────
  function onMessage(m) {
    if (!m || typeof m !== "object") return;
    var type = m.type || "chat";
    if (type !== "chat") return; // events (subs, raids, …) don't count toward tallies
    if (paused) return;

    var channel = typeof m.channel === "string" ? m.channel : "";
    if (channel && !Object.prototype.hasOwnProperty.call(channels, channel)) {
      channels[channel] = true;
      channelsDirty = true;
    }

    var key = (m.platform || "?") + ":" + (m.authorId || m.author || "");
    var c = Object.prototype.hasOwnProperty.call(chatters, key) ? chatters[key] : null;
    if (!c && chatterCount < MAX_AUTHORS) {
      c = chatters[key] = {
        name: m.author || "anon",
        color: m.authorColor || "",
        platform: m.platform || "",
        total: 0,
        perChannel: {},
      };
      chatterCount++;
    }
    if (c) {
      if (m.author) c.name = m.author; // track display-name changes
      if (m.authorColor) c.color = m.authorColor;
      c.total++;
      if (channel) c.perChannel[channel] = (c.perChannel[channel] || 0) + 1;
    }

    var segs = Array.isArray(m.segments) ? m.segments : [];
    for (var i = 0; i < segs.length; i++) {
      var seg = segs[i];
      if (!seg || (seg.type || seg.kind) !== "emote") continue;
      var code = seg.code || seg.name || seg.text;
      if (!code) continue;
      var e = Object.prototype.hasOwnProperty.call(emotes, code) ? emotes[code] : null;
      if (!e) {
        if (emoteCount >= MAX_EMOTES) continue;
        e = emotes[code] = { total: 0, perChannel: {} };
        emoteCount++;
      }
      e.total++;
      if (channel) e.perChannel[channel] = (e.perChannel[channel] || 0) + 1;
    }

    dirty = true;
  }

  function countOf(entry) {
    return channelFilter ? entry.perChannel[channelFilter] || 0 : entry.total;
  }

  function topRows(map) {
    var rows = [];
    Object.keys(map).forEach(function (key) {
      var n = countOf(map[key]);
      if (n > 0) rows.push({ key: key, entry: map[key], count: n });
    });
    rows.sort(function (a, b) { return b.count - a.count || (a.key < b.key ? -1 : 1); });
    return rows.slice(0, topN);
  }

  // ── rendering ───────────────────────────────────────────────────────────────
  function barEl(count, max) {
    var bar = el("div", "bar");
    var inner = el("span");
    inner.style.width = (max > 0 ? Math.max(1, (count / max) * 100) : 0) + "%";
    bar.appendChild(inner);
    return bar;
  }

  function chatterRow(row, i, max) {
    var li = el("li", "row");
    var line = el("div", "line");
    line.appendChild(el("span", "rank", String(i + 1)));
    var name = el("span", "name", row.entry.name);
    name.style.color = clampColor(row.entry.color);
    line.appendChild(name);
    var p = PLAT[row.entry.platform];
    var plat = el("span", "plat", p ? p.tag : (row.entry.platform || "?").slice(0, 2).toUpperCase());
    plat.style.color = p ? p.color : "#8A93A6";
    line.appendChild(plat);
    line.appendChild(el("span", "count", String(row.count)));
    li.appendChild(line);
    li.appendChild(barEl(row.count, max));
    return li;
  }

  function emoteRow(row, i, max) {
    var li = el("li", "row");
    var line = el("div", "line");
    line.appendChild(el("span", "rank", String(i + 1)));
    line.appendChild(el("span", "name", row.key));
    line.appendChild(el("span", "count", String(row.count)));
    li.appendChild(line);
    li.appendChild(barEl(row.count, max));
    return li;
  }

  function renderChannelOptions() {
    var sel = document.getElementById("channel");
    sel.textContent = "";
    var all = el("option", null, "All channels");
    all.value = "";
    sel.appendChild(all);
    Object.keys(channels).sort().forEach(function (ch) {
      var o = el("option", null, ch);
      o.value = ch;
      sel.appendChild(o);
    });
    sel.value = channelFilter;
  }

  function render() {
    dirty = false;
    if (channelsDirty) {
      channelsDirty = false;
      renderChannelOptions();
    }
    var board = document.getElementById("board");
    var empty = document.getElementById("empty");
    if (failed) {
      fill(board, []);
      return;
    }
    var rows = topRows(tab === "chatters" ? chatters : emotes);
    var max = rows.length ? rows[0].count : 0;
    fill(board, rows.map(function (row, i) {
      return tab === "chatters" ? chatterRow(row, i, max) : emoteRow(row, i, max);
    }));
    empty.style.display = rows.length ? "none" : "";
  }

  function showError(text) {
    failed = true;
    var empty = document.getElementById("empty");
    empty.textContent = text;
    empty.style.display = "";
    render();
  }

  // ── controls ────────────────────────────────────────────────────────────────
  function setTab(next) {
    tab = next;
    var tc = document.getElementById("tab-chatters");
    var te = document.getElementById("tab-emotes");
    tc.className = "tab" + (next === "chatters" ? " tab-active" : "");
    te.className = "tab" + (next === "emotes" ? " tab-active" : "");
    tc.setAttribute("aria-selected", String(next === "chatters"));
    te.setAttribute("aria-selected", String(next === "emotes"));
    render();
  }

  document.getElementById("tab-chatters").addEventListener("click", function () { setTab("chatters"); });
  document.getElementById("tab-emotes").addEventListener("click", function () { setTab("emotes"); });

  document.getElementById("pause").addEventListener("click", function () {
    paused = !paused;
    this.textContent = paused ? "Resume" : "Pause";
    this.className = paused ? "paused" : "";
  });

  document.getElementById("reset").addEventListener("click", function () {
    chatters = {};
    chatterCount = 0;
    emotes = {};
    emoteCount = 0;
    render();
  });

  document.getElementById("topn").addEventListener("change", function () {
    topN = parseInt(this.value, 10) || 25;
    render();
  });

  document.getElementById("channel").addEventListener("change", function () {
    channelFilter = this.value;
    render();
  });

  // ── boot ────────────────────────────────────────────────────────────────────
  if (!v) {
    showError("Virta bridge unavailable — reload the panel.");
    return;
  }

  setInterval(function () {
    if (dirty || channelsDirty) render();
  }, RENDER_MS);

  v.on("events.message", onMessage); // registered before subscribing so no message slips past
  v.send({ type: "events.subscribe", payload: {} })
    .catch(function (e) {
      v.off("events.message", onMessage);
      showError("Couldn't subscribe to the event stream" + (e && e.message ? " — " + e.message : "") + ".");
    });

  window.addEventListener("pagehide", function () {
    v.send({ type: "events.unsubscribe" }).catch(function () { /* closing anyway */ });
  });
})();

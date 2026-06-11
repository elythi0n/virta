/*
 * VOD Replay — Virta plugin GUI.
 *
 * Replays a Twitch VOD's chat against a local playback clock so reviews and reruns get
 * live-feeling chat. Sandboxed: the page itself makes no network calls (CSP blocks them);
 * every request goes through the window.__virta postMessage bridge as an http.fetch message,
 * and the daemon only allows the endpoint declared in the manifest (https://gql.twitch.tv/gql).
 */
(function () {
  "use strict";
  var v = window.__virta;

  var GQL_URL = "https://gql.twitch.tv/gql";
  var CLIENT_ID = "kimne78kx3ncx6brgo4mv6wki5h1ko"; // Twitch's public web Client-ID
  var COMMENTS_HASH = "b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a";

  var TICK_MS = 100; // playback clock granularity
  var MAX_LINES = 300; // visible feed cap; oldest lines are dropped
  var BUFFER_AHEAD_S = 120; // keep this many seconds of chat buffered past the clock
  var SEEK_BACKFILL = 20; // lines rendered immediately from before the seek point
  var FETCH_RETRY_MS = 3000; // back off this long after a failed page fetch

  // Fallback author colors for commenters without a userColor, hashed by name.
  var PALETTE = [
    "#FF6B6B", "#5B8CFF", "#51CF66", "#FCC419", "#CC5DE8",
    "#22B8CF", "#FF922B", "#94D82D", "#F06595", "#748FFC",
  ];

  // ── state ───────────────────────────────────────────────────────────────────
  var videoID = "";
  var playing = false;
  var speed = 1;
  var currentOffset = 0; // VOD seconds the clock has reached
  var lastTick = 0;
  var duration = 0; // seconds; exact when the API reports lengthSeconds, inferred otherwise
  var durationExact = false;
  var buffer = []; // loaded comments in offset order; [0, nextIdx) are already rendered
  var nextIdx = 0;
  var seen = {}; // comment IDs already buffered (guards against page overlap)
  var horizon = 0; // highest offset loaded so far
  var cursor = ""; // pagination cursor for the next page
  var hasNext = true;
  var fetching = false;
  var nextFetchAt = 0; // earliest time another background fetch may start
  var gen = 0; // bumped on load/seek/exit so stale fetch results are discarded
  var pinned = true; // auto-scroll pinned to bottom
  var dragging = false; // user is holding the seek slider

  // ── DOM ─────────────────────────────────────────────────────────────────────
  var $ = function (id) { return document.getElementById(id); };
  var setupEl = $("setup");
  var playerEl = $("player");
  var inputEl = $("vod-input");
  var loadEl = $("load");
  var setupErrorEl = $("setup-error");
  var setupBusyEl = $("setup-busy");
  var vodLabelEl = $("vod-label");
  var feedEl = $("feed");
  var feedNoteEl = $("feed-note");
  var resumeEl = $("resume-scroll");
  var playPauseEl = $("playpause");
  var clockEl = $("clock");
  var seekEl = $("seek");
  var speedEl = $("speed");

  // textContent only — never innerHTML with remote data.
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  // ── colors ──────────────────────────────────────────────────────────────────
  // Twitch user colors are picked for light backgrounds; on our dark panel the darkest
  // ones vanish. Blend toward white until perceived luminance clears a floor.
  function clampColor(hex) {
    if (!hex || !/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.test(hex)) return "#E6EAF2";
    var h = hex.slice(1);
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    var r = parseInt(h.slice(0, 2), 16);
    var g = parseInt(h.slice(2, 4), 16);
    var b = parseInt(h.slice(4, 6), 16);
    var lum = 0.2126 * r + 0.7152 * g + 0.0722 * b;
    var FLOOR = 112;
    if (lum >= FLOOR) return hex;
    var t = (FLOOR - lum) / (255 - lum || 1);
    function lift(c) {
      var n = Math.round(c + (255 - c) * t);
      return (n < 16 ? "0" : "") + n.toString(16);
    }
    return "#" + lift(r) + lift(g) + lift(b);
  }

  function fallbackColor(name) {
    var hash = 0;
    for (var i = 0; i < name.length; i++) hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0;
    return PALETTE[Math.abs(hash) % PALETTE.length];
  }

  // ── time formatting ─────────────────────────────────────────────────────────
  function pad2(n) { return (n < 10 ? "0" : "") + n; }
  function fmtTime(seconds) {
    var s = Math.max(0, Math.floor(seconds));
    var h = Math.floor(s / 3600);
    var m = Math.floor((s % 3600) / 60);
    var sec = s % 60;
    return h > 0 ? h + ":" + pad2(m) + ":" + pad2(sec) : m + ":" + pad2(sec);
  }

  // ── Twitch GQL (via the host's http bridge) ─────────────────────────────────
  // variables: {videoID, contentOffsetSeconds} for the first page of a position,
  // {videoID, cursor} for subsequent pages.
  function fetchCommentPage(variables) {
    return v.send({
      type: "http.fetch",
      payload: {
        url: GQL_URL,
        method: "POST",
        headers: { "Client-ID": CLIENT_ID, "Content-Type": "application/json" },
        body: JSON.stringify([{
          operationName: "VideoCommentsByOffsetOrCursor",
          variables: variables,
          extensions: { persistedQuery: { version: 1, sha256Hash: COMMENTS_HASH } },
        }]),
      },
    }).then(function (res) {
      if (!res || typeof res.body !== "string") throw new Error("Empty response from Twitch.");
      if (res.status < 200 || res.status >= 300) throw new Error("Twitch returned HTTP " + res.status + ".");
      var parsed;
      try { parsed = JSON.parse(res.body); } catch (e) { throw new Error("Unreadable response from Twitch."); }
      var first = Array.isArray(parsed) ? parsed[0] : parsed;
      var video = first && first.data ? first.data.video : null;
      if (!video) throw new Error("VOD not found — check the URL or ID.");
      var comments = video.comments;
      if (!comments || !Array.isArray(comments.edges)) {
        throw new Error("Chat replay is unavailable for this VOD.");
      }
      var out = [];
      var lastCursor = "";
      comments.edges.forEach(function (edge) {
        var n = edge && edge.node;
        if (edge && edge.cursor) lastCursor = edge.cursor;
        if (!n || typeof n.contentOffsetSeconds !== "number") return;
        var frags = n.message && Array.isArray(n.message.fragments) ? n.message.fragments : [];
        var text = "";
        for (var i = 0; i < frags.length; i++) {
          if (frags[i] && typeof frags[i].text === "string") text += frags[i].text;
        }
        out.push({
          id: n.id || "",
          offset: n.contentOffsetSeconds,
          author: n.commenter && n.commenter.displayName ? n.commenter.displayName : "anonymous",
          color: n.message && n.message.userColor ? n.message.userColor : "",
          text: text,
        });
      });
      out.sort(function (a, b) { return a.offset - b.offset; });
      return {
        comments: out,
        cursor: lastCursor,
        hasNext: !!(comments.pageInfo && comments.pageInfo.hasNextPage),
        lengthSeconds: typeof video.lengthSeconds === "number" ? video.lengthSeconds : 0,
      };
    });
  }

  // Fold a fetched page into the buffer and update pagination/duration state.
  function ingest(page) {
    for (var i = 0; i < page.comments.length; i++) {
      var c = page.comments[i];
      var key = c.id || (c.offset + ":" + c.author + ":" + c.text);
      if (Object.prototype.hasOwnProperty.call(seen, key)) continue;
      seen[key] = true;
      buffer.push(c);
      if (c.offset > horizon) horizon = c.offset;
    }
    cursor = page.cursor;
    hasNext = page.hasNext && !!page.cursor;
    if (page.lengthSeconds > 0) {
      duration = page.lengthSeconds;
      durationExact = true;
    }
    // Duration is inferred from the last loaded comment until the API reports it.
    if (!durationExact && horizon > duration) duration = horizon;
  }

  // ── feed rendering ──────────────────────────────────────────────────────────
  function lineEl(c) {
    var line = el("div", "msg");
    line.appendChild(el("span", "ts", fmtTime(c.offset)));
    var who = el("span", "author", c.author);
    who.style.color = clampColor(c.color || fallbackColor(c.author));
    line.appendChild(who);
    line.appendChild(el("span", "body", c.text));
    return line;
  }

  function trimFeed() {
    while (feedEl.childNodes.length > MAX_LINES) feedEl.removeChild(feedEl.firstChild);
  }

  function scrollToBottom() {
    feedEl.scrollTop = feedEl.scrollHeight;
  }

  function setPinned(next) {
    pinned = next;
    resumeEl.hidden = next;
  }

  function note(text) {
    feedNoteEl.textContent = text;
    feedNoteEl.hidden = false;
  }
  function hideNote() {
    feedNoteEl.hidden = true;
  }

  // Append every buffered comment the clock has reached.
  function flush() {
    var appended = false;
    while (nextIdx < buffer.length && buffer[nextIdx].offset <= currentOffset) {
      feedEl.appendChild(lineEl(buffer[nextIdx]));
      nextIdx++;
      appended = true;
    }
    if (appended) {
      hideNote();
      trimFeed();
      if (pinned) scrollToBottom();
    }
  }

  // ── background buffering ────────────────────────────────────────────────────
  function maybeFetch() {
    if (fetching || !hasNext || !videoID || !cursor) return;
    if (Date.now() < nextFetchAt) return;
    if (horizon >= currentOffset + BUFFER_AHEAD_S) return;
    var myGen = gen;
    fetching = true;
    fetchCommentPage({ videoID: videoID, cursor: cursor }).then(
      function (page) {
        if (myGen !== gen) return;
        fetching = false;
        ingest(page);
      },
      function () {
        if (myGen !== gen) return;
        fetching = false;
        nextFetchAt = Date.now() + FETCH_RETRY_MS; // transient failure; retry later
      }
    );
  }

  // ── playback clock ──────────────────────────────────────────────────────────
  function setPlaying(next) {
    playing = next;
    lastTick = Date.now();
    playPauseEl.textContent = next ? "Pause" : "Play";
  }

  function updateTransport(previewOffset) {
    var shown = previewOffset !== undefined ? previewOffset : currentOffset;
    var total = durationExact
      ? fmtTime(duration)
      : (duration > 0 ? "~" + fmtTime(duration) : "--:--");
    clockEl.textContent = fmtTime(shown) + " / " + total;
    seekEl.max = String(Math.max(1, Math.ceil(duration)));
    if (!dragging) seekEl.value = String(Math.floor(currentOffset));
  }

  function tick() {
    if (playerEl.hidden) return;
    var now = Date.now();
    if (playing) {
      currentOffset += ((now - lastTick) / 1000) * speed;
      // While the duration is inferred and more pages exist, let the clock extend it.
      if (!durationExact && hasNext && currentOffset > duration) duration = currentOffset;
      if (currentOffset > duration && (durationExact || !hasNext)) {
        currentOffset = duration;
        if (nextIdx >= buffer.length && !fetching) {
          setPlaying(false);
          note("End of chat replay.");
        }
      }
    }
    lastTick = now;
    flush();
    maybeFetch();
    updateTransport();
  }

  // ── load / seek ─────────────────────────────────────────────────────────────
  function resetBuffers(baseOffset) {
    gen++;
    buffer = [];
    nextIdx = 0;
    seen = {};
    cursor = "";
    hasNext = true;
    fetching = false;
    nextFetchAt = 0;
    horizon = baseOffset;
    currentOffset = baseOffset;
    lastTick = Date.now();
    feedEl.textContent = "";
    setPinned(true);
    return gen;
  }

  function parseVOD(raw) {
    var s = (raw || "").trim();
    if (/^\d+$/.test(s)) return s;
    var m = s.match(/twitch\.tv\/videos\/(\d+)/i);
    return m ? m[1] : "";
  }

  function setupError(text) {
    setupErrorEl.textContent = text;
    setupErrorEl.hidden = false;
  }

  function setSetupBusy(busy) {
    setupBusyEl.hidden = !busy;
    loadEl.disabled = busy;
    inputEl.disabled = busy;
  }

  function loadVOD() {
    var id = parseVOD(inputEl.value);
    setupErrorEl.hidden = true;
    if (!id) {
      setupError("Enter a Twitch VOD URL (twitch.tv/videos/…) or a numeric VOD ID.");
      return;
    }
    setSetupBusy(true);
    var myGen = resetBuffers(0);
    videoID = "";
    duration = 0;
    durationExact = false;
    fetchCommentPage({ videoID: id, contentOffsetSeconds: 0 }).then(
      function (page) {
        if (myGen !== gen) return;
        setSetupBusy(false);
        videoID = id;
        ingest(page);
        vodLabelEl.textContent = "VOD " + id;
        setupEl.hidden = true;
        playerEl.hidden = false;
        hideNote();
        if (!buffer.length && !hasNext) {
          note("This VOD has no chat messages.");
        }
        setPlaying(true);
        updateTransport();
      },
      function (e) {
        if (myGen !== gen) return;
        setSetupBusy(false);
        setupError(e && e.message ? e.message : "Couldn't load the VOD chat.");
      }
    );
  }

  function seekTo(target) {
    target = Math.max(0, Math.floor(target));
    var myGen = resetBuffers(target);
    note("Loading chat at " + fmtTime(target) + "…");
    updateTransport();
    fetchCommentPage({ videoID: videoID, contentOffsetSeconds: target }).then(
      function (page) {
        if (myGen !== gen) return;
        ingest(page);
        // The page usually includes the comments just before the offset; render the
        // last few immediately so the feed isn't blank after a seek.
        var pre = [];
        while (nextIdx < buffer.length && buffer[nextIdx].offset <= currentOffset) {
          pre.push(buffer[nextIdx]);
          nextIdx++;
        }
        var backfill = pre.slice(-SEEK_BACKFILL);
        for (var i = 0; i < backfill.length; i++) feedEl.appendChild(lineEl(backfill[i]));
        trimFeed();
        scrollToBottom();
        if (backfill.length) hideNote();
        else note("No chat yet at this point — playing on.");
      },
      function (e) {
        if (myGen !== gen) return;
        note(e && e.message ? e.message : "Couldn't load chat at that position.");
      }
    );
  }

  function exitToSetup() {
    gen++;
    setPlaying(false);
    videoID = "";
    buffer = [];
    nextIdx = 0;
    seen = {};
    feedEl.textContent = "";
    hideNote();
    playerEl.hidden = true;
    setupEl.hidden = false;
    setupErrorEl.hidden = true;
    inputEl.focus();
  }

  // ── controls ────────────────────────────────────────────────────────────────
  loadEl.addEventListener("click", loadVOD);
  inputEl.addEventListener("keydown", function (e) {
    if (e.key === "Enter") loadVOD();
  });

  playPauseEl.addEventListener("click", function () {
    if (!playing && currentOffset >= duration && (durationExact || !hasNext) && duration > 0) {
      seekTo(0); // replay from the start when toggled at the end
    }
    setPlaying(!playing);
    hideNote();
  });

  speedEl.addEventListener("change", function () {
    speed = parseFloat(this.value) || 1;
  });

  seekEl.addEventListener("input", function () {
    dragging = true;
    updateTransport(parseInt(this.value, 10) || 0);
  });
  seekEl.addEventListener("change", function () {
    dragging = false;
    seekTo(parseInt(this.value, 10) || 0);
  });

  $("change").addEventListener("click", exitToSetup);

  // Auto-scroll pinning: pause when the user scrolls up; resume from the pill.
  feedEl.addEventListener("scroll", function () {
    var nearBottom = feedEl.scrollTop + feedEl.clientHeight >= feedEl.scrollHeight - 24;
    setPinned(nearBottom);
  });
  resumeEl.addEventListener("click", function () {
    setPinned(true);
    scrollToBottom();
  });

  // ── boot ────────────────────────────────────────────────────────────────────
  if (!v) {
    setupError("Virta bridge unavailable — reload the panel.");
    loadEl.disabled = true;
    return;
  }
  if (!v.hasScope("http")) {
    setupError("Missing 'http' scope — reinstall the plugin to grant it.");
    loadEl.disabled = true;
    return;
  }

  setInterval(tick, TICK_MS);
})();

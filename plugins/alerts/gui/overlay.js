/*
 * Alerts — standalone OBS overlay.
 *
 * This page runs as an OBS browser source with no host SPA around it, so it does NOT use the
 * __virta.js bridge. It authenticates with the read-scoped ?token= minted by the panel's
 * "Copy overlay URL" button and talks to the daemon directly, same-origin only:
 *
 *   - GET /v1/plugins/com.virta.alerts/config  (Authorization: Bearer <token>) for the rules,
 *     re-fetched every 60s and on every WS reconnect so panel edits propagate without
 *     restarting OBS.
 *   - WebSocket /v1/stream?token=<token> for live events. Raw WireEvents are parsed here:
 *     type "message" carries a UnifiedMessage; alert-type messages (sub/resub/giftsub/raid/
 *     host/follow) are mapped to alert data. Magnitudes (gift count, raid viewers, resub
 *     months, tier) ride in the platform's system-message text, so they are parsed from the
 *     joined segment text via the shared parser in animations.js.
 *
 * Reconnects use jittered exponential backoff; replay (`since`) is only requested after a drop
 * so a fresh OBS start never replays a backlog of old hype.
 */
(function () {
  "use strict";
  var A = window.VirtaAlertAnimations;
  var PLUGIN_ID = "com.virta.alerts";
  var CONFIG_REFRESH_MS = 60000;
  var BACKOFF_MS = [1000, 2000, 4000, 8000, 15000];

  var stage = document.getElementById("stage");
  var hintEl = document.getElementById("hint");

  function hint(text) {
    if (text) {
      hintEl.textContent = text;
      hintEl.hidden = false;
    } else {
      hintEl.hidden = true;
    }
  }

  var params = new URLSearchParams(window.location.search);
  var token = params.get("token") || "";
  // Which overlay instance this browser source is. Old copied URLs have no ?overlay= — they
  // behave as an unfiltered overlay at the legacy rules.position.
  var overlayId = params.get("overlay") || "";
  if (!token) {
    hint("Virta Alerts: missing ?token= — copy the overlay URL from the Alerts panel in Virta and paste it into this browser source.");
    return;
  }

  var rules = A.defaultRules();
  var instance = { id: "", name: "Legacy", types: [], position: rules.position };
  var instanceMissing = false;
  // Overlay player: sound + TTS both enabled; gap synced from config.
  var player = A.createPlayer(stage, { gapMs: rules.gapMs, sound: true, tts: true });

  function setPosition(pos) {
    stage.className = "stage pos-" + (pos === "top" || pos === "center" ? pos : "bottom");
  }

  // ── config ──────────────────────────────────────────────────────────────────
  function fetchConfig() {
    return fetch("/v1/plugins/" + encodeURIComponent(PLUGIN_ID) + "/config", {
      headers: { Authorization: "Bearer " + token },
    })
      .then(function (res) {
        if (res.status === 401 || res.status === 403) {
          hint("Virta Alerts: the overlay token was rejected — copy a fresh overlay URL from the Alerts panel.");
          throw new Error("unauthorized");
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        // The daemon wraps saved values as {config:{...}}; tolerate an unwrapped object too.
        var values = (data && data.config) || data || {};
        rules = A.normalizeRules(values.rules);
        player.setGap(rules.gapMs);
        if (overlayId) {
          var overlays = A.normalizeOverlays(values.overlays, rules);
          var found = null;
          overlays.forEach(function (o) { if (o.id === overlayId) found = o; });
          instanceMissing = !found;
          if (found) instance = found;
        } else {
          instance = { id: "", name: "Legacy", types: [], position: rules.position };
        }
        setPosition(instance.position);
        hint(instanceMissing
          ? "Virta Alerts: this overlay was removed in the Alerts panel — copy a fresh URL."
          : "");

        // Apply per-style CSS overrides saved via the panel code editor.
        A.clearAllOverrides();
        var styleOvr = (values.styleOverrides && typeof values.styleOverrides === "object") ? values.styleOverrides : {};
        for (var sid in styleOvr) {
          if (Object.prototype.hasOwnProperty.call(styleOvr, sid) && typeof styleOvr[sid] === "string" && styleOvr[sid]) {
            try { A.setStyleOverride(sid, styleOvr[sid]); } catch (e) { /* invalid CSS is still set; browser ignores bad rules */ }
          }
        }
        // Apply per-animation JS overrides.
        var animOvr = (values.animOverrides && typeof values.animOverrides === "object") ? values.animOverrides : {};
        for (var aid in animOvr) {
          if (Object.prototype.hasOwnProperty.call(animOvr, aid) && typeof animOvr[aid] === "string" && animOvr[aid]) {
            try { A.setCustomPreset(aid, animOvr[aid]); } catch (e) { console.warn("Virta Alerts anim override [" + aid + "]:", e); }
          }
        }
        // Refresh custom event definitions so the hook poller uses the latest rules.
        customEvents = Array.isArray(values.customEvents) ? values.customEvents : [];
      })
      .catch(function () { /* keep the last good rules; retry on the next tick */ });
  }

  fetchConfig();
  setInterval(fetchConfig, CONFIG_REFRESH_MS);

  // ── live events ─────────────────────────────────────────────────────────────
  function handleMessage(m) {
    if (!m || !rules.enabled || instanceMissing) return;
    var info = A.typeInfo(m.type);
    if (!info) return; // not an alert type (chat, system, …)
    if (instance.types.length && instance.types.indexOf(m.type) === -1) return; // filtered to other overlays
    var rule = rules.types[m.type];
    if (!rule || !rule.enabled) return;
    if (m.annotations && m.annotations.hidden) return; // a filter hid it; never celebrate it

    var body = "";
    (m.segments || []).forEach(function (s) { body += (s && s.text) || ""; });
    var mags = A.parseMagnitudes(m.type, body);
    var author = (m.author && (m.author.display_name || m.author.login)) || "Someone";
    player.enqueue({
      type: m.type,
      author: author,
      count: mags.count,
      viewers: mags.viewers,
      months: mags.months,
      tier: mags.tier,
    }, rule);
  }

  // ── stream connection ───────────────────────────────────────────────────────
  var lastSeq = 0;
  var everConnected = false;
  var attempt = 0;

  function connect() {
    var proto = window.location.protocol === "https:" ? "wss" : "ws";
    var ws;
    try {
      ws = new WebSocket(proto + "://" + window.location.host + "/v1/stream?token=" + encodeURIComponent(token));
    } catch (e) {
      scheduleReconnect();
      return;
    }

    ws.onopen = function () {
      attempt = 0;
      // First connect: live-only (since 0 = no replay) so a fresh OBS start doesn't blast a
      // backlog. After a drop: ask for replay past lastSeq so a blip doesn't eat alerts; the
      // seq dedupe below swallows the at-least-once overlap.
      ws.send(JSON.stringify({
        action: "subscribe",
        channels: [],
        since: everConnected ? lastSeq : 0,
      }));
      everConnected = true;
      fetchConfig(); // panel edits made while disconnected propagate immediately
    };

    ws.onmessage = function (ev) {
      var event;
      try {
        event = JSON.parse(ev.data);
      } catch (e) {
        return;
      }
      if (typeof event.seq === "number") {
        if (event.seq <= lastSeq) return; // replay overlap — already handled
        lastSeq = event.seq;
      }
      if (event.type === "message" && event.message) handleMessage(event.message);
    };

    ws.onerror = function () {
      try { ws.close(); } catch (e) { /* already closing */ }
    };

    ws.onclose = function () {
      scheduleReconnect();
    };
  }

  function scheduleReconnect() {
    var base = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
    attempt += 1;
    var jitter = Math.random() * 400;
    setTimeout(connect, base + jitter);
  }

  connect();

  // ── Custom hook events (external integrations) ──────────────────────────────
  // Poll every 2s for events fired via POST /v1/plugins/{id}/hook/{name}.
  var hookLastMs = 0;
  var customEvents = []; // refreshed from config on every fetchConfig()

  function playHookEvent(event) {
    var evtName = event.name; // lowercased by the daemon
    var evtRule = null;
    for (var i = 0; i < customEvents.length; i++) {
      var ce = customEvents[i];
      if (ce.name && ce.name.toLowerCase().replace(/[^a-z0-9]+/g, "-") === evtName) {
        evtRule = ce;
        break;
      }
    }
    if (!evtRule || evtRule.enabled === false) return;
    var payload = {};
    try { payload = JSON.parse(JSON.stringify(event.payload || {})); } catch (e) { /* ignore */ }
    player.enqueue({
      type: "custom",
      author: (typeof payload.author === "string" ? payload.author : "") || "",
      badge: evtRule.badge || evtName.toUpperCase(),
      _ext: payload,
    }, {
      animation: evtRule.animation || "pop",
      style: evtRule.style || "gradient-glass",
      template: evtRule.template || "{author}",
      durationMs: 6000,
    });
  }

  function pollHookEvents() {
    fetch("/v1/plugins/" + encodeURIComponent(PLUGIN_ID) + "/hook/events?since=" + hookLastMs, {
      headers: { Authorization: "Bearer " + token },
    })
      .then(function (res) {
        if (res.status === 204) return null;
        if (!res.ok) return null;
        return res.json();
      })
      .then(function (body) {
        if (!body || !Array.isArray(body.events)) return;
        body.events.forEach(function (e) {
          if (typeof e.ts === "number" && e.ts > hookLastMs) hookLastMs = e.ts;
          playHookEvent(e);
        });
      })
      .catch(function () { /* silent — keep polling */ });
  }

  setInterval(pollHookEvents, 2000);

  // ── Test relay (daemon polling) ─────────────────────────────────────────────
  // The panel POSTs a signal via the bridge → daemon stores it in memory.
  // We poll every 2s with ?since=<lastSeenMs> so only genuinely new signals fire.
  // This works cross-process (Electron panel → OBS browser source) unlike localStorage.
  var testLastMs = 0;

  function pollTestSignal() {
    fetch("/v1/plugins/" + encodeURIComponent(PLUGIN_ID) + "/signal?since=" + testLastMs, {
      headers: { Authorization: "Bearer " + token },
    })
      .then(function (res) {
        if (res.status === 204) return null; // nothing new
        if (!res.ok) return null;
        return res.json();
      })
      .then(function (body) {
        if (!body) return;
        // body: { ts: <unixMs>, payload: { data, rule, ts } }
        if (typeof body.ts === "number") testLastMs = body.ts;
        var obj = body.payload;
        if (!obj || !obj.data) return;
        if (!rules.enabled || instanceMissing) return;
        var info = A.typeInfo(obj.data.type);
        if (!info) return;
        if (instance.types.length && instance.types.indexOf(obj.data.type) === -1) return;
        var rule = obj.rule || rules.types[obj.data.type];
        if (!rule || !rule.enabled) return;
        player.flash(obj.data, rule);
      })
      .catch(function () { /* network errors are silent — keep polling */ });
  }

  setInterval(pollTestSignal, 2000);
})();

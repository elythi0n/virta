/*
 * Polls — Virta plugin GUI.
 *
 * Sandboxed: this page makes no network calls itself (CSP blocks them). Everything goes through
 * the window.__virta postMessage bridge: live chat via the events stream, persistence via the
 * plugin's saved config. Votes are tallied client-side, one per person per platform.
 */
(function () {
  "use strict";
  var v = window.__virta;

  var MAX_OPTIONS = 6;
  var MIN_OPTIONS = 2;
  var HISTORY_LIMIT = 20;
  var RENDER_COALESCE_MS = 300;

  // Trimmed body must be exactly "1".."6", "vote N", or "!vote N" (case-insensitive).
  var VOTE_RE = /^(?:!?vote\s+)?([1-6])$/i;

  var state = {
    mode: "compose", // "compose" | "live" | "results"
    poll: null, // { q, options: [{label, votes}], duration, endsAt }
    voters: {}, // "authorId||author:platform" -> option index; revotes move, never double-count
    history: []
  };

  var composeOptions = ["", ""]; // option labels being edited
  var liveRows = []; // persistent row refs so bar widths transition instead of rebuilding
  var renderQueued = false;
  var timerHandle = null;

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
  function $(id) {
    return document.getElementById(id);
  }

  function setConn(ok, label) {
    $("conn-dot").className = "dot " + (ok === null ? "dot-wait" : ok ? "dot-ok" : "dot-err");
    $("conn-label").textContent = label;
  }

  function pct(votes, total) {
    return total > 0 ? Math.round((votes / total) * 100) : 0;
  }

  function fmtClock(ms) {
    var s = Math.max(0, Math.ceil(ms / 1000));
    var m = Math.floor(s / 60);
    return m + ":" + String(s % 60).padStart(2, "0");
  }

  // ── view switching ──────────────────────────────────────────────────────────
  function show(mode) {
    state.mode = mode;
    $("view-compose").hidden = mode !== "compose";
    $("view-live").hidden = mode !== "live";
    $("view-results").hidden = mode !== "results";
  }

  // ── Compose ─────────────────────────────────────────────────────────────────
  function renderCompose() {
    fill(
      $("options"),
      composeOptions.map(function (value, i) {
        var row = el("div", "opt-edit");
        row.appendChild(el("span", "opt-num", String(i + 1)));
        var input = el("input", "input opt-input");
        input.type = "text";
        input.maxLength = 80;
        input.placeholder = "Option " + (i + 1);
        input.value = value;
        input.addEventListener("input", function () {
          composeOptions[i] = input.value;
          updateStartEnabled();
        });
        row.appendChild(input);
        var rm = el("button", "btn btn-ghost btn-rm", "×");
        rm.type = "button";
        rm.title = "Remove option";
        rm.disabled = composeOptions.length <= MIN_OPTIONS;
        rm.addEventListener("click", function () {
          composeOptions.splice(i, 1);
          renderCompose();
        });
        row.appendChild(rm);
        return row;
      })
    );
    $("add-option").disabled = composeOptions.length >= MAX_OPTIONS;
    updateStartEnabled();
  }

  function filledOptions() {
    return composeOptions
      .map(function (s) { return s.trim(); })
      .filter(function (s) { return s !== ""; });
  }

  function updateStartEnabled() {
    var ok = $("question").value.trim() !== "" && filledOptions().length >= MIN_OPTIONS;
    $("start").disabled = !ok;
  }

  // ── Live poll ───────────────────────────────────────────────────────────────
  function startPoll() {
    var labels = filledOptions();
    var duration = parseInt($("duration").value, 10) || 0;
    state.poll = {
      q: $("question").value.trim(),
      options: labels.map(function (label) { return { label: label, votes: 0 }; }),
      duration: duration,
      endsAt: duration > 0 ? Date.now() + duration * 1000 : 0
    };
    state.voters = {};

    $("live-question").textContent = state.poll.q;
    liveRows = buildRows($("live-rows"), state.poll.options);
    $("countdown").hidden = duration === 0;
    show("live");
    renderLive();

    if (timerHandle) clearInterval(timerHandle);
    timerHandle = setInterval(tick, 500);
    tick();
  }

  // Builds one persistent row per option; callers update counts/widths in place so the
  // bar width transition actually animates between renders.
  function buildRows(container, options) {
    var refs = [];
    fill(
      container,
      options.map(function (opt, i) {
        var row = el("div", "opt-row");
        var line = el("div", "opt-line");
        line.appendChild(el("span", "opt-num", String(i + 1)));
        line.appendChild(el("span", "opt-label", opt.label));
        var count = el("span", "opt-count", "0");
        var pctEl = el("span", "opt-pct", "0%");
        line.appendChild(count);
        line.appendChild(pctEl);
        row.appendChild(line);
        var bar = el("div", "bar");
        var inner = el("span");
        inner.style.width = "0%";
        bar.appendChild(inner);
        row.appendChild(bar);
        refs.push({ row: row, count: count, pct: pctEl, inner: inner });
        return row;
      })
    );
    return refs;
  }

  function updateRows(refs, options) {
    var total = options.reduce(function (sum, o) { return sum + o.votes; }, 0);
    var max = options.reduce(function (m, o) { return Math.max(m, o.votes); }, 0);
    options.forEach(function (opt, i) {
      var p = pct(opt.votes, total);
      refs[i].count.textContent = String(opt.votes);
      refs[i].pct.textContent = p + "%";
      refs[i].inner.style.width = p + "%";
      refs[i].row.classList.toggle("lead", total > 0 && opt.votes === max);
    });
    return total;
  }

  function renderLive() {
    if (!state.poll) return;
    var total = updateRows(liveRows, state.poll.options);
    var unique = Object.keys(state.voters).length;
    $("live-totals").textContent =
      total + (total === 1 ? " vote" : " votes") + " · " +
      unique + (unique === 1 ? " voter" : " voters");
  }

  function scheduleRender() {
    if (renderQueued) return;
    renderQueued = true;
    setTimeout(function () {
      renderQueued = false;
      if (state.mode === "live") renderLive();
    }, RENDER_COALESCE_MS);
  }

  function tick() {
    if (state.mode !== "live" || !state.poll || state.poll.endsAt === 0) return;
    var left = state.poll.endsAt - Date.now();
    $("countdown").textContent = fmtClock(left);
    if (left <= 0) endPoll();
  }

  // ── Vote counting ───────────────────────────────────────────────────────────
  function onMessage(msg) {
    // Only count while a poll is live; anything delivered before start is ignored.
    if (state.mode !== "live" || !state.poll || !msg) return;
    if (msg.type && msg.type !== "chat") return;
    if (msg.deleted) return;
    var m = VOTE_RE.exec(String(msg.body || "").trim());
    if (!m) return;
    var n = parseInt(m[1], 10);
    if (n > state.poll.options.length) return;
    var key = (msg.authorId || msg.author || "anon") + ":" + (msg.platform || "?");
    var prev = state.voters[key];
    if (prev === n - 1) return;
    if (prev !== undefined) state.poll.options[prev].votes -= 1; // revote moves, never adds
    state.poll.options[n - 1].votes += 1;
    state.voters[key] = n - 1;
    scheduleRender();
  }

  // ── Results + history ───────────────────────────────────────────────────────
  function endPoll() {
    if (!state.poll || state.mode !== "live") return;
    if (timerHandle) { clearInterval(timerHandle); timerHandle = null; }
    renderLive(); // flush any coalesced votes before freezing the tally

    var poll = state.poll;
    var total = poll.options.reduce(function (sum, o) { return sum + o.votes; }, 0);
    var winnerIdx = 0;
    poll.options.forEach(function (o, i) {
      if (o.votes > poll.options[winnerIdx].votes) winnerIdx = i;
    });
    var winner = total > 0 ? poll.options[winnerIdx].label : "No votes";

    $("results-question").textContent = poll.q;
    $("winner-label").textContent = winner;
    $("winner-pct").textContent = total > 0 ? pct(poll.options[winnerIdx].votes, total) + "%" : "";
    $("winner-banner").classList.toggle("winner-empty", total === 0);
    updateRows(buildRows($("result-rows"), poll.options), poll.options);
    var unique = Object.keys(state.voters).length;
    $("results-totals").textContent =
      total + (total === 1 ? " vote" : " votes") + " · " +
      unique + (unique === 1 ? " voter" : " voters");
    show("results");

    state.history.unshift({
      q: poll.q,
      options: poll.options.map(function (o) { return { label: o.label, votes: o.votes }; }),
      winner: winner,
      total: total,
      endedAt: new Date(Date.now()).toISOString()
    });
    state.history = state.history.slice(0, HISTORY_LIMIT);
    renderHistory();
    v.send({ type: "config.set", payload: { values: { history: state.history } } })
      .catch(function () { /* persistence is best-effort; the panel keeps working */ });
  }

  function newPoll() {
    state.poll = null;
    state.voters = {};
    composeOptions = ["", ""];
    $("question").value = "";
    renderCompose();
    show("compose");
  }

  function renderHistory() {
    $("history-count").textContent = state.history.length > 0 ? String(state.history.length) : "";
    fill(
      $("history-list"),
      state.history.length === 0
        ? [el("li", "muted", "No finished polls yet")]
        : state.history.map(function (h) {
            var li = el("li");
            li.appendChild(el("div", "h-q", h.q));
            var meta = el("div", "h-meta");
            meta.appendChild(el("span", "h-winner", h.winner));
            meta.appendChild(el("span", "h-votes", h.total + (h.total === 1 ? " vote" : " votes")));
            var when = new Date(h.endedAt);
            meta.appendChild(el("span", "h-date", isNaN(when.getTime()) ? "" : when.toLocaleDateString()));
            li.appendChild(meta);
            return li;
          })
    );
  }

  function showError(text) {
    var bar = $("err");
    bar.textContent = text;
    bar.hidden = false;
  }

  // ── boot ────────────────────────────────────────────────────────────────────
  $("question").addEventListener("input", updateStartEnabled);
  $("add-option").addEventListener("click", function () {
    if (composeOptions.length >= MAX_OPTIONS) return;
    composeOptions.push("");
    renderCompose();
  });
  $("start").addEventListener("click", startPoll);
  $("end").addEventListener("click", endPoll);
  $("new-poll").addEventListener("click", newPoll);
  $("history-toggle").addEventListener("click", function () {
    var list = $("history-list");
    list.hidden = !list.hidden;
    $("history-toggle").setAttribute("aria-expanded", list.hidden ? "false" : "true");
  });

  renderCompose();
  renderHistory();

  v.send({ type: "config.get" })
    .then(function (saved) {
      if (saved && Array.isArray(saved.history)) state.history = saved.history.slice(0, HISTORY_LIMIT);
    })
    .catch(function () { /* no saved history yet */ })
    .then(renderHistory);

  v.on("events.message", onMessage);
  v.send({ type: "events.subscribe", payload: {} })
    .then(function () {
      setConn(true, "Watching chat");
    })
    .catch(function () {
      setConn(false, "No chat feed");
      showError("Couldn't subscribe to the chat stream — polls will run, but votes won't arrive.");
    });
})();

/*
 * Alerts — configurator panel.
 *
 * Sandboxed: this page makes no network calls itself (CSP blocks them). Everything privileged
 * goes through the window.__virta postMessage bridge: rules persistence via config.get/set,
 * live events via events.subscribe → events.message, and the tokenized OBS URL via overlay.url.
 *
 * The preview strip renders through the exact same path as the OBS overlay — the shared
 * renderAlert/createPlayer in animations.js plus alert-styles.css — so what plays here is what
 * goes on stream. Real incoming events also play in the preview, making the panel a confidence
 * monitor while live.
 */
(function () {
  "use strict";
  var v = window.__virta;
  var A = window.VirtaAlertAnimations;

  var SAVE_DEBOUNCE_MS = 500;
  var AUTHORS = ["nova_tide", "PixelPanda", "ghostriderTTV", "MossyOak", "Kaelthura", "retro_rita", "BinaryBard", "LunaSpark"];

  // fullCfg is the whole stored config object (config.set is a full replace, so fields we don't
  // own — like the host-managed overlay_token — must be carried along untouched).
  var fullCfg = {};
  var rules = A.defaultRules();
  var overlays = A.defaultOverlays(rules);
  var player = null;
  var saveTimer = null;
  var statusTimer = null;

  // ── tiny DOM helpers (textContent only — never innerHTML with stream data) ──
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }
  function $(id) {
    return document.getElementById(id);
  }
  function pick(list) {
    return list[(Math.random() * list.length) | 0];
  }

  function status(text, isError) {
    var s = $("status");
    s.textContent = text;
    s.className = "status" + (isError ? " status-err" : "");
    if (statusTimer) clearTimeout(statusTimer);
    if (text && !isError) {
      statusTimer = setTimeout(function () { s.textContent = ""; }, 2000);
    }
  }

  function notice(text) {
    var n = $("notice");
    n.textContent = text || "";
    n.hidden = !text;
  }

  // ── persistence ─────────────────────────────────────────────────────────────
  function saveNow() {
    fullCfg.rules = rules;
    fullCfg.overlays = overlays;
    return v.send({ type: "config.set", payload: { values: fullCfg } })
      .then(function () { status("Saved"); })
      .catch(function (e) { status("Save failed" + (e && e.message ? ": " + e.message : ""), true); });
  }

  function save() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(saveNow, SAVE_DEBOUNCE_MS);
  }

  // ── preview playback ────────────────────────────────────────────────────────
  function sampleData(type, forcedMagnitude) {
    var d = { type: type, author: pick(AUTHORS), tier: pick(["1", "1", "2", "3", "Prime"]) };
    // Randomized magnitudes deliberately straddle the default tier thresholds so repeated tests
    // exercise base rule and tiers alike.
    if (type === "giftsub") d.count = forcedMagnitude != null ? forcedMagnitude : pick([1, 5, 10, 25, 50, 100]);
    if (type === "raid" || type === "host") d.viewers = forcedMagnitude != null ? forcedMagnitude : pick([12, 48, 86, 210, 560]);
    if (type === "resub") d.months = forcedMagnitude != null ? forcedMagnitude : pick([2, 3, 7, 12, 24, 36]);
    return d;
  }

  function playTest(type, forcedMagnitude) {
    player.enqueue(sampleData(type, forcedMagnitude), rules.types[type]);
  }

  function setPosition(pos) {
    $("stage").className = "stage pos-" + (pos === "top" || pos === "center" ? pos : "bottom");
  }

  // ── select builders ─────────────────────────────────────────────────────────
  function optionList(select, items, value, inheritLabel) {
    select.textContent = "";
    if (inheritLabel) {
      var inh = el("option", null, inheritLabel);
      inh.value = "";
      select.appendChild(inh);
    }
    items.forEach(function (item) {
      var o = el("option", null, item.name);
      o.value = item.id;
      select.appendChild(o);
    });
    select.value = value || "";
    return select;
  }

  function animationSelect(value, inheritLabel) {
    return optionList(el("select", "select"), A.presets, value, inheritLabel);
  }
  function styleSelect(value, inheritLabel) {
    return optionList(el("select", "select"), A.styles, value, inheritLabel);
  }

  function field(labelText, control) {
    var wrap = el("label", "field");
    wrap.appendChild(el("span", "field-label", labelText));
    wrap.appendChild(control);
    return wrap;
  }

  // ── tier rows ───────────────────────────────────────────────────────────────
  var MAGNITUDE_LABEL = { count: "Min gifts", viewers: "Min viewers", months: "Min months" };

  function tierRow(typeId, tier, index) {
    var rule = rules.types[typeId];
    var row = el("div", "tier-row");

    var min = el("input", "input input-num");
    min.type = "number";
    min.min = "0";
    min.step = "1";
    min.value = String(tier.min);
    min.addEventListener("change", function () {
      var n = parseInt(min.value, 10);
      tier.min = isFinite(n) && n >= 0 ? n : 0;
      min.value = String(tier.min);
      save();
    });
    row.appendChild(field(MAGNITUDE_LABEL[A.typeInfo(typeId).magnitude] || "Min", min));

    var anim = animationSelect(tier.animation || "", "Inherit");
    anim.addEventListener("change", function () {
      if (anim.value) tier.animation = anim.value;
      else delete tier.animation;
      save();
    });
    row.appendChild(field("Animation", anim));

    var style = styleSelect(tier.style || "", "Inherit");
    style.addEventListener("change", function () {
      if (style.value) tier.style = style.value;
      else delete tier.style;
      save();
    });
    row.appendChild(field("Style", style));

    var dur = el("input", "input input-num");
    dur.type = "number";
    dur.min = "1";
    dur.max = "60";
    dur.step = "0.5";
    dur.placeholder = "inherit";
    if (tier.durationMs) dur.value = String(tier.durationMs / 1000);
    dur.addEventListener("change", function () {
      var n = parseFloat(dur.value);
      if (isFinite(n) && n > 0) tier.durationMs = Math.min(60000, Math.max(1000, Math.round(n * 1000)));
      else { delete tier.durationMs; dur.value = ""; }
      save();
    });
    row.appendChild(field("Seconds", dur));

    var tpl = el("input", "input input-tpl");
    tpl.type = "text";
    tpl.maxLength = 200;
    tpl.placeholder = "inherit";
    tpl.value = tier.template || "";
    tpl.addEventListener("input", function () {
      if (tpl.value) tier.template = tpl.value;
      else delete tier.template;
      save();
    });
    row.appendChild(field("Template", tpl));

    var actions = el("div", "tier-actions");
    var test = el("button", "btn btn-small", "Test");
    test.type = "button";
    test.title = "Play a sample alert at this tier's threshold";
    test.addEventListener("click", function () { playTest(typeId, tier.min); });
    actions.appendChild(test);

    var rm = el("button", "btn btn-small btn-rm", "×");
    rm.type = "button";
    rm.title = "Remove tier";
    rm.addEventListener("click", function () {
      rule.tiers.splice(index, 1);
      save();
      renderCards();
    });
    actions.appendChild(rm);
    row.appendChild(actions);

    return row;
  }

  function tiersBlock(typeId) {
    var info = A.typeInfo(typeId);
    var rule = rules.types[typeId];
    var block = el("div", "tiers");
    block.appendChild(el("div", "tiers-label", "Threshold tiers"));
    block.appendChild(el("div", "tiers-hint",
      "Highest matching “" + (MAGNITUDE_LABEL[info.magnitude] || "min").toLowerCase() + "” wins and overrides the base rule."));

    rule.tiers.forEach(function (tier, i) {
      block.appendChild(tierRow(typeId, tier, i));
    });

    var add = el("button", "btn btn-small btn-add", "+ Add tier");
    add.type = "button";
    add.addEventListener("click", function () {
      var last = rule.tiers.length ? rule.tiers[rule.tiers.length - 1].min : 0;
      rule.tiers.push({ min: last > 0 ? last * 2 : 10 });
      save();
      renderCards();
    });
    block.appendChild(add);
    return block;
  }

  // ── per-type cards ──────────────────────────────────────────────────────────
  function typeCard(info) {
    var rule = rules.types[info.id];
    var card = el("section", "card");

    var head = el("div", "card-head");
    var toggle = el("label", "card-toggle");
    var enabled = el("input");
    enabled.type = "checkbox";
    enabled.checked = !!rule.enabled;
    enabled.addEventListener("change", function () {
      rule.enabled = enabled.checked;
      card.classList.toggle("card-off", !rule.enabled);
      save();
    });
    toggle.appendChild(enabled);
    toggle.appendChild(el("span", "card-title", info.label));
    head.appendChild(toggle);

    var test = el("button", "btn btn-small", "Test");
    test.type = "button";
    test.title = "Play a sample " + info.label.toLowerCase() + " alert (randomized, so tiers get exercised)";
    test.addEventListener("click", function () { playTest(info.id); });
    head.appendChild(test);
    card.appendChild(head);
    card.classList.toggle("card-off", !rule.enabled);

    var grid = el("div", "card-grid");

    var anim = animationSelect(rule.animation);
    anim.addEventListener("change", function () { rule.animation = anim.value; save(); });
    grid.appendChild(field("Animation", anim));

    var style = styleSelect(rule.style);
    style.addEventListener("change", function () { rule.style = style.value; save(); });
    grid.appendChild(field("Style", style));

    var dur = el("input", "input input-num");
    dur.type = "number";
    dur.min = "1";
    dur.max = "60";
    dur.step = "0.5";
    dur.value = String(rule.durationMs / 1000);
    dur.addEventListener("change", function () {
      var n = parseFloat(dur.value);
      rule.durationMs = isFinite(n) && n > 0 ? Math.min(60000, Math.max(1000, Math.round(n * 1000))) : 6000;
      dur.value = String(rule.durationMs / 1000);
      save();
    });
    grid.appendChild(field("Seconds", dur));
    card.appendChild(grid);

    var tpl = el("input", "input input-tpl");
    tpl.type = "text";
    tpl.maxLength = 200;
    tpl.value = rule.template;
    tpl.addEventListener("input", function () {
      rule.template = tpl.value; // empty falls back to the type default on next load
      save();
    });
    var tplField = field("Template — {author} {count} {viewers} {months} {tier}", tpl);
    tplField.className = "field field-wide";
    card.appendChild(tplField);

    if (info.magnitude) card.appendChild(tiersBlock(info.id));
    return card;
  }

  function renderCards() {
    var host = $("cards");
    host.textContent = "";
    A.eventTypes.forEach(function (info) {
      host.appendChild(typeCard(info));
    });
  }

  // ── overlay instances ───────────────────────────────────────────────────────
  function showUrlFallback(url) {
    var box = $("url-fallback");
    var input = $("overlay-url");
    input.value = url;
    box.hidden = false;
    input.focus();
    input.select();
  }

  function copyToClipboard(url) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(url);
    }
    return Promise.reject(new Error("clipboard unavailable"));
  }

  function copyOverlayUrl(overlay) {
    notice("");
    v.send({ type: "overlay.url", payload: { path: "overlay.html" } })
      .then(function (res) {
        var url = res && res.url;
        if (!url) throw new Error("no URL returned");
        url += "&overlay=" + encodeURIComponent(overlay.id);
        $("obs-hint").hidden = false;
        // overlay.url may have written overlay_token into the plugin config behind our back;
        // refresh the cached full config so the next save doesn't clobber it.
        v.send({ type: "config.get" })
          .then(function (cfg) {
            if (cfg && typeof cfg === "object") {
              fullCfg = cfg;
              fullCfg.rules = rules;
              fullCfg.overlays = overlays;
            }
          })
          .catch(function () { /* keep the cached copy */ });
        return copyToClipboard(url)
          .then(function () { status("Copied “" + overlay.name + "” URL"); })
          .catch(function () { showUrlFallback(url); status("Copy the URL below"); });
      })
      .catch(function (e) {
        var msg = e && e.message ? e.message : "";
        if (/unknown message type/i.test(msg) || /overlay url unavailable/i.test(msg)) {
          notice("This Virta version doesn't support overlay URLs yet — update Virta to use the overlay.");
        } else {
          notice("Couldn't get the overlay URL" + (msg ? ": " + msg : "") + ".");
        }
      });
  }

  function newOverlayId() {
    var id;
    do {
      id = "ov-" + Math.random().toString(36).slice(2, 8);
    } while (overlays.some(function (o) { return o.id === id; }));
    return id;
  }

  function overlayRow(overlay) {
    var row = el("div", "overlay-row");

    var name = el("input", "input input-name");
    name.type = "text";
    name.maxLength = 40;
    name.value = overlay.name;
    name.addEventListener("input", function () {
      overlay.name = name.value.trim() || "Overlay";
      save();
    });
    row.appendChild(field("Name", name));

    // Type filter chips: none selected = all alert types play on this overlay.
    var chips = el("div", "chip-group");
    A.eventTypes.forEach(function (info) {
      var chip = el("button", "chip" + (overlay.types.indexOf(info.id) >= 0 ? " chip-on" : ""), info.label);
      chip.type = "button";
      chip.addEventListener("click", function () {
        var i = overlay.types.indexOf(info.id);
        if (i >= 0) overlay.types.splice(i, 1);
        else overlay.types.push(info.id);
        chip.classList.toggle("chip-on", i < 0);
        hint.textContent = filterHint(overlay);
        save();
      });
      chips.appendChild(chip);
    });
    var chipField = el("div", "field field-chips");
    chipField.appendChild(el("span", "field-label", "Alert types"));
    chipField.appendChild(chips);
    var hint = el("span", "chip-hint", filterHint(overlay));
    chipField.appendChild(hint);
    row.appendChild(chipField);

    var pos = el("select", "select");
    [["top", "Top"], ["center", "Center"], ["bottom", "Bottom"]].forEach(function (p) {
      var o = el("option", null, p[1]);
      o.value = p[0];
      pos.appendChild(o);
    });
    pos.value = overlay.position;
    pos.addEventListener("change", function () {
      overlay.position = pos.value;
      save();
    });
    row.appendChild(field("Position", pos));

    var actions = el("div", "overlay-actions");
    var copy = el("button", "btn btn-primary btn-small", "Copy URL");
    copy.type = "button";
    copy.addEventListener("click", function () { copyOverlayUrl(overlay); });
    actions.appendChild(copy);

    var rm = el("button", "btn btn-small btn-rm", "×");
    rm.type = "button";
    rm.title = overlays.length <= 1 ? "Keep at least one overlay" : "Remove overlay (its URL stops working)";
    rm.disabled = overlays.length <= 1;
    rm.addEventListener("click", function () {
      var i = overlays.indexOf(overlay);
      if (i >= 0 && overlays.length > 1) {
        overlays.splice(i, 1);
        save();
        renderOverlays();
      }
    });
    actions.appendChild(rm);
    row.appendChild(actions);

    return row;
  }

  function filterHint(overlay) {
    return overlay.types.length === 0 ? "No filter — every alert type plays here." : "Only the selected types play here.";
  }

  function renderOverlays() {
    var host = $("overlays");
    host.textContent = "";
    overlays.forEach(function (o) {
      host.appendChild(overlayRow(o));
    });
  }

  // ── live events → preview ───────────────────────────────────────────────────
  function onLiveMessage(m) {
    if (!m || typeof m !== "object" || !rules.enabled) return;
    var info = A.typeInfo(m.type);
    if (!info) return;
    var rule = rules.types[m.type];
    if (!rule || !rule.enabled) return;
    // FeedMessages may carry structured magnitudes in m.event; otherwise parse the body text —
    // the same extraction the overlay applies to the raw wire.
    var ev = m.event || {};
    var mags = A.parseMagnitudes(m.type, m.body || "");
    player.enqueue({
      type: m.type,
      author: m.author || "Someone",
      count: ev.count != null ? ev.count : mags.count,
      viewers: ev.viewers != null ? ev.viewers : mags.viewers,
      months: ev.months != null ? ev.months : mags.months,
      tier: ev.tier != null ? ev.tier : mags.tier,
    }, rule);
  }

  // ── boot ────────────────────────────────────────────────────────────────────
  if (!v) {
    notice("Virta bridge unavailable — reload the panel.");
    return;
  }

  player = A.createPlayer($("stage"));

  $("add-overlay").addEventListener("click", function () {
    overlays.push({ id: newOverlayId(), name: "Overlay " + (overlays.length + 1), types: [], position: "bottom" });
    save();
    renderOverlays();
  });

  $("master").addEventListener("change", function () {
    rules.enabled = this.checked;
    save();
  });

  $("position").addEventListener("change", function () {
    rules.position = this.value;
    setPosition(rules.position);
    save();
  });

  v.send({ type: "config.get" })
    .then(function (cfg) {
      if (cfg && typeof cfg === "object") fullCfg = cfg;
      var hadRules = !!(fullCfg.rules && typeof fullCfg.rules === "object" && fullCfg.rules.types);
      rules = A.normalizeRules(fullCfg.rules);
      overlays = A.normalizeOverlays(fullCfg.overlays, rules);
      if (!hadRules) saveNow(); // first run: persist defaults so the overlay sees them
    })
    .catch(function () {
      notice("Couldn't load saved settings — showing defaults; changes may not persist.");
    })
    .then(function () {
      $("master").checked = !!rules.enabled;
      $("position").value = rules.position;
      setPosition(rules.position);
      renderOverlays();
      renderCards();

      v.on("events.message", onLiveMessage); // registered before subscribing so nothing slips past
      v.send({ type: "events.subscribe", payload: {} }).catch(function () {
        // No live mirror — tests still work; the overlay subscribes on its own.
      });
    });

  window.addEventListener("pagehide", function () {
    v.send({ type: "events.unsubscribe" }).catch(function () { /* closing anyway */ });
  });
})();

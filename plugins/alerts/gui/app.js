/*
 * Alerts — configurator panel.
 *
 * Sandboxed: this page makes no network calls itself (CSP blocks them). Everything privileged
 * goes through the window.__virta postMessage bridge: rules persistence via config.get/set,
 * live events via events.subscribe → events.message, and the tokenized OBS URL via overlay.url.
 *
 * UI model:
 *  - Tab strip across the top switches between event types; only one is shown at a time.
 *  - Animation and style pickers open as full-panel modals with live hover previews.
 *  - Test fires instantly via player.flash() and also relays to open OBS overlays via
 *    localStorage (same daemon origin → storage event fires in overlay browser sources).
 *  - Custom CSS is injected into the preview immediately; custom JS goes to overlays only.
 */
(function () {
  "use strict";
  var v = window.__virta;
  var A = window.VirtaAlertAnimations;

  var SAVE_DEBOUNCE_MS = 500;
  var AUTHORS = ["nova_tide", "PixelPanda", "ghostriderTTV", "MossyOak", "Kaelthura", "retro_rita", "BinaryBard", "LunaSpark"];

  var fullCfg = {};
  var rules = A.defaultRules();
  var overlays = A.defaultOverlays(rules);
  var player = null;
  var saveTimer = null;
  var statusTimer = null;
  var selectedType = A.eventTypes[0].id;
  var pickerCleanup = null;

  // ── tiny DOM helpers ─────────────────────────────────────────────────────
  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }
  function $(id) { return document.getElementById(id); }
  function pick(list) { return list[(Math.random() * list.length) | 0]; }

  function status(text, isError) {
    var s = $("status");
    s.textContent = text;
    s.className = "status" + (isError ? " status-err" : "");
    if (statusTimer) clearTimeout(statusTimer);
    if (text && !isError) statusTimer = setTimeout(function () { s.textContent = ""; }, 2000);
  }

  function notice(text) {
    var n = $("notice");
    n.textContent = text || "";
    n.hidden = !text;
  }

  // ── persistence ──────────────────────────────────────────────────────────
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

  // ── export / import ─────────────────────────────────────────────────────────
  function exportConfig() {
    fullCfg.rules = rules;
    fullCfg.overlays = overlays;
    var exportable = {
      _version: 1,
      rules: fullCfg.rules,
      overlays: fullCfg.overlays,
      styleOverrides: fullCfg.styleOverrides || {},
      animOverrides: fullCfg.animOverrides || {},
    };
    var json = JSON.stringify(exportable, null, 2);

    if (window.showSaveFilePicker) {
      window.showSaveFilePicker({
        suggestedName: "virta-alerts.json",
        types: [{ description: "JSON config", accept: { "application/json": [".json"] } }],
      }).then(function (handle) {
        return handle.createWritable().then(function (writable) {
          return writable.write(json).then(function () { return writable.close(); });
        });
      }).then(function () {
        status("Config saved");
      }).catch(function (e) {
        if (e && e.name === "AbortError") return; // user cancelled
        status("Export failed");
      });
      return;
    }

    // Fallback: trigger a download to the default downloads folder.
    var blob = new Blob([json], { type: "application/json" });
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = "virta-alerts.json";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(function () { URL.revokeObjectURL(url); }, 5000);
    status("Config exported");
  }

  function importConfig(file) {
    var reader = new FileReader();
    reader.onload = function (e) {
      var parsed;
      try { parsed = JSON.parse(e.target.result); } catch (err) {
        notice("Import failed: not valid JSON.");
        return;
      }
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        notice("Import failed: expected a JSON object.");
        return;
      }
      // Strip the version marker; pass everything else directly as the new config.
      delete parsed._version;
      v.send({ type: "config.set", payload: { values: parsed } })
        .then(function () {
          // Reload the page so the full boot sequence re-applies the imported config.
          window.location.reload();
        })
        .catch(function (err) {
          notice("Import failed: " + (err && err.message ? err.message : "could not save."));
        });
    };
    reader.onerror = function () { notice("Import failed: couldn't read file."); };
    reader.readAsText(file);
  }

  $("btn-export").addEventListener("click", exportConfig);
  $("btn-import").addEventListener("click", function () { $("import-file").click(); });
  $("import-file").addEventListener("change", function () {
    if (this.files && this.files[0]) {
      importConfig(this.files[0]);
      this.value = ""; // reset so the same file can be re-imported
    }
  });

  // ── preview playback ─────────────────────────────────────────────────────
  function sampleData(type, forcedMagnitude) {
    var d = { type: type, author: pick(AUTHORS), tier: pick(["1", "1", "2", "3", "Prime"]) };
    if (type === "giftsub") d.count = forcedMagnitude != null ? forcedMagnitude : pick([1, 5, 10, 25, 50, 100]);
    if (type === "raid" || type === "host") d.viewers = forcedMagnitude != null ? forcedMagnitude : pick([12, 48, 86, 210, 560]);
    if (type === "resub") d.months = forcedMagnitude != null ? forcedMagnitude : pick([2, 3, 7, 12, 24, 36]);
    return d;
  }

  function playTest(type, forcedMagnitude) {
    var data = sampleData(type, forcedMagnitude);
    var rule = rules.types[type];
    player.flash(data, rule);
    // Relay to OBS overlay browser sources via the daemon signal relay (works across Electron ↔ OBS).
    var sig = { data: data, rule: rule, ts: Date.now() };
    v.send({ type: "test.relay", payload: sig }).catch(function () {
      // Fallback: localStorage relay for same-origin (same browser) setups.
      try { localStorage.setItem("va-test", JSON.stringify(sig)); } catch (e) { /* quota/private */ }
    });
  }

  function setPosition(pos) {
    $("stage").className = "stage pos-" + (pos === "top" || pos === "center" ? pos : "bottom");
  }

  // ── sample card builder for pickers ─────────────────────────────────────
  function buildSampleCard(styleId) {
    var style = A.getStyle(styleId);
    var card = el("div", "va-card va-type-sub " + style.className);
    card.appendChild(el("span", "va-badge", "NEW SUB"));
    card.appendChild(el("div", "va-text", "nova_tide just subscribed!"));
    return card;
  }

  // ── picker modal ─────────────────────────────────────────────────────────
  function closePicker() {
    $("picker").classList.remove("picker-visible");
    if (pickerCleanup) { pickerCleanup(); pickerCleanup = null; }
  }

  // cfg: { title, gridClass, items, currentId, allowInherit, renderItem(item, isActive, onSelect), onSelect }
  function openPicker(cfg) {
    $("picker-title").textContent = cfg.title;
    var grid = $("picker-grid");
    grid.textContent = "";
    grid.className = "picker-grid " + (cfg.gridClass || "picker-grid-2");

    var cleanups = [];

    if (cfg.allowInherit) {
      var inh = el("div", "picker-item picker-item-inherit" + (!cfg.currentId ? " picker-item-active" : ""), "— Inherit from type —");
      inh.addEventListener("click", function () { cfg.onSelect(""); closePicker(); });
      grid.appendChild(inh);
    }

    cfg.items.forEach(function (item) {
      var div = cfg.renderItem(item, item.id === cfg.currentId, function () {
        cfg.onSelect(item.id);
        closePicker();
      });
      if (div._cleanup) cleanups.push(div._cleanup);
      grid.appendChild(div);
    });

    pickerCleanup = function () { cleanups.forEach(function (fn) { fn(); }); };
    $("picker-close").onclick = closePicker;
    $("picker").classList.add("picker-visible");
  }

  // Animation picker item: mini checkerboard stage, hover plays enter animation.
  function animPickerItem(preset, isActive, onSelect) {
    var item = el("div", "picker-item" + (isActive ? " picker-item-active" : ""));
    var mini = el("div", "picker-mini-stage");
    var card = buildSampleCard("gradient-glass");
    card.style.opacity = "1";
    mini.appendChild(card);
    item.appendChild(mini);
    item.appendChild(el("span", "picker-item-name", preset.name));

    function onEnter() {
      (card.getAnimations ? card.getAnimations() : []).forEach(function (a) { try { a.cancel(); } catch (e) { } });
      A.stopHold(card);
      card.style.cssText = "";
      preset.enter(card);
    }
    function onLeave() {
      (card.getAnimations ? card.getAnimations() : []).forEach(function (a) { try { a.cancel(); } catch (e) { } });
      A.stopHold(card);
      card.style.cssText = "opacity:1;transform:none;filter:none;clip-path:none";
    }
    item.addEventListener("mouseenter", onEnter);
    item.addEventListener("mouseleave", onLeave);
    item.addEventListener("click", onSelect);
    item._cleanup = function () {
      item.removeEventListener("mouseenter", onEnter);
      item.removeEventListener("mouseleave", onLeave);
    };
    return item;
  }

  // Style picker item: static card rendered in each style so users see the real look.
  function stylePickerItem(style, isActive, onSelect) {
    var item = el("div", "picker-item picker-item-style" + (isActive ? " picker-item-active" : ""));
    item.appendChild(buildSampleCard(style.id));
    item.appendChild(el("span", "picker-item-name", style.name));
    item.addEventListener("click", onSelect);
    return item;
  }

  function openAnimationPicker(current, allowInherit, onSelect) {
    openPicker({
      title: "Choose animation",
      gridClass: "picker-grid-2",
      items: A.presets,
      currentId: current,
      allowInherit: allowInherit,
      renderItem: animPickerItem,
      onSelect: onSelect,
    });
  }

  function openStylePicker(current, allowInherit, onSelect) {
    openPicker({
      title: "Choose style",
      gridClass: "picker-grid-2",
      items: A.styles,
      currentId: current,
      allowInherit: allowInherit,
      renderItem: stylePickerItem,
      onSelect: onSelect,
    });
  }

  // ── pick buttons (replace <select> for animation / style) ────────────────
  function makePickBtn(labelText, onClick) {
    var btn = el("button", "pick-btn");
    btn.type = "button";
    var lbl = el("span", "pick-btn-label", labelText);
    btn.appendChild(lbl);
    btn.appendChild(el("span", "pick-btn-arrow", "▾"));
    btn.addEventListener("click", onClick);
    return { btn: btn, lbl: lbl };
  }

  function animBtn(value, allowInherit, onChange) {
    var cur = value;
    var display = cur ? A.get(cur).name : (allowInherit ? "— Inherit —" : "None");
    var p = makePickBtn(display, function () {
      openAnimationPicker(cur, allowInherit, function (id) {
        cur = id;
        p.lbl.textContent = id ? A.get(id).name : "— Inherit —";
        onChange(id);
      });
    });
    return p.btn;
  }

  function styleBtn(value, allowInherit, onChange) {
    var cur = value;
    var display = cur ? A.getStyle(cur).name : (allowInherit ? "— Inherit —" : "None");
    var p = makePickBtn(display, function () {
      openStylePicker(cur, allowInherit, function (id) {
        cur = id;
        p.lbl.textContent = id ? A.getStyle(id).name : "— Inherit —";
        onChange(id);
      });
    });
    return p.btn;
  }

  function field(labelText, control) {
    var wrap = el("label", "field");
    wrap.appendChild(el("span", "field-label", labelText));
    wrap.appendChild(control);
    return wrap;
  }

  // ── tier rows ─────────────────────────────────────────────────────────────
  var MAGNITUDE_LABEL = { count: "Min gifts", viewers: "Min viewers", months: "Min months" };

  function tierRow(typeId, tier, index) {
    var rule = rules.types[typeId];
    var row = el("div", "tier-row");

    var min = el("input", "input input-num");
    min.type = "number"; min.min = "0"; min.step = "1"; min.value = String(tier.min);
    min.addEventListener("change", function () {
      var n = parseInt(min.value, 10);
      tier.min = isFinite(n) && n >= 0 ? n : 0;
      min.value = String(tier.min);
      save();
    });
    row.appendChild(field(MAGNITUDE_LABEL[A.typeInfo(typeId).magnitude] || "Min", min));

    row.appendChild(field("Animation", animBtn(tier.animation || "", true, function (id) {
      if (id) tier.animation = id; else delete tier.animation;
      save();
    })));

    row.appendChild(field("Style", styleBtn(tier.style || "", true, function (id) {
      if (id) tier.style = id; else delete tier.style;
      save();
    })));

    var dur = el("input", "input input-num");
    dur.type = "number"; dur.min = "1"; dur.max = "60"; dur.step = "0.5"; dur.placeholder = "inherit";
    if (tier.durationMs) dur.value = String(tier.durationMs / 1000);
    dur.addEventListener("change", function () {
      var n = parseFloat(dur.value);
      if (isFinite(n) && n > 0) tier.durationMs = Math.min(60000, Math.max(1000, Math.round(n * 1000)));
      else { delete tier.durationMs; dur.value = ""; }
      save();
    });
    row.appendChild(field("Seconds", dur));

    var tpl = el("input", "input");
    tpl.type = "text"; tpl.maxLength = 200; tpl.placeholder = "inherit"; tpl.value = tier.template || "";
    tpl.addEventListener("input", function () {
      if (tpl.value) tier.template = tpl.value; else delete tier.template;
      save();
    });
    row.appendChild(field("Template", tpl));

    var actions = el("div", "tier-actions");

    var testBtn = el("button", "btn btn-small", "Test");
    testBtn.type = "button";
    testBtn.title = "Play a sample alert at this tier's threshold";
    testBtn.addEventListener("click", function () { playTest(typeId, tier.min); });
    actions.appendChild(testBtn);

    var rm = el("button", "btn btn-small btn-rm", "×");
    rm.type = "button"; rm.title = "Remove tier";
    rm.addEventListener("click", function () {
      rule.tiers.splice(index, 1);
      save();
      renderTypeEditor();
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

    rule.tiers.forEach(function (tier, i) { block.appendChild(tierRow(typeId, tier, i)); });

    var add = el("button", "btn btn-small btn-add", "+ Add tier");
    add.type = "button";
    add.addEventListener("click", function () {
      var last = rule.tiers.length ? rule.tiers[rule.tiers.length - 1].min : 0;
      rule.tiers.push({ min: last > 0 ? last * 2 : 10 });
      save();
      renderTypeEditor();
    });
    block.appendChild(add);
    return block;
  }

  // ── code editors (style CSS / animation JS inline in type card) ─────────
  // Returns a section element with expand/collapse, Modified badge, and Reset button.
  // cfg: { title, kind ('style'|'anim'), id, baseCode, overrides }
  // overrides is fullCfg.styleOverrides or fullCfg.animOverrides (live reference).
  function codeSection(cfg) {
    var sec = el("div", "code-sec");
    var head = el("div", "code-sec-head");
    var toggle = el("span", "code-sec-toggle", cfg.title + ": " + cfg.name);
    var hasOverride = !!(cfg.overrides[cfg.id]);
    var badge = el("span", "code-badge " + (hasOverride ? "code-badge-modified" : "code-badge-base"),
      hasOverride ? "Modified" : "Original");
    var actions = el("div", "code-sec-actions");
    var resetBtn = el("button", "btn btn-small", "Reset");
    resetBtn.type = "button";
    resetBtn.title = "Discard changes, restore original";
    resetBtn.style.display = hasOverride ? "" : "none";
    actions.appendChild(resetBtn);
    head.appendChild(toggle);
    head.appendChild(badge);
    head.appendChild(actions);
    sec.appendChild(head);

    var body = el("div", "code-sec-body");
    if (cfg.kind === "style") {
      body.appendChild(el("div", "code-note", "Edits apply to all events using this style. Changes override the original CSS."));
    } else {
      body.appendChild(el("div", "code-note", "Edits apply to all events using this animation. Use el.animate(), holdLoop(), stopHold(), burstConfetti()."));
    }
    var cmWrap = el("div", "code-cm-wrap");
    body.appendChild(cmWrap);
    var errMsg = el("div", "code-err");
    errMsg.hidden = true;
    body.appendChild(errMsg);
    sec.appendChild(body);

    // Initialise the CodeMirror instance after this element is appended to the DOM.
    // We defer via setTimeout(0) so the element has layout (CM needs a parent with dimensions).
    var cm = null;
    var cmPending = cfg.overrides[cfg.id] || cfg.baseCode;

    function initCm() {
      if (cm) return;
      cm = window.CodeMirror(cmWrap, {
        value: cmPending,
        mode: cfg.kind === "style" ? "css" : "javascript",
        theme: "virta-dark",
        indentUnit: 2,
        tabSize: 2,
        indentWithTabs: false,
        lineNumbers: true,
        lineWrapping: false,
        autoCloseBrackets: true,
        extraKeys: { Tab: function (c) { c.execCommand("insertSoftTab"); } },
      });
      cm.on("change", onCmChange);
    }

    // Expand / collapse on head click; initialise CM on first open.
    head.addEventListener("click", function (e) {
      if (e.target === resetBtn) return;
      var opening = !sec.classList.contains("code-sec-open");
      sec.classList.toggle("code-sec-open");
      if (opening) {
        setTimeout(function () {
          initCm();
          if (cm) cm.refresh();
        }, 0);
      }
    });

    var codeTimer = null;
    function onCmChange() {
      badge.className = "code-badge code-badge-modified";
      badge.textContent = "Modified";
      resetBtn.style.display = "";
      errMsg.hidden = true;
      cmWrap.classList.remove("code-editor-error");

      if (codeTimer) clearTimeout(codeTimer);
      codeTimer = setTimeout(function () {
        var code = cm ? cm.getValue() : cmPending;
        if (cfg.kind === "anim") {
          try {
            A.setCustomPreset(cfg.id, code);
            errMsg.hidden = true;
            cmWrap.classList.remove("code-editor-error");
          } catch (e) {
            errMsg.textContent = String(e.message || e);
            errMsg.hidden = false;
            cmWrap.classList.add("code-editor-error");
            return;
          }
        } else {
          A.setStyleOverride(cfg.id, code);
        }
        cfg.overrides[cfg.id] = code;
        save();
      }, 600);
    }

    resetBtn.addEventListener("click", function () {
      if (cm) {
        cm.setValue(cfg.baseCode);
      } else {
        cmPending = cfg.baseCode;
      }
      delete cfg.overrides[cfg.id];
      badge.className = "code-badge code-badge-base";
      badge.textContent = "Original";
      resetBtn.style.display = "none";
      errMsg.hidden = true;
      cmWrap.classList.remove("code-editor-error");
      if (cfg.kind === "anim") {
        A.removeCustomPreset(cfg.id);
      } else {
        A.clearStyleOverride(cfg.id);
      }
      save();
      playTest(cfg.typeId);
    });

    // Expose update function so the type card can refresh content when picker changes selection.
    sec.updateFor = function (newId, newName, newBaseCode) {
      cfg.id = newId;
      cfg.name = newName;
      cfg.baseCode = newBaseCode;
      toggle.textContent = cfg.title + ": " + newName;
      var ov = cfg.overrides[newId];
      var newVal = ov || newBaseCode;
      if (cm) {
        cm.setValue(newVal);
      } else {
        cmPending = newVal;
      }
      errMsg.hidden = true;
      cmWrap.classList.remove("code-editor-error");
      var modified = !!(ov);
      badge.className = "code-badge " + (modified ? "code-badge-modified" : "code-badge-base");
      badge.textContent = modified ? "Modified" : "Original";
      resetBtn.style.display = modified ? "" : "none";
    };

    return sec;
  }

  // ── type navigation (vertical list in left panel) ────────────────────────
  function renderTypeTabs() {
    var nav = $("type-nav");
    nav.textContent = "";
    A.eventTypes.forEach(function (info) {
      var rule = rules.types[info.id];
      var btn = el("button", "type-nav-item" + (info.id === selectedType ? " type-nav-item-active" : ""));
      btn.type = "button";
      var dot = el("span", "type-nav-dot" + (rule && rule.enabled ? " type-nav-dot-on" : ""));
      btn.appendChild(dot);
      btn.appendChild(document.createTextNode(info.label));
      btn.addEventListener("click", function () {
        selectedType = info.id;
        renderTypeTabs();
        renderTypeEditor();
      });
      nav.appendChild(btn);
    });
  }

  // ── accordion section builder ─────────────────────────────────────────────
  // kindClass: 'visual'|'audio'|'queue'|'media'|'webhook'; expanded: bool; buildFn(body)
  function accordSection(kindClass, label, expanded, buildFn) {
    var sec = el("div", "accord-sec accord-" + kindClass);
    var head = el("button", "accord-head", "");
    head.type = "button";
    head.appendChild(el("span", "accord-dot"));
    head.appendChild(el("span", "accord-label", label));
    var chev = el("span", "accord-chevron", "▶");
    head.appendChild(chev);
    sec.appendChild(head);

    var body = el("div", "accord-body");
    if (!expanded) body.hidden = true;
    else sec.classList.add("accord-open");
    buildFn(body);

    head.addEventListener("click", function () {
      var open = !body.hidden;
      body.hidden = open;
      sec.classList.toggle("accord-open", !open);
    });
    sec.appendChild(body);
    return sec;
  }

  // ── single event-type editor ──────────────────────────────────────────────
  function typeCard(info) {
    var rule = rules.types[info.id];
    var defRule = A.defaultRules().types[info.id];
    var card = el("section", "card");

    // ── head: enable toggle + test button ──────────────────────────────────
    var head = el("div", "card-head");
    var toggle = el("label", "card-toggle");
    var enabled = el("input");
    enabled.type = "checkbox"; enabled.checked = !!rule.enabled;
    enabled.addEventListener("change", function () {
      rule.enabled = enabled.checked;
      card.classList.toggle("card-off", !rule.enabled);
      save();
    });
    toggle.appendChild(enabled);
    toggle.appendChild(el("span", "card-title", info.label));
    head.appendChild(toggle);

    var testBtn = el("button", "btn btn-primary btn-small", "▶ Test");
    testBtn.type = "button";
    testBtn.title = "Play a sample " + info.label.toLowerCase() + " alert";
    testBtn.addEventListener("click", function () { playTest(info.id); });
    head.appendChild(testBtn);
    card.appendChild(head);
    card.classList.toggle("card-off", !rule.enabled);

    // Ensure override maps exist.
    if (!fullCfg.styleOverrides) fullCfg.styleOverrides = {};
    if (!fullCfg.animOverrides) fullCfg.animOverrides = {};

    // Code editors are built early so animation/style picker callbacks can update them.
    var styleSec = codeSection({
      title: "Style CSS", kind: "style", id: rule.style,
      name: A.getStyle(rule.style).name,
      baseCode: A.getStyle(rule.style).css || "",
      overrides: fullCfg.styleOverrides, typeId: info.id,
    });
    var animSec = codeSection({
      title: "Animation JS", kind: "anim", id: rule.animation,
      name: A.get(rule.animation).name,
      baseCode: (function () {
        for (var i = 0; i < A.presets.length; i++) {
          if (A.presets[i].id === rule.animation) return A.presets[i].template || "";
        }
        return "";
      }()),
      overrides: fullCfg.animOverrides, typeId: info.id,
    });

    // ── quick controls: animation, style, duration, template ───────────────
    var quick = el("div", "card-quick");
    var grid = el("div", "card-grid");

    grid.appendChild(field("Animation", animBtn(rule.animation, false, function (id) {
      rule.animation = id || defRule.animation;
      var newTpl = "";
      for (var pi = 0; pi < A.presets.length; pi++) {
        if (A.presets[pi].id === rule.animation) { newTpl = A.presets[pi].template || ""; break; }
      }
      animSec.updateFor(rule.animation, A.get(rule.animation).name, newTpl);
      save(); playTest(info.id);
    })));
    grid.appendChild(field("Style", styleBtn(rule.style, false, function (id) {
      rule.style = id || defRule.style;
      var styleObj = A.getStyle(rule.style);
      styleSec.updateFor(rule.style, styleObj.name, styleObj.css || "");
      save(); playTest(info.id);
    })));

    var dur = el("input", "input input-num");
    dur.type = "number"; dur.min = "1"; dur.max = "60"; dur.step = "0.5";
    dur.value = String(rule.durationMs / 1000);
    dur.addEventListener("change", function () {
      var n = parseFloat(dur.value);
      rule.durationMs = isFinite(n) && n > 0 ? Math.min(60000, Math.max(1000, Math.round(n * 1000))) : 6000;
      dur.value = String(rule.durationMs / 1000);
      save();
    });
    grid.appendChild(field("Seconds", dur));
    quick.appendChild(grid);

    var tpl = el("input", "input");
    tpl.type = "text"; tpl.maxLength = 200; tpl.value = rule.template;
    tpl.addEventListener("input", function () { rule.template = tpl.value; save(); });
    var tplField = field("Template — {author} {count} {viewers} {months} {tier}", tpl);
    tplField.className = "field field-wide";
    quick.appendChild(tplField);
    card.appendChild(quick);

    // ── accordion sections ─────────────────────────────────────────────────

    var sections = el("div", "accord-sections");

    // — Visual —
    sections.appendChild(accordSection("visual", "Visual", true, function (body) {
      function poolChips(label, poolArr, openPicker, allowInherit) {
        var wrap = el("div", "pool-wrap");
        wrap.appendChild(el("span", "pool-label", label + ":"));
        var chips = el("div", "pool-chips");
        function redraw() {
          chips.textContent = "";
          poolArr.forEach(function (id, i) {
            var chip = el("span", "pool-chip", id);
            var rm = el("button", "pool-chip-rm", "\xd7");
            rm.type = "button";
            rm.addEventListener("click", function () { poolArr.splice(i, 1); save(); redraw(); });
            chip.appendChild(rm);
            chips.appendChild(chip);
          });
          var add = el("button", "btn pool-chip-add", "+");
          add.type = "button"; add.title = "Add a variation";
          add.addEventListener("click", function () {
            openPicker(null, allowInherit, function (id) {
              if (id && poolArr.indexOf(id) === -1) { poolArr.push(id); save(); redraw(); }
            });
          });
          chips.appendChild(add);
        }
        redraw();
        wrap.appendChild(chips);
        return wrap;
      }

      body.appendChild(poolChips("Anim pool",  rule.animPool,  openAnimationPicker, false));
      body.appendChild(poolChips("Style pool", rule.stylePool, openStylePicker, false));

      var badgeRow = el("div", "badge-row");
      var badgeIn = el("input", "input badge-input");
      badgeIn.type = "text"; badgeIn.maxLength = 32;
      badgeIn.value = rule.badge || ""; badgeIn.placeholder = info.badge;
      badgeIn.title = "Badge label — leave blank for the default";
      badgeIn.addEventListener("input", function () { rule.badge = badgeIn.value; save(); });
      badgeRow.appendChild(field("Badge", badgeIn));
      body.appendChild(badgeRow);

      body.appendChild(styleSec);
      body.appendChild(animSec);
      if (info.magnitude) body.appendChild(tiersBlock(info.id));
    }));

    // — Audio —
    sections.appendChild(accordSection("audio", "Audio", false, function (body) {
      if (!rule.sound) rule.sound = { src: "", volume: 0.8, muted: false };
      var soundRow = el("div", "sound-row");
      var builtins = [
        ["", "None"], ["builtin:chime", "Chime"], ["builtin:pop", "Pop"],
        ["builtin:whoosh", "Whoosh"], ["builtin:bell", "Bell"],
        ["builtin:notify", "Notify"], ["builtin:cash", "Cash"],
      ];
      var soundSel = el("select", "select sound-sel");
      builtins.forEach(function (pair) {
        var opt = document.createElement("option");
        opt.value = pair[0]; opt.textContent = pair[1];
        soundSel.appendChild(opt);
      });
      var customOpt = document.createElement("option");
      customOpt.value = "__custom"; customOpt.textContent = "Custom URL…";
      soundSel.appendChild(customOpt);

      var isBuiltin = builtins.some(function (p) { return p[0] === rule.sound.src; });
      soundSel.value = isBuiltin ? rule.sound.src : (rule.sound.src ? "__custom" : "");
      var customUrlIn = el("input", "input sound-url");
      customUrlIn.type = "text"; customUrlIn.placeholder = "https://…/sound.mp3";
      customUrlIn.value = (!isBuiltin && rule.sound.src) ? rule.sound.src : "";
      customUrlIn.hidden = soundSel.value !== "__custom";

      soundSel.addEventListener("change", function () {
        customUrlIn.hidden = soundSel.value !== "__custom";
        rule.sound.src = soundSel.value === "__custom" ? customUrlIn.value : soundSel.value;
        save();
      });
      customUrlIn.addEventListener("input", function () { rule.sound.src = customUrlIn.value; save(); });

      var volSlider = el("input", "sound-vol");
      volSlider.type = "range"; volSlider.min = "0"; volSlider.max = "1"; volSlider.step = "0.05";
      volSlider.value = String(typeof rule.sound.volume === "number" ? rule.sound.volume : 0.8);
      volSlider.title = "Volume";
      volSlider.addEventListener("input", function () { rule.sound.volume = parseFloat(volSlider.value); save(); });

      var muteBtn = el("button", "btn btn-small sound-mute", rule.sound.muted ? "🔇" : "🔊");
      muteBtn.type = "button"; muteBtn.title = "Mute / unmute";
      muteBtn.addEventListener("click", function () {
        rule.sound.muted = !rule.sound.muted;
        muteBtn.textContent = rule.sound.muted ? "🔇" : "🔊";
        save();
      });
      var testSoundBtn = el("button", "btn btn-small", "▶");
      testSoundBtn.type = "button"; testSoundBtn.title = "Preview sound";
      testSoundBtn.addEventListener("click", function () { A.playSound(rule.sound); });

      soundRow.appendChild(el("span", "sound-label", "Sound"));
      soundRow.appendChild(soundSel);
      soundRow.appendChild(customUrlIn);
      soundRow.appendChild(volSlider);
      soundRow.appendChild(muteBtn);
      soundRow.appendChild(testSoundBtn);
      body.appendChild(soundRow);

      // TTS
      if (!rule.tts) rule.tts = { enabled: false, rate: 1.0, pitch: 1.0, template: "" };
      var ttsRow = el("div", "tts-row");
      var ttsLbl = el("label", "tts-toggle");
      var ttsChk = el("input"); ttsChk.type = "checkbox"; ttsChk.checked = !!rule.tts.enabled;
      var ttsControls = el("div", "tts-controls");
      ttsControls.hidden = !rule.tts.enabled;
      ttsChk.addEventListener("change", function () {
        rule.tts.enabled = ttsChk.checked; ttsControls.hidden = !ttsChk.checked; save();
      });
      ttsLbl.appendChild(ttsChk);
      ttsLbl.appendChild(document.createTextNode("Text-to-speech"));
      ttsRow.appendChild(ttsLbl);

      var rateSlider = el("input", "tts-slider");
      rateSlider.type = "range"; rateSlider.min = "0.5"; rateSlider.max = "2"; rateSlider.step = "0.1";
      rateSlider.value = String(rule.tts.rate || 1); rateSlider.title = "Rate";
      rateSlider.addEventListener("input", function () { rule.tts.rate = parseFloat(rateSlider.value); save(); });

      var pitchSlider = el("input", "tts-slider");
      pitchSlider.type = "range"; pitchSlider.min = "0"; pitchSlider.max = "2"; pitchSlider.step = "0.1";
      pitchSlider.value = String(rule.tts.pitch || 1); pitchSlider.title = "Pitch";
      pitchSlider.addEventListener("input", function () { rule.tts.pitch = parseFloat(pitchSlider.value); save(); });

      var ttsTpl = el("input", "input");
      ttsTpl.type = "text"; ttsTpl.maxLength = 200;
      ttsTpl.placeholder = "Custom speak text (leave blank to use template)";
      ttsTpl.value = rule.tts.template || "";
      ttsTpl.addEventListener("input", function () { rule.tts.template = ttsTpl.value; save(); });

      var ttsTestBtn = el("button", "btn btn-small", "▶ Test TTS");
      ttsTestBtn.type = "button";
      ttsTestBtn.addEventListener("click", function () {
        var sample = sampleData(info.id);
        var text = rule.tts.template ? A.templateText(rule.tts.template, sample) : A.templateText(rule.template, sample);
        A.speakAlert(text, rule.tts);
      });

      ttsControls.appendChild(field("Rate", rateSlider));
      ttsControls.appendChild(field("Pitch", pitchSlider));
      var ttsTplF = field("Speak text", ttsTpl);
      ttsTplF.className = "field field-wide";
      ttsControls.appendChild(ttsTplF);
      ttsControls.appendChild(ttsTestBtn);
      ttsRow.appendChild(ttsControls);
      body.appendChild(ttsRow);
    }));

    // — Queue —
    sections.appendChild(accordSection("queue", "Queue", false, function (body) {
      if (!rule.cooldownMs) rule.cooldownMs = 0;
      if (!rule.queueMax)   rule.queueMax   = 0;
      var queueRow = el("div", "queue-row");

      var cdIn = el("input", "input input-num queue-num");
      cdIn.type = "number"; cdIn.min = "0"; cdIn.max = "3600"; cdIn.step = "1";
      cdIn.value = String(Math.round(rule.cooldownMs / 1000));
      cdIn.title = "Skip if same type fired within this many seconds (0 = off)";
      cdIn.addEventListener("change", function () {
        var n = parseInt(cdIn.value, 10);
        rule.cooldownMs = isFinite(n) && n >= 0 ? n * 1000 : 0;
        save();
      });

      var qmIn = el("input", "input input-num queue-num");
      qmIn.type = "number"; qmIn.min = "0"; qmIn.max = "50"; qmIn.step = "1";
      qmIn.value = String(rule.queueMax || 0);
      qmIn.title = "Drop when this many of this type are waiting (0 = unlimited)";
      qmIn.addEventListener("change", function () {
        var n = parseInt(qmIn.value, 10);
        rule.queueMax = isFinite(n) && n >= 0 ? n : 0;
        save();
      });

      queueRow.appendChild(field("Cooldown (s)", cdIn));
      queueRow.appendChild(field("Max queued", qmIn));
      body.appendChild(queueRow);
    }));

    // — Media —
    sections.appendChild(accordSection("media", "Media", false, function (body) {
      if (!rule.media) rule.media = { src: "", fit: "contain", position: "top" };

      var mediaRow = el("div", "media-row");
      var srcIn = el("input", "input");
      srcIn.type = "text"; srcIn.maxLength = 1000;
      srcIn.placeholder = "https://…/image.gif  (leave blank for no image)";
      srcIn.value = rule.media.src || "";
      srcIn.addEventListener("input", function () { rule.media.src = srcIn.value.trim(); save(); });

      var fitSel = el("select", "select media-fit");
      [["contain", "Contain"], ["cover", "Cover"], ["fill", "Stretch"]].forEach(function (p) {
        var o = el("option", null, p[1]); o.value = p[0]; fitSel.appendChild(o);
      });
      fitSel.value = rule.media.fit || "contain";
      fitSel.addEventListener("change", function () { rule.media.fit = fitSel.value; save(); });

      var posSel = el("select", "select media-pos");
      [["top", "Above text"], ["bottom", "Below text"], ["bg", "Background"]].forEach(function (p) {
        var o = el("option", null, p[1]); o.value = p[0]; posSel.appendChild(o);
      });
      posSel.value = rule.media.position || "top";
      posSel.addEventListener("change", function () { rule.media.position = posSel.value; save(); });

      var srcF = field("Image / GIF URL", srcIn);
      srcF.className = "field field-wide";
      mediaRow.appendChild(srcF);
      mediaRow.appendChild(field("Fit", fitSel));
      mediaRow.appendChild(field("Position", posSel));
      body.appendChild(mediaRow);
      body.appendChild(el("div", "tab-hint", "PNG, JPG, GIF, WebP. Shown inside the alert card on OBS overlays."));
    }));

    // — Webhook —
    sections.appendChild(accordSection("webhook", "Outbound webhook", false, function (body) {
      if (!rule.webhook) rule.webhook = { url: "", enabled: false };

      var whRow = el("div", "wh-row");

      var whUrlIn = el("input", "input");
      whUrlIn.type = "text"; whUrlIn.maxLength = 1000;
      whUrlIn.placeholder = "https://… (Discord, Zapier, custom endpoint…)";
      whUrlIn.value = rule.webhook.url || "";
      whUrlIn.addEventListener("input", function () { rule.webhook.url = whUrlIn.value.trim(); save(); });

      var whEnabledLbl = el("label", "wh-enabled");
      var whChk = el("input"); whChk.type = "checkbox"; whChk.checked = !!rule.webhook.enabled;
      whChk.addEventListener("change", function () { rule.webhook.enabled = whChk.checked; save(); });
      whEnabledLbl.appendChild(whChk);
      whEnabledLbl.appendChild(document.createTextNode(" Enabled"));

      var whTestBtn = el("button", "btn btn-small", "▶ Test");
      whTestBtn.type = "button"; whTestBtn.title = "Fire a test POST to the configured URL";
      whTestBtn.addEventListener("click", function () {
        var url = rule.webhook.url.trim();
        if (!url) { status("Enter a URL first", true); return; }
        try {
          fetch(url, {
            method: "POST", mode: "no-cors",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ event: info.id, author: "TestUser", text: "Test from Virta Alerts", badge: info.badge }),
          }).then(function () { status("Webhook sent"); }).catch(function () { status("Webhook sent (no-cors)"); });
        } catch (e) { status("Failed: " + e.message, true); }
      });

      var urlF = field("POST to URL", whUrlIn);
      urlF.className = "field field-wide";
      whRow.appendChild(urlF);
      var actRow = el("div", "wh-actions");
      actRow.appendChild(whEnabledLbl);
      actRow.appendChild(whTestBtn);
      whRow.appendChild(actRow);
       body.appendChild(whRow);
      body.appendChild(el("div", "tab-hint", "Overlay POSTs { event, author, text, badge } when this alert fires. Uses no-cors — works with Discord, Zapier, Make, and most receivers."));
    }));

    card.appendChild(sections);
    return card;
  }

  function renderTypeEditor() {
    var host = $("type-editor");
    host.textContent = "";
    var info = A.typeInfo(selectedType);
    if (info) host.appendChild(typeCard(info));
  }

  // ── overlays ──────────────────────────────────────────────────────────────
  function showUrlFallback(url) {
    $("overlay-url").value = url;
    $("url-fallback").hidden = false;
    $("overlay-url").focus();
    $("overlay-url").select();
  }

  function copyToClipboard(url) {
    if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(url);
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
        v.send({ type: "config.get" })
          .then(function (cfg) {
            if (cfg && typeof cfg === "object") {
              fullCfg = cfg;
              fullCfg.rules = rules;
              fullCfg.overlays = overlays;
            }
          })
          .catch(function () { });
        return copyToClipboard(url)
          .then(function () { status("Copied “" + overlay.name + "” URL"); })
          .catch(function () { showUrlFallback(url); status("Copy the URL below"); });
      })
      .catch(function (e) {
        var msg = e && e.message ? e.message : "";
        if (/unknown message type/i.test(msg) || /overlay url unavailable/i.test(msg)) {
          notice("This Virta version doesn’t support overlay URLs yet — update Virta to use the overlay.");
        } else {
          notice("Couldn’t get the overlay URL" + (msg ? ": " + msg : "") + ".");
        }
      });
  }

  function newOverlayId() {
    var id;
    do { id = "ov-" + Math.random().toString(36).slice(2, 8); }
    while (overlays.some(function (o) { return o.id === id; }));
    return id;
  }

  function filterHint(overlay) {
    return overlay.types.length === 0
      ? "No filter — every alert type plays here."
      : "Only the selected types play here.";
  }

  function overlayRow(overlay) {
    var row = el("div", "overlay-row");

    var name = el("input", "input input-name");
    name.type = "text"; name.maxLength = 40; name.value = overlay.name;
    name.addEventListener("input", function () { overlay.name = name.value.trim() || "Overlay"; save(); });
    row.appendChild(field("Name", name));

    var chips = el("div", "chip-group");
    A.eventTypes.forEach(function (info) {
      var chip = el("button", "chip" + (overlay.types.indexOf(info.id) >= 0 ? " chip-on" : ""), info.label);
      chip.type = "button";
      chip.addEventListener("click", function () {
        var i = overlay.types.indexOf(info.id);
        if (i >= 0) overlay.types.splice(i, 1); else overlay.types.push(info.id);
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
    pos.addEventListener("change", function () { overlay.position = pos.value; save(); });
    row.appendChild(field("Position", pos));

    var actions = el("div", "overlay-actions");
    var copy = el("button", "btn btn-primary btn-small", "Copy URL");
    copy.type = "button";
    copy.addEventListener("click", function () { copyOverlayUrl(overlay); });
    actions.appendChild(copy);

    var rm = el("button", "btn btn-small btn-rm", "\xd7");
    rm.type = "button";
    rm.title = overlays.length <= 1 ? "Keep at least one overlay" : "Remove overlay (its URL stops working)";
    rm.disabled = overlays.length <= 1;
    rm.addEventListener("click", function () {
      var i = overlays.indexOf(overlay);
      if (i >= 0 && overlays.length > 1) { overlays.splice(i, 1); save(); renderOverlays(); }
    });
    actions.appendChild(rm);
    row.appendChild(actions);
    return row;
  }

  function renderOverlays() {
    var host = $("overlays");
    host.textContent = "";
    overlays.forEach(function (o) { host.appendChild(overlayRow(o)); });
  }

  // ── custom hook events ────────────────────────────────────────────────────
  function generateSecret() {
    var arr = new Uint8Array(20);
    crypto.getRandomValues(arr);
    return Array.prototype.map.call(arr, function (b) { return ("0" + b.toString(16)).slice(-2); }).join("");
  }

  function customEventRow(evt, idx) {
    var row = el("div", "custom-evt-row");

    var nameWrap = el("label", "custom-evt-name-wrap");
    var nameIn = el("input", "input custom-evt-name-input");
    nameIn.type = "text"; nameIn.maxLength = 40; nameIn.value = evt.name;
    nameIn.placeholder = "e.g. Donation";
    nameIn.addEventListener("input", function () {
      evt.name = nameIn.value.trim() || ("event-" + (idx + 1));
      save();
    });
    nameWrap.appendChild(nameIn);

    var badgeIn = el("input", "input custom-evt-badge-input");
    badgeIn.type = "text"; badgeIn.maxLength = 16; badgeIn.value = evt.badge || "";
    badgeIn.placeholder = "BADGE";
    badgeIn.title = "Badge text shown in the card (defaults to event name in caps)";
    badgeIn.addEventListener("input", function () { evt.badge = badgeIn.value.trim(); save(); });
    nameWrap.appendChild(badgeIn);
    row.appendChild(nameWrap);

    var tpl = el("input", "input custom-evt-tpl");
    tpl.type = "text"; tpl.maxLength = 200; tpl.value = evt.template || "{author}";
    tpl.placeholder = "Template: {author} {text} {amount} …";
    tpl.title = "Any {field} from the webhook JSON body is substituted.";
    tpl.addEventListener("input", function () { evt.template = tpl.value; save(); });
    row.appendChild(tpl);

    var pickers = el("div", "custom-evt-pickers");
    pickers.appendChild(field("Anim", animBtn(evt.animation || "pop", false, function (id) {
      evt.animation = id || "pop"; save();
    })));
    pickers.appendChild(field("Style", styleBtn(evt.style || "gradient-glass", false, function (id) {
      evt.style = id || "gradient-glass"; save();
    })));
    row.appendChild(pickers);

    var actions = el("div", "custom-evt-actions");

    var enabledLabel = el("label", "custom-evt-toggle");
    var enabledChk = el("input", "");
    enabledChk.type = "checkbox"; enabledChk.checked = evt.enabled !== false;
    enabledChk.addEventListener("change", function () { evt.enabled = this.checked; save(); });
    enabledLabel.appendChild(enabledChk);
    enabledLabel.appendChild(document.createTextNode("On"));
    actions.appendChild(enabledLabel);

    var testBtn = el("button", "btn btn-small", "Test");
    testBtn.type = "button";
    testBtn.title = "Fire a sample custom event";
    testBtn.addEventListener("click", function () {
      player.flash({
        type: "custom",
        author: "TestUser",
        badge: evt.badge || evt.name.toUpperCase(),
        _ext: { author: "TestUser", text: "This is a test payload!", amount: "9.99" },
      }, {
        animation: evt.animation || "pop",
        style: evt.style || "gradient-glass",
        template: evt.template || "{author}",
        durationMs: 5000,
      });
    });
    actions.appendChild(testBtn);

    var copyBtn = el("button", "btn btn-small", "Copy URL");
    copyBtn.type = "button";
    copyBtn.title = "Copy webhook URL to clipboard";
    copyBtn.addEventListener("click", function () {
      var slug = evt.name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || ("event-" + (idx + 1));
      v.send({ type: "overlay.url", payload: { path: "overlay.html" } })
        .then(function (res) {
          // Extract base URL (scheme+host) from the overlay URL.
          var base = res.url ? res.url.replace(/\/plugins\/.*$/, "") : window.location.origin;
          var secret = fullCfg.webhookSecret || "";
          var url = base + "/v1/plugins/com.virta.alerts/hook/" + encodeURIComponent(slug) + "?secret=" + encodeURIComponent(secret);
          return navigator.clipboard.writeText(url);
        })
        .then(function () { status("URL copied"); })
        .catch(function () { status("Copy failed — check browser permissions", true); });
    });
    actions.appendChild(copyBtn);

    var rmBtn = el("button", "btn btn-small btn-rm", "✕");
    rmBtn.type = "button"; rmBtn.title = "Remove this custom event";
    rmBtn.addEventListener("click", function () {
      fullCfg.customEvents.splice(idx, 1);
      save();
      renderCustomEvents();
    });
    actions.appendChild(rmBtn);
    row.appendChild(actions);
    return row;
  }

  function renderCustomEvents() {
    var host = $("custom-events-list");
    host.textContent = "";
    var evts = fullCfg.customEvents || [];
    evts.forEach(function (evt, i) { host.appendChild(customEventRow(evt, i)); });
  }

  // ── live events → preview ─────────────────────────────────────────────────
  function onLiveMessage(m) {
    if (!m || typeof m !== "object" || !rules.enabled) return;
    var info = A.typeInfo(m.type);
    if (!info) return;
    var rule = rules.types[m.type];
    if (!rule || !rule.enabled) return;
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

  // ── boot ──────────────────────────────────────────────────────────────────
  if (!v) { notice("Virta bridge unavailable — reload the panel."); return; }

  // Panel live-preview: no sound/TTS (audio output should come from the overlay, not the config panel).
  player = A.createPlayer($("stage"), { gapMs: rules.gapMs, sound: false, tts: false, webhook: false });

  $("add-overlay").addEventListener("click", function () {
    overlays.push({ id: newOverlayId(), name: "Overlay " + (overlays.length + 1), types: [], position: "bottom" });
    save();
    renderOverlays();
  });

  $("master").addEventListener("change", function () { rules.enabled = this.checked; save(); });

  $("gap-ms").addEventListener("change", function () {
    var n = parseInt(this.value, 10);
    rules.gapMs = isFinite(n) && n >= 0 ? Math.min(5000, n) : 500;
    this.value = String(rules.gapMs);
    player.setGap(rules.gapMs);
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
      player.setGap(rules.gapMs);
      if (!hadRules) saveNow();
    })
    .catch(function () { notice("Couldn’t load saved settings — showing defaults; changes may not persist."); })
    .then(function () {
      $("master").checked = !!rules.enabled;
      $("position").value = rules.position;
      setPosition(rules.position);
      $("gap-ms").value = String(rules.gapMs != null ? rules.gapMs : 500);

      // Apply any saved overrides to the live preview.
      A.clearAllOverrides();
      var sOvr = (fullCfg.styleOverrides && typeof fullCfg.styleOverrides === "object") ? fullCfg.styleOverrides : {};
      for (var sid in sOvr) {
        if (Object.prototype.hasOwnProperty.call(sOvr, sid) && sOvr[sid]) {
          try { A.setStyleOverride(sid, sOvr[sid]); } catch (e) { /* CSS errors are non-fatal */ }
        }
      }
      var aOvr = (fullCfg.animOverrides && typeof fullCfg.animOverrides === "object") ? fullCfg.animOverrides : {};
      for (var aid in aOvr) {
        if (Object.prototype.hasOwnProperty.call(aOvr, aid) && aOvr[aid]) {
          try { A.setCustomPreset(aid, aOvr[aid]); } catch (e) { console.warn("Alerts anim override [" + aid + "]:", e); }
        }
      }

      // Ensure webhook secret exists.
      if (!fullCfg.webhookSecret) {
        fullCfg.webhookSecret = generateSecret();
        save();
      }
      if (!fullCfg.customEvents) fullCfg.customEvents = [];

      renderTypeTabs();
      renderTypeEditor();
      renderOverlays();
      renderCustomEvents();

      // Wire up collapsible right-panel sections (Overlays, Custom events).
      function setupRightSec(headEl, bodyEl) {
        function isOpen() { return !bodyEl.hidden; }
        function update() {
          headEl.classList.toggle("open", isOpen());
        }
        headEl.querySelector(".right-sec-toggle").addEventListener("click", function () {
          bodyEl.hidden = !bodyEl.hidden;
          update();
        });
        update();
      }
      setupRightSec($("overlays-head"), $("overlays-body"));
      setupRightSec($("custom-events-head"), $("custom-events-body"));

      // Preview expand/collapse toggle.
      var editorLayout = document.querySelector(".editor-layout");
      $("preview-expand").addEventListener("click", function () {
        var wide = editorLayout.classList.toggle("preview-wide");
        this.textContent = wide ? "⤡" : "⤢";
        this.title = wide ? "Collapse preview" : "Expand preview";
      });

      // Webhook secret display + management.
      var secretEl = $("webhook-secret-val");
      function refreshSecretDisplay() {
        var s = fullCfg.webhookSecret || "";
        secretEl.textContent = s.slice(0, 8) + "…" + s.slice(-4);
        secretEl.title = s;
      }
      refreshSecretDisplay();

      $("webhook-secret-copy").addEventListener("click", function () {
        navigator.clipboard.writeText(fullCfg.webhookSecret || "")
          .then(function () { status("Secret copied"); })
          .catch(function () { status("Copy failed", true); });
      });
      $("webhook-secret-regen").addEventListener("click", function () {
        if (!confirm("Regenerate the webhook secret? All existing webhook URLs will stop working.")) return;
        fullCfg.webhookSecret = generateSecret();
        refreshSecretDisplay();
        save();
      });

      $("add-custom-event").addEventListener("click", function () {
        if (!fullCfg.customEvents) fullCfg.customEvents = [];
        fullCfg.customEvents.push({ name: "Event " + (fullCfg.customEvents.length + 1), badge: "", template: "{author}", animation: "pop", style: "gradient-glass", enabled: true });
        save();
        renderCustomEvents();
      });

      v.on("events.message", onLiveMessage);
      v.send({ type: "events.subscribe", payload: {} }).catch(function () { });
    });

  window.addEventListener("pagehide", function () {
    v.send({ type: "events.unsubscribe" }).catch(function () { });
  });
})();

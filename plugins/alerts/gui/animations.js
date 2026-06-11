/*
 * Alerts — shared animation library + alert renderer.
 *
 * Loaded by BOTH the configurator panel (index.html) and the standalone OBS overlay
 * (overlay.html), so the preview in the panel is pixel-for-pixel the same rendering path the
 * overlay uses on stream.
 *
 * Everything hangs off one global registry, window.VirtaAlertAnimations:
 *
 *   presets   — [{ id, name, enter(el)→Promise, exit(el)→Promise }] animation presets
 *   get(id)   — look a preset up (falls back to a plain fade; also the forced path when the
 *               viewer prefers reduced motion)
 *   styles    — [{ id, name, className }] card style presets (classes live in alert-styles.css)
 *   getStyle(id)
 *
 * To add a new animation: push one object with enter/exit implemented via the Web Animations
 * API onto `presets` (or edit the array below) — both the panel selects and the overlay pick it
 * up automatically. To add a card style: add a `.va-style-<id>` class in alert-styles.css and
 * one entry in `styles`. No other file needs to change.
 *
 * Also shared here so the two pages can never drift apart:
 *   eventTypes      — the six alert event types with labels and magnitude keys
 *   defaultRules()  — the config written on first run
 *   normalizeRules()— sanitize whatever is stored into a complete rules object
 *   parseMagnitudes — best-effort extraction of count/viewers/months/tier from the event's
 *                     body text (the wire carries magnitudes only inside the platform's system
 *                     message text, e.g. "X is raiding with a party of 1,234.")
 *   resolveRule()   — apply threshold tiers (highest matching `min` wins) over the base rule
 *   renderAlert()   — build the card with textContent only, run enter → hold → exit
 *   createPlayer()  — strictly-serial alert queue (cap 20) used by preview and overlay
 */
(function () {
  "use strict";

  // ── small async helpers ──────────────────────────────────────────────────────
  function wait(ms) {
    return new Promise(function (resolve) { setTimeout(resolve, ms); });
  }

  // Run a Web Animation and resolve when it finishes; resolves (never rejects) on cancel or on
  // engines without WAAPI so the alert queue can never wedge.
  function animate(el, keyframes, opts) {
    var anim;
    try {
      anim = el.animate(keyframes, opts);
    } catch (e) {
      return Promise.resolve();
    }
    return anim.finished.catch(function () { /* canceled — fine */ });
  }

  // Start an infinite "hold" loop (neon pulse, rainbow sweep) that runs while the card sits on
  // screen. Loops are tracked on the element and canceled by stopHold() during exit/cleanup.
  function holdLoop(el, keyframes, opts) {
    try {
      var anim = el.animate(keyframes, Object.assign({ iterations: Infinity }, opts));
      if (!el.__vaHold) el.__vaHold = [];
      el.__vaHold.push(anim);
    } catch (e) { /* no WAAPI — the card just sits still */ }
  }

  function stopHold(el) {
    (el.__vaHold || []).forEach(function (a) {
      try { a.cancel(); } catch (e) { /* already gone */ }
    });
    el.__vaHold = [];
  }

  function reducedMotion() {
    return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  }

  // ── confetti particle system ─────────────────────────────────────────────────
  // A canvas burst behind the card. The canvas covers the stage (the card's parent), is removed
  // when the burst ends, and sits earlier in the DOM than the card so the card stays on top.
  var CONFETTI_COLORS = ["#5B8CFF", "#29F0FF", "#F5C84B", "#FF5BA0", "#7CFFB2", "#FF8E5B"];

  function burstConfetti(stage, card) {
    if (!stage || !stage.appendChild) return;
    var canvas = document.createElement("canvas");
    canvas.className = "va-confetti";
    stage.insertBefore(canvas, card || null);
    var ctx = canvas.getContext("2d");
    if (!ctx) { canvas.remove(); return; }

    var dpr = window.devicePixelRatio || 1;
    var w = stage.clientWidth || 640;
    var h = stage.clientHeight || 360;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    ctx.scale(dpr, dpr);

    // Burst origin: the card's center within the stage (fall back to lower middle).
    var ox = w / 2;
    var oy = h * 0.7;
    if (card && card.getBoundingClientRect) {
      var cr = card.getBoundingClientRect();
      var sr = stage.getBoundingClientRect();
      if (cr.width > 0) {
        ox = cr.left - sr.left + cr.width / 2;
        oy = cr.top - sr.top + cr.height / 2;
      }
    }

    var parts = [];
    for (var i = 0; i < 130; i++) {
      var angle = Math.random() * Math.PI * 2;
      var speed = 3 + Math.random() * 11;
      parts.push({
        x: ox,
        y: oy,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed - 7, // bias the burst upward
        size: 4 + Math.random() * 5,
        stretch: 0.4 + Math.random() * 0.8, // rectangles, not squares — reads as paper
        rot: Math.random() * Math.PI * 2,
        vr: (Math.random() - 0.5) * 0.4,
        color: CONFETTI_COLORS[(Math.random() * CONFETTI_COLORS.length) | 0],
        wobble: Math.random() * Math.PI * 2,
      });
    }

    var DURATION = 2400;
    var start = performance.now();
    var raf = 0;

    function frame(now) {
      var t = now - start;
      ctx.clearRect(0, 0, w, h);
      var fade = t > DURATION * 0.7 ? 1 - (t - DURATION * 0.7) / (DURATION * 0.3) : 1;
      ctx.globalAlpha = Math.max(0, fade);
      for (var j = 0; j < parts.length; j++) {
        var p = parts[j];
        p.vy += 0.22; // gravity
        p.vx *= 0.985; // drag
        p.x += p.vx + Math.sin(p.wobble += 0.08);
        p.y += p.vy;
        p.rot += p.vr;
        ctx.save();
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rot);
        ctx.fillStyle = p.color;
        ctx.fillRect(-p.size / 2, (-p.size * p.stretch) / 2, p.size, p.size * p.stretch);
        ctx.restore();
      }
      if (t < DURATION) {
        raf = requestAnimationFrame(frame);
      } else {
        cancelAnimationFrame(raf);
        canvas.remove();
      }
    }
    raf = requestAnimationFrame(frame);
  }

  // ── animation presets ────────────────────────────────────────────────────────
  // Each preset: { id, name, enter(el)→Promise, exit(el)→Promise }. `el` is the .va-card; its
  // parent is the stage (used by confetti). Exits must call stopHold(el) if the preset loops.

  function fadeEnter(el) {
    return animate(el, [{ opacity: 0 }, { opacity: 1 }], { duration: 380, easing: "ease-out", fill: "both" });
  }
  function fadeExit(el) {
    stopHold(el);
    return animate(el, [{ opacity: 1 }, { opacity: 0 }], { duration: 300, easing: "ease-in", fill: "both" });
  }

  var presets = [
    {
      id: "slide-up",
      name: "Slide up",
      enter: function (el) {
        return animate(el, [
          { transform: "translateY(48px)", opacity: 0 },
          { transform: "translateY(-7px)", opacity: 1, offset: 0.72 },
          { transform: "translateY(0)", opacity: 1 },
        ], { duration: 520, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "translateY(0)", opacity: 1 },
          { transform: "translateY(18px)", opacity: 0 },
        ], { duration: 280, easing: "cubic-bezier(0.4, 0, 1, 1)", fill: "both" });
      },
    },
    {
      id: "slide-bounce",
      name: "Slide bounce",
      enter: function (el) {
        return animate(el, [
          { transform: "translateY(-130%)", opacity: 0 },
          { transform: "translateY(0)", opacity: 1, offset: 0.42 },
          { transform: "translateY(-17%)", opacity: 1, offset: 0.6 },
          { transform: "translateY(0)", opacity: 1, offset: 0.76 },
          { transform: "translateY(-6%)", opacity: 1, offset: 0.88 },
          { transform: "translateY(0)", opacity: 1 },
        ], { duration: 840, easing: "cubic-bezier(0.3, 0.6, 0.35, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "translateY(0)", opacity: 1 },
          { transform: "translateY(-40%)", opacity: 0 },
        ], { duration: 300, easing: "cubic-bezier(0.5, 0, 0.75, 0)", fill: "both" });
      },
    },
    {
      id: "pop",
      name: "Pop",
      enter: function (el) {
        return animate(el, [
          { transform: "scale(0.4)", opacity: 0 },
          { transform: "scale(1.12)", opacity: 1, offset: 0.58 },
          { transform: "scale(0.97)", opacity: 1, offset: 0.8 },
          { transform: "scale(1)", opacity: 1 },
        ], { duration: 480, easing: "cubic-bezier(0.34, 1.2, 0.64, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(0.55)", opacity: 0 },
        ], { duration: 240, easing: "cubic-bezier(0.5, 0, 0.75, 0)", fill: "both" });
      },
    },
    {
      id: "fade-zoom",
      name: "Fade zoom",
      enter: function (el) {
        return animate(el, [
          { transform: "scale(1.15)", opacity: 0 },
          { transform: "scale(1)", opacity: 1 },
        ], { duration: 540, easing: "cubic-bezier(0.16, 1, 0.3, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(1.08)", opacity: 0 },
        ], { duration: 340, easing: "ease-in", fill: "both" });
      },
    },
    {
      id: "flip-in",
      name: "Flip in",
      enter: function (el) {
        el.style.transformOrigin = "50% 0%";
        return animate(el, [
          { transform: "perspective(800px) rotateX(-95deg)", opacity: 0 },
          { transform: "perspective(800px) rotateX(13deg)", opacity: 1, offset: 0.68 },
          { transform: "perspective(800px) rotateX(-4deg)", opacity: 1, offset: 0.86 },
          { transform: "perspective(800px) rotateX(0deg)", opacity: 1 },
        ], { duration: 640, easing: "cubic-bezier(0.25, 0.9, 0.35, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "perspective(800px) rotateX(0deg)", opacity: 1 },
          { transform: "perspective(800px) rotateX(72deg)", opacity: 0 },
        ], { duration: 320, easing: "ease-in", fill: "both" });
      },
    },
    {
      id: "glitch",
      name: "Glitch",
      enter: function (el) {
        // step-end easing on every keyframe makes values jump instead of interpolate — the
        // jitter is the point.
        return animate(el, [
          { transform: "translate(-14px, 5px)", clipPath: "inset(55% 0 8% 0)", opacity: 0.6, easing: "step-end" },
          { transform: "translate(11px, -4px)", clipPath: "inset(10% 0 60% 0)", opacity: 1, easing: "step-end", offset: 0.12 },
          { transform: "translate(-8px, -6px)", clipPath: "inset(35% 0 30% 0)", opacity: 0.4, easing: "step-end", offset: 0.24 },
          { transform: "translate(13px, 3px)", clipPath: "inset(0 0 72% 0)", opacity: 1, easing: "step-end", offset: 0.36 },
          { transform: "translate(-10px, 6px)", clipPath: "inset(62% 0 0 0)", opacity: 0.7, easing: "step-end", offset: 0.48 },
          { transform: "translate(6px, -3px)", clipPath: "inset(20% 0 42% 0)", opacity: 1, easing: "step-end", offset: 0.6 },
          { transform: "translate(-4px, 2px)", clipPath: "inset(5% 0 10% 0)", opacity: 0.85, easing: "step-end", offset: 0.72 },
          { transform: "translate(2px, -1px)", clipPath: "inset(0 0 0 0)", opacity: 1, easing: "step-end", offset: 0.84 },
          { transform: "translate(0, 0)", clipPath: "inset(0 0 0 0)", opacity: 1 },
        ], { duration: 560, fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "translate(0, 0)", clipPath: "inset(0 0 0 0)", opacity: 1, easing: "step-end" },
          { transform: "translate(10px, -4px)", clipPath: "inset(30% 0 25% 0)", opacity: 0.7, easing: "step-end", offset: 0.3 },
          { transform: "translate(-12px, 5px)", clipPath: "inset(8% 0 55% 0)", opacity: 0.4, easing: "step-end", offset: 0.6 },
          { transform: "translate(8px, 0)", clipPath: "inset(45% 0 40% 0)", opacity: 0 },
        ], { duration: 300, fill: "both" });
      },
    },
    {
      id: "shake",
      name: "Shake",
      enter: function (el) {
        // Land fast, then rattle — built for hype moments.
        return animate(el, [
          { transform: "scale(0.7)", opacity: 0 },
          { transform: "scale(1)", opacity: 1 },
        ], { duration: 180, easing: "ease-out", fill: "both" }).then(function () {
          return animate(el, [
            { transform: "translate(0, 0) rotate(0deg)" },
            { transform: "translate(-11px, 2px) rotate(-2.4deg)", offset: 0.1 },
            { transform: "translate(10px, -3px) rotate(2deg)", offset: 0.22 },
            { transform: "translate(-9px, 1px) rotate(-1.8deg)", offset: 0.34 },
            { transform: "translate(8px, 2px) rotate(1.6deg)", offset: 0.46 },
            { transform: "translate(-6px, -2px) rotate(-1.2deg)", offset: 0.58 },
            { transform: "translate(5px, 1px) rotate(0.9deg)", offset: 0.7 },
            { transform: "translate(-3px, 0) rotate(-0.5deg)", offset: 0.82 },
            { transform: "translate(0, 0) rotate(0deg)" },
          ], { duration: 620, easing: "linear", fill: "both" });
        });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(0.85) translateY(10px)", opacity: 0 },
        ], { duration: 260, easing: "ease-in", fill: "both" });
      },
    },
    {
      id: "neon-pulse",
      name: "Neon pulse",
      enter: function (el) {
        // Pulse with the card's own border color so the glow matches any style preset.
        var c = "rgba(91, 140, 255, 0.9)";
        try {
          var bc = getComputedStyle(el).borderTopColor;
          if (bc && bc !== "rgba(0, 0, 0, 0)") c = bc;
        } catch (e) { /* keep default */ }
        return animate(el, [
          { transform: "scale(0.92)", opacity: 0 },
          { transform: "scale(1)", opacity: 1 },
        ], { duration: 320, easing: "ease-out", fill: "both" }).then(function () {
          holdLoop(el, [
            { boxShadow: "0 0 10px 0 " + c },
            { boxShadow: "0 0 34px 7px " + c },
            { boxShadow: "0 0 10px 0 " + c },
          ], { duration: 1300, easing: "ease-in-out" });
        });
      },
      exit: function (el) {
        stopHold(el);
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(0.96)", opacity: 0 },
        ], { duration: 300, easing: "ease-in", fill: "both" });
      },
    },
    {
      id: "confetti",
      name: "Confetti",
      enter: function (el) {
        burstConfetti(el.parentElement, el);
        return animate(el, [
          { transform: "scale(0.5) translateY(30px)", opacity: 0 },
          { transform: "scale(1.1) translateY(-4px)", opacity: 1, offset: 0.6 },
          { transform: "scale(1) translateY(0)", opacity: 1 },
        ], { duration: 520, easing: "cubic-bezier(0.34, 1.3, 0.64, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(1.06)", opacity: 0 },
        ], { duration: 320, easing: "ease-in", fill: "both" });
      },
    },
    {
      id: "rainbow-sweep",
      name: "Rainbow sweep",
      enter: function (el) {
        return animate(el, [
          { transform: "translateY(26px)", opacity: 0, filter: "hue-rotate(0deg)" },
          { transform: "translateY(0)", opacity: 1, filter: "hue-rotate(90deg)" },
        ], { duration: 460, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" }).then(function () {
          holdLoop(el, [
            { filter: "hue-rotate(0deg) saturate(1.5)" },
            { filter: "hue-rotate(360deg) saturate(1.5)" },
          ], { duration: 2200, easing: "linear" });
        });
      },
      exit: function (el) {
        stopHold(el);
        return animate(el, [
          { transform: "translateY(0)", opacity: 1 },
          { transform: "translateY(14px)", opacity: 0 },
        ], { duration: 300, easing: "ease-in", fill: "both" });
      },
    },
  ];

  var FALLBACK_FADE = { id: "fade", name: "Fade", enter: fadeEnter, exit: fadeExit };

  function get(id) {
    // Reduced motion wins over everything: every preset degrades to a plain fade.
    if (reducedMotion()) return FALLBACK_FADE;
    for (var i = 0; i < presets.length; i++) {
      if (presets[i].id === id) return presets[i];
    }
    return presets[0] || FALLBACK_FADE;
  }

  // ── card style presets (classes implemented in alert-styles.css) ─────────────
  var styles = [
    { id: "minimal-dark", name: "Minimal dark", className: "va-style-minimal-dark" },
    { id: "neon-outline", name: "Neon outline", className: "va-style-neon-outline" },
    { id: "gradient-glass", name: "Gradient glass", className: "va-style-gradient-glass" },
    { id: "gold-epic", name: "Gold epic", className: "va-style-gold-epic" },
    { id: "retro-arcade", name: "Retro arcade", className: "va-style-retro-arcade" },
    { id: "clean-light", name: "Clean light", className: "va-style-clean-light" },
  ];

  function getStyle(id) {
    for (var i = 0; i < styles.length; i++) {
      if (styles[i].id === id) return styles[i];
    }
    return styles[0];
  }

  // ── alert event model ────────────────────────────────────────────────────────
  // magnitude: which alertData field threshold tiers compare against (null = no tiers).
  var eventTypes = [
    { id: "sub", label: "Sub", badge: "NEW SUB", magnitude: null },
    { id: "resub", label: "Resub", badge: "RESUB", magnitude: "months" },
    { id: "giftsub", label: "Gift subs", badge: "GIFT", magnitude: "count" },
    { id: "raid", label: "Raid", badge: "RAID", magnitude: "viewers" },
    { id: "host", label: "Host", badge: "HOST", magnitude: "viewers" },
    { id: "follow", label: "Follow", badge: "FOLLOW", magnitude: null },
  ];

  function typeInfo(id) {
    for (var i = 0; i < eventTypes.length; i++) {
      if (eventTypes[i].id === id) return eventTypes[i];
    }
    return null;
  }

  var DEFAULT_DURATION_MS = 6000;

  function defaultRules() {
    return {
      version: 1,
      enabled: true,
      position: "bottom",
      types: {
        sub: {
          enabled: true, animation: "slide-bounce", style: "gradient-glass",
          durationMs: DEFAULT_DURATION_MS, template: "{author} just subscribed!", tiers: [],
        },
        resub: {
          enabled: true, animation: "fade-zoom", style: "gradient-glass",
          durationMs: DEFAULT_DURATION_MS, template: "{author} resubscribed for {months} months!",
          tiers: [{ min: 12, animation: "neon-pulse", style: "neon-outline" }],
        },
        giftsub: {
          enabled: true, animation: "pop", style: "neon-outline",
          durationMs: DEFAULT_DURATION_MS, template: "{author} gifted {count} subs!",
          tiers: [
            { min: 5, animation: "shake" },
            { min: 20, animation: "confetti", style: "gold-epic" },
          ],
        },
        raid: {
          enabled: true, animation: "slide-up", style: "minimal-dark",
          durationMs: DEFAULT_DURATION_MS, template: "{author} is raiding with {viewers} viewers!",
          tiers: [
            { min: 50, animation: "pop", style: "neon-outline" },
            { min: 200, animation: "confetti", style: "gold-epic" },
          ],
        },
        host: {
          enabled: true, animation: "slide-up", style: "minimal-dark",
          durationMs: DEFAULT_DURATION_MS, template: "{author} is hosting with {viewers} viewers!", tiers: [],
        },
        follow: {
          enabled: true, animation: "slide-up", style: "minimal-dark",
          durationMs: DEFAULT_DURATION_MS, template: "{author} just followed!", tiers: [],
        },
      },
    };
  }

  function clampDuration(ms, fallback) {
    var n = Number(ms);
    if (!isFinite(n) || n <= 0) return fallback;
    return Math.min(60000, Math.max(1000, Math.round(n)));
  }

  // Sanitize a stored rules object into a complete one: unknown fields dropped, missing fields
  // filled from defaults, tiers cleaned and sorted ascending by min.
  function normalizeRules(raw) {
    var def = defaultRules();
    if (!raw || typeof raw !== "object") return def;
    var out = {
      version: 1,
      enabled: typeof raw.enabled === "boolean" ? raw.enabled : def.enabled,
      position: raw.position === "top" || raw.position === "center" || raw.position === "bottom" ? raw.position : def.position,
      types: {},
    };
    eventTypes.forEach(function (t) {
      var d = def.types[t.id];
      var r = raw.types && raw.types[t.id];
      if (!r || typeof r !== "object") {
        out.types[t.id] = d;
        return;
      }
      var tiers = [];
      if (t.magnitude && Array.isArray(r.tiers)) {
        r.tiers.forEach(function (tier) {
          if (!tier || typeof tier !== "object") return;
          var min = Number(tier.min);
          if (!isFinite(min) || min < 0) return;
          var clean = { min: Math.round(min) };
          if (typeof tier.animation === "string" && tier.animation) clean.animation = tier.animation;
          if (typeof tier.style === "string" && tier.style) clean.style = tier.style;
          if (typeof tier.template === "string" && tier.template) clean.template = tier.template;
          var dur = Number(tier.durationMs);
          if (isFinite(dur) && dur > 0) clean.durationMs = clampDuration(dur, d.durationMs);
          tiers.push(clean);
        });
        tiers.sort(function (a, b) { return a.min - b.min; });
      }
      out.types[t.id] = {
        enabled: typeof r.enabled === "boolean" ? r.enabled : d.enabled,
        animation: typeof r.animation === "string" && r.animation ? r.animation : d.animation,
        style: typeof r.style === "string" && r.style ? r.style : d.style,
        durationMs: clampDuration(r.durationMs, d.durationMs),
        template: typeof r.template === "string" && r.template ? r.template : d.template,
        tiers: tiers,
      };
    });
    return out;
  }

  // ── magnitude extraction ─────────────────────────────────────────────────────
  // The wire's UnifiedMessage has no structured magnitude fields; counts ride inside the
  // platform's system-message text. Parse them out best-effort; every value is optional and
  // templates fall back to sane defaults (count 1, viewers/months 0).
  function parseNum(s) {
    var n = parseInt(String(s).replace(/,/g, ""), 10);
    return isFinite(n) ? n : null;
  }

  function parseMagnitudes(type, text) {
    var body = String(text || "");
    var out = {};
    var m;
    if (type === "giftsub") {
      // "is gifting 5 Tier 1 Subs", "gifted 20 subs", "gifted a Tier 1 sub to X" (count 1)
      m = body.match(/gift\w*\s+(?:a\s+)?([\d,]+)/i);
      out.count = m ? parseNum(m[1]) : null;
      if (out.count === null) out.count = 1;
    } else if (type === "raid" || type === "host") {
      // "is raiding with a party of 1,234" / "is hosting with 56 viewers"
      m = body.match(/(?:party of|with)\s+([\d,]+)/i) || body.match(/([\d,]+)/);
      out.viewers = m ? parseNum(m[1]) : null;
      if (out.viewers === null) out.viewers = 0;
    } else if (type === "resub") {
      // "They've subscribed for 12 months!"
      m = body.match(/([\d,]+)\s+months?/i);
      out.months = m ? parseNum(m[1]) : null;
      if (out.months === null) out.months = 0;
    }
    m = body.match(/tier\s+([0-9]+)/i);
    if (m) out.tier = m[1];
    else if (/prime/i.test(body)) out.tier = "Prime";
    return out;
  }

  // The number a type's threshold tiers compare against, or null when the type has no tiers.
  function magnitudeOf(type, data) {
    var info = typeInfo(type);
    if (!info || !info.magnitude) return null;
    var n = Number(data && data[info.magnitude]);
    return isFinite(n) ? n : null;
  }

  // Apply threshold tiers over the base rule: the highest tier whose min <= magnitude wins, and
  // only the fields it defines override the base.
  function resolveRule(rule, magnitude) {
    rule = rule || {};
    var eff = {
      animation: rule.animation || "slide-up",
      style: rule.style || "minimal-dark",
      durationMs: clampDuration(rule.durationMs, DEFAULT_DURATION_MS),
      template: rule.template || "{author}",
    };
    if (typeof magnitude === "number" && Array.isArray(rule.tiers)) {
      var best = null;
      rule.tiers.forEach(function (t) {
        if (!t || typeof t.min !== "number" || magnitude < t.min) return;
        if (!best || t.min > best.min) best = t;
      });
      if (best) {
        if (best.animation) eff.animation = best.animation;
        if (best.style) eff.style = best.style;
        if (best.template) eff.template = best.template;
        if (best.durationMs) eff.durationMs = clampDuration(best.durationMs, eff.durationMs);
      }
    }
    return eff;
  }

  // ── card building & playback ─────────────────────────────────────────────────
  var PLACEHOLDER_RE = /\{(author|count|viewers|months|tier)\}/g;

  function placeholderValue(key, data) {
    switch (key) {
      case "author": return data.author || "Someone";
      case "count": return String(data.count != null ? data.count : 1);
      case "viewers": return String(data.viewers != null ? data.viewers : 0);
      case "months": return String(data.months != null ? data.months : 0);
      case "tier": return data.tier != null ? String(data.tier) : "";
      default: return "";
    }
  }

  // Substitute template placeholders into DOM nodes — textContent only, never markup, so stream
  // data can't inject anything.
  function applyTemplate(template, data) {
    var frag = document.createDocumentFragment();
    var tpl = String(template || "");
    var last = 0;
    var m;
    PLACEHOLDER_RE.lastIndex = 0;
    while ((m = PLACEHOLDER_RE.exec(tpl)) !== null) {
      if (m.index > last) frag.appendChild(document.createTextNode(tpl.slice(last, m.index)));
      var span = document.createElement("span");
      span.className = "va-tpl va-tpl-" + m[1];
      span.textContent = placeholderValue(m[1], data);
      frag.appendChild(span);
      last = m.index + m[0].length;
    }
    if (last < tpl.length) frag.appendChild(document.createTextNode(tpl.slice(last)));
    return frag;
  }

  // Build the alert card, play enter → hold → exit inside `container`, then remove it.
  // alertData: { type, author, count?, viewers?, months?, tier? }. rule: the per-type rule
  // (tiers included — resolution happens here so panel preview and overlay can't diverge).
  function renderAlert(container, alertData, rule) {
    var data = alertData || {};
    var eff = resolveRule(rule, magnitudeOf(data.type, data));
    var style = getStyle(eff.style);
    var info = typeInfo(data.type);

    var card = document.createElement("div");
    card.className = "va-card va-type-" + (data.type || "sub") + " " + style.className;
    var badge = document.createElement("span");
    badge.className = "va-badge";
    badge.textContent = info ? info.badge : "ALERT";
    card.appendChild(badge);
    var text = document.createElement("div");
    text.className = "va-text";
    text.appendChild(applyTemplate(eff.template, data));
    card.appendChild(text);
    container.appendChild(card);

    var preset = get(eff.animation);
    return Promise.resolve()
      .then(function () { return preset.enter(card); })
      .then(function () { return wait(eff.durationMs); })
      .then(function () { return preset.exit(card); })
      .catch(function () { /* a broken preset must not wedge the queue */ })
      .then(function () {
        stopHold(card);
        if (card.parentNode) card.parentNode.removeChild(card);
      });
  }

  // Strictly-serial alert player: one alert at a time, 500ms gap, queue capped at 20 (oldest
  // dropped — it's a live overlay, stale hype helps no one).
  var QUEUE_CAP = 20;
  var GAP_MS = 500;

  function createPlayer(container) {
    var queue = [];
    var playing = false;

    function pump() {
      if (playing) return;
      var next = queue.shift();
      if (!next) return;
      playing = true;
      renderAlert(container, next.data, next.rule)
        .then(function () { return wait(GAP_MS); })
        .then(function () {
          playing = false;
          pump();
        });
    }

    return {
      enqueue: function (data, rule) {
        if (queue.length >= QUEUE_CAP) queue.shift();
        queue.push({ data: data, rule: rule });
        pump();
      },
      clear: function () { queue.length = 0; },
    };
  }

  // ── overlay instances ────────────────────────────────────────────────────────
  // One overlay instance = one OBS browser source: its own URL (?overlay=<id>), an event-type
  // filter (empty = all types), and a position. Stored in config as a sibling of rules so the
  // panel and every overlay page agree.
  function defaultOverlays(rules) {
    return [{ id: "main", name: "Main", types: [], position: (rules && rules.position) || "bottom" }];
  }

  function normalizeOverlays(raw, rules) {
    if (!Array.isArray(raw)) return defaultOverlays(rules);
    var seen = {};
    var out = [];
    raw.forEach(function (o) {
      if (!o || typeof o !== "object") return;
      var id = typeof o.id === "string" && /^[a-z0-9][a-z0-9-]{0,31}$/.test(o.id) ? o.id : null;
      if (!id || seen[id]) return;
      seen[id] = true;
      var types = [];
      if (Array.isArray(o.types)) {
        o.types.forEach(function (t) {
          if (typeInfo(t) && types.indexOf(t) === -1) types.push(t);
        });
      }
      out.push({
        id: id,
        name: typeof o.name === "string" && o.name.trim() ? o.name.trim().slice(0, 40) : "Overlay",
        types: types,
        position: o.position === "top" || o.position === "center" || o.position === "bottom" ? o.position : "bottom",
      });
    });
    return out.length ? out : defaultOverlays(rules);
  }

  // ── public registry ──────────────────────────────────────────────────────────
  window.VirtaAlertAnimations = {
    presets: presets,
    get: get,
    styles: styles,
    getStyle: getStyle,
    eventTypes: eventTypes,
    typeInfo: typeInfo,
    defaultRules: defaultRules,
    normalizeRules: normalizeRules,
    defaultOverlays: defaultOverlays,
    normalizeOverlays: normalizeOverlays,
    parseMagnitudes: parseMagnitudes,
    magnitudeOf: magnitudeOf,
    resolveRule: resolveRule,
    applyTemplate: applyTemplate,
    renderAlert: renderAlert,
    createPlayer: createPlayer,
    wait: wait,
  };
})();

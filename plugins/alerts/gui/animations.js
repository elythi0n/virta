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

  // Template strings are shown in the code editor when a user picks an animation. They use raw
  // WAAPI (el.animate) so the code is self-contained and doesn't depend on internal helpers.
  // holdLoop / stopHold / burstConfetti are injected into the execution scope by compileAnim().
  var ANIM_HEADER = "// el is the .va-card element  |  available: holdLoop, stopHold, burstConfetti\n\n";

  var presets = [
    {
      id: "slide-up",
      name: "Slide up",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(48px)', opacity: 0 },",
        "    { transform: 'translateY(-7px)', opacity: 1, offset: 0.72 },",
        "    { transform: 'translateY(0)', opacity: 1 }",
        "  ], { duration: 520, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(0)', opacity: 1 },",
        "    { transform: 'translateY(18px)', opacity: 0 }",
        "  ], { duration: 280, easing: 'cubic-bezier(0.4, 0, 1, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(-130%)', opacity: 0 },",
        "    { transform: 'translateY(0)', opacity: 1, offset: 0.42 },",
        "    { transform: 'translateY(-17%)', opacity: 1, offset: 0.6 },",
        "    { transform: 'translateY(0)', opacity: 1, offset: 0.76 },",
        "    { transform: 'translateY(-6%)', opacity: 1, offset: 0.88 },",
        "    { transform: 'translateY(0)', opacity: 1 }",
        "  ], { duration: 840, easing: 'cubic-bezier(0.3, 0.6, 0.35, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(0)', opacity: 1 },",
        "    { transform: 'translateY(-40%)', opacity: 0 }",
        "  ], { duration: 300, easing: 'cubic-bezier(0.5, 0, 0.75, 0)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scale(0.4)', opacity: 0 },",
        "    { transform: 'scale(1.12)', opacity: 1, offset: 0.58 },",
        "    { transform: 'scale(0.97)', opacity: 1, offset: 0.8 },",
        "    { transform: 'scale(1)', opacity: 1 }",
        "  ], { duration: 480, easing: 'cubic-bezier(0.34, 1.2, 0.64, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(0.55)', opacity: 0 }",
        "  ], { duration: 240, easing: 'cubic-bezier(0.5, 0, 0.75, 0)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1.15)', opacity: 0 },",
        "    { transform: 'scale(1)', opacity: 1 }",
        "  ], { duration: 540, easing: 'cubic-bezier(0.16, 1, 0.3, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(1.08)', opacity: 0 }",
        "  ], { duration: 340, easing: 'ease-in', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  el.style.transformOrigin = '50% 0%';",
        "  return el.animate([",
        "    { transform: 'perspective(800px) rotateX(-95deg)', opacity: 0 },",
        "    { transform: 'perspective(800px) rotateX(13deg)', opacity: 1, offset: 0.68 },",
        "    { transform: 'perspective(800px) rotateX(-4deg)', opacity: 1, offset: 0.86 },",
        "    { transform: 'perspective(800px) rotateX(0deg)', opacity: 1 }",
        "  ], { duration: 640, easing: 'cubic-bezier(0.25, 0.9, 0.35, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'perspective(800px) rotateX(0deg)', opacity: 1 },",
        "    { transform: 'perspective(800px) rotateX(72deg)', opacity: 0 }",
        "  ], { duration: 320, easing: 'ease-in', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translate(-14px, 5px)', clipPath: 'inset(55% 0 8% 0)', opacity: 0.6, easing: 'step-end' },",
        "    { transform: 'translate(11px, -4px)', clipPath: 'inset(10% 0 60% 0)', opacity: 1, easing: 'step-end', offset: 0.12 },",
        "    { transform: 'translate(-8px, -6px)', clipPath: 'inset(35% 0 30% 0)', opacity: 0.4, easing: 'step-end', offset: 0.24 },",
        "    { transform: 'translate(13px, 3px)', clipPath: 'inset(0 0 72% 0)', opacity: 1, easing: 'step-end', offset: 0.36 },",
        "    { transform: 'translate(-10px, 6px)', clipPath: 'inset(62% 0 0 0)', opacity: 0.7, easing: 'step-end', offset: 0.48 },",
        "    { transform: 'translate(6px, -3px)', clipPath: 'inset(20% 0 42% 0)', opacity: 1, easing: 'step-end', offset: 0.6 },",
        "    { transform: 'translate(-4px, 2px)', clipPath: 'inset(5% 0 10% 0)', opacity: 0.85, easing: 'step-end', offset: 0.72 },",
        "    { transform: 'translate(2px, -1px)', clipPath: 'inset(0 0 0 0)', opacity: 1, easing: 'step-end', offset: 0.84 },",
        "    { transform: 'translate(0, 0)', clipPath: 'inset(0 0 0 0)', opacity: 1 }",
        "  ], { duration: 560, fill: 'both' }).finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'translate(0, 0)', clipPath: 'inset(0 0 0 0)', opacity: 1, easing: 'step-end' },",
        "    { transform: 'translate(10px, -4px)', clipPath: 'inset(30% 0 25% 0)', opacity: 0.7, easing: 'step-end', offset: 0.3 },",
        "    { transform: 'translate(-12px, 5px)', clipPath: 'inset(8% 0 55% 0)', opacity: 0.4, easing: 'step-end', offset: 0.6 },",
        "    { transform: 'translate(8px, 0)', clipPath: 'inset(45% 0 40% 0)', opacity: 0 }",
        "  ], { duration: 300, fill: 'both' }).finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scale(0.7)', opacity: 0 },",
        "    { transform: 'scale(1)', opacity: 1 }",
        "  ], { duration: 180, easing: 'ease-out', fill: 'both' }).finished.catch(function() {}).then(function() {",
        "    return el.animate([",
        "      { transform: 'translate(0, 0) rotate(0deg)' },",
        "      { transform: 'translate(-11px, 2px) rotate(-2.4deg)', offset: 0.1 },",
        "      { transform: 'translate(10px, -3px) rotate(2deg)', offset: 0.22 },",
        "      { transform: 'translate(-9px, 1px) rotate(-1.8deg)', offset: 0.34 },",
        "      { transform: 'translate(8px, 2px) rotate(1.6deg)', offset: 0.46 },",
        "      { transform: 'translate(-6px, -2px) rotate(-1.2deg)', offset: 0.58 },",
        "      { transform: 'translate(5px, 1px) rotate(0.9deg)', offset: 0.7 },",
        "      { transform: 'translate(-3px, 0) rotate(-0.5deg)', offset: 0.82 },",
        "      { transform: 'translate(0, 0) rotate(0deg)' }",
        "    ], { duration: 620, easing: 'linear', fill: 'both' }).finished.catch(function() {});",
        "  });",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(0.85) translateY(10px)', opacity: 0 }",
        "  ], { duration: 260, easing: 'ease-in', fill: 'both' }).finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  var c = 'rgba(91, 140, 255, 0.9)';",
        "  try {",
        "    var bc = getComputedStyle(el).borderTopColor;",
        "    if (bc && bc !== 'rgba(0, 0, 0, 0)') c = bc;",
        "  } catch(e) {}",
        "  return el.animate([",
        "    { transform: 'scale(0.92)', opacity: 0 },",
        "    { transform: 'scale(1)', opacity: 1 }",
        "  ], { duration: 320, easing: 'ease-out', fill: 'both' }).finished.catch(function() {}).then(function() {",
        "    holdLoop(el, [",
        "      { boxShadow: '0 0 10px 0 ' + c },",
        "      { boxShadow: '0 0 34px 7px ' + c },",
        "      { boxShadow: '0 0 10px 0 ' + c }",
        "    ], { duration: 1300, easing: 'ease-in-out' });",
        "  });",
        "}",
        "",
        "function exit(el) {",
        "  stopHold(el);",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(0.96)', opacity: 0 }",
        "  ], { duration: 300, easing: 'ease-in', fill: 'both' }).finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  burstConfetti(el.parentElement, el);",
        "  return el.animate([",
        "    { transform: 'scale(0.5) translateY(30px)', opacity: 0 },",
        "    { transform: 'scale(1.1) translateY(-4px)', opacity: 1, offset: 0.6 },",
        "    { transform: 'scale(1) translateY(0)', opacity: 1 }",
        "  ], { duration: 520, easing: 'cubic-bezier(0.34, 1.3, 0.64, 1)', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(1.06)', opacity: 0 }",
        "  ], { duration: 320, easing: 'ease-in', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(26px)', opacity: 0, filter: 'hue-rotate(0deg)' },",
        "    { transform: 'translateY(0)', opacity: 1, filter: 'hue-rotate(90deg)' }",
        "  ], { duration: 460, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', fill: 'both' })",
        "    .finished.catch(function() {}).then(function() {",
        "      holdLoop(el, [",
        "        { filter: 'hue-rotate(0deg) saturate(1.5)' },",
        "        { filter: 'hue-rotate(360deg) saturate(1.5)' }",
        "      ], { duration: 2200, easing: 'linear' });",
        "    });",
        "}",
        "",
        "function exit(el) {",
        "  stopHold(el);",
        "  return el.animate([",
        "    { transform: 'translateY(0)', opacity: 1 },",
        "    { transform: 'translateY(14px)', opacity: 0 }",
        "  ], { duration: 300, easing: 'ease-in', fill: 'both' })",
        "    .finished.catch(function() {});",
        "}",
      ].join("\n"),
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
    // ── elastic: overshoot spring entry ──────────────────────────────────────
    {
      id: "elastic",
      name: "Elastic",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scale(0.4) translateY(30px)', opacity: 0 },",
        "    { transform: 'scale(1.12) translateY(-8px)', opacity: 1, offset: 0.65 },",
        "    { transform: 'scale(0.96) translateY(3px)', offset: 0.82 },",
        "    { transform: 'scale(1) translateY(0)', opacity: 1 }",
        "  ], { duration: 550, easing: 'ease-out', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(0.85)', opacity: 0 }",
        "  ], { duration: 260, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "scale(0.4) translateY(30px)", opacity: 0 },
          { transform: "scale(1.12) translateY(-8px)", opacity: 1, offset: 0.65 },
          { transform: "scale(0.96) translateY(3px)", offset: 0.82 },
          { transform: "scale(1) translateY(0)", opacity: 1 },
        ], { duration: 550, easing: "ease-out", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(0.85)", opacity: 0 },
        ], { duration: 260, easing: "ease-in", fill: "both" });
      },
    },
    // ── slide-left: enters from the right ────────────────────────────────────
    {
      id: "slide-left",
      name: "Slide left",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateX(80px)', opacity: 0 },",
        "    { transform: 'translateX(0)', opacity: 1 }",
        "  ], { duration: 400, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'translateX(0)', opacity: 1 },",
        "    { transform: 'translateX(-50px)', opacity: 0 }",
        "  ], { duration: 300, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "translateX(80px)", opacity: 0 },
          { transform: "translateX(0)", opacity: 1 },
        ], { duration: 400, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "translateX(0)", opacity: 1 },
          { transform: "translateX(-50px)", opacity: 0 },
        ], { duration: 300, easing: "ease-in", fill: "both" });
      },
    },
    // ── wipe: horizontal reveal/conceal ──────────────────────────────────────
    {
      id: "wipe",
      name: "Wipe",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { clipPath: 'inset(0 100% 0 0)', opacity: 1 },",
        "    { clipPath: 'inset(0 0% 0 0)', opacity: 1 }",
        "  ], { duration: 480, easing: 'cubic-bezier(0.4, 0, 0.2, 1)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { clipPath: 'inset(0 0% 0 0)', opacity: 1 },",
        "    { clipPath: 'inset(0 0% 0 100%)', opacity: 1 }",
        "  ], { duration: 320, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { clipPath: "inset(0 100% 0 0)", opacity: 1 },
          { clipPath: "inset(0 0% 0 0)", opacity: 1 },
        ], { duration: 480, easing: "cubic-bezier(0.4, 0, 0.2, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { clipPath: "inset(0 0% 0 0)", opacity: 1 },
          { clipPath: "inset(0 0% 0 100%)", opacity: 1 },
        ], { duration: 320, easing: "ease-in", fill: "both" });
      },
    },
    // ── blur-in: gaussian bloom entrance ─────────────────────────────────────
    {
      id: "blur-in",
      name: "Blur in",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { filter: 'blur(18px)', opacity: 0, transform: 'scale(1.06)' },",
        "    { filter: 'blur(0)', opacity: 1, transform: 'scale(1)' }",
        "  ], { duration: 500, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { filter: 'blur(0)', opacity: 1 },",
        "    { filter: 'blur(12px)', opacity: 0 }",
        "  ], { duration: 280, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { filter: "blur(18px)", opacity: 0, transform: "scale(1.06)" },
          { filter: "blur(0)", opacity: 1, transform: "scale(1)" },
        ], { duration: 500, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { filter: "blur(0)", opacity: 1 },
          { filter: "blur(12px)", opacity: 0 },
        ], { duration: 280, easing: "ease-in", fill: "both" });
      },
    },
    // ── swing: pendulum drop from top ─────────────────────────────────────────
    {
      id: "swing",
      name: "Swing",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  el.style.transformOrigin = 'top center';",
        "  return el.animate([",
        "    { transform: 'rotateX(-90deg)', opacity: 0 },",
        "    { transform: 'rotateX(18deg)', opacity: 1, offset: 0.6 },",
        "    { transform: 'rotateX(-8deg)', offset: 0.78 },",
        "    { transform: 'rotateX(0deg)', opacity: 1 }",
        "  ], { duration: 560, easing: 'ease-out', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  el.style.transformOrigin = 'top center';",
        "  return el.animate([",
        "    { transform: 'rotateX(0deg)', opacity: 1 },",
        "    { transform: 'rotateX(90deg)', opacity: 0 }",
        "  ], { duration: 280, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        el.style.transformOrigin = "top center";
        return animate(el, [
          { transform: "rotateX(-90deg)", opacity: 0 },
          { transform: "rotateX(18deg)", opacity: 1, offset: 0.6 },
          { transform: "rotateX(-8deg)", offset: 0.78 },
          { transform: "rotateX(0deg)", opacity: 1 },
        ], { duration: 560, easing: "ease-out", fill: "both" });
      },
      exit: function (el) {
        el.style.transformOrigin = "top center";
        return animate(el, [
          { transform: "rotateX(0deg)", opacity: 1 },
          { transform: "rotateX(90deg)", opacity: 0 },
        ], { duration: 280, easing: "ease-in", fill: "both" });
      },
    },
    // ── rise-glow: float up while radiating light ─────────────────────────────
    {
      id: "rise-glow",
      name: "Rise glow",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(40px)', opacity: 0, filter: 'brightness(2.5) blur(6px)' },",
        "    { transform: 'translateY(0)', opacity: 1, filter: 'brightness(1) blur(0)' }",
        "  ], { duration: 500, easing: 'cubic-bezier(0.16, 1, 0.3, 1)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'translateY(0)', opacity: 1, filter: 'brightness(1)' },",
        "    { transform: 'translateY(-20px)', opacity: 0, filter: 'brightness(2)' }",
        "  ], { duration: 320, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "translateY(40px)", opacity: 0, filter: "brightness(2.5) blur(6px)" },
          { transform: "translateY(0)", opacity: 1, filter: "brightness(1) blur(0)" },
        ], { duration: 500, easing: "cubic-bezier(0.16, 1, 0.3, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "translateY(0)", opacity: 1, filter: "brightness(1)" },
          { transform: "translateY(-20px)", opacity: 0, filter: "brightness(2)" },
        ], { duration: 320, easing: "ease-in", fill: "both" });
      },
    },
    // ── typewriter: scale in letter by letter via clip ────────────────────────
    {
      id: "typewriter",
      name: "Typewriter",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  el.style.overflow = 'hidden';",
        "  el.style.whiteSpace = 'nowrap';",
        "  return el.animate([",
        "    { width: '0%', opacity: 1 },",
        "    { width: '100%', opacity: 1 }",
        "  ], { duration: 420, easing: 'steps(28, end)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { opacity: 1 },",
        "    { opacity: 0 }",
        "  ], { duration: 200, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        el.style.overflow = "hidden";
        el.style.whiteSpace = "nowrap";
        return animate(el, [
          { width: "0%", opacity: 1 },
          { width: "100%", opacity: 1 },
        ], { duration: 420, easing: "steps(28, end)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { opacity: 1 },
          { opacity: 0 },
        ], { duration: 200, easing: "ease-in", fill: "both" });
      },
    },
    // ── spin-zoom: rotate in from nothing ─────────────────────────────────────
    {
      id: "spin-zoom",
      name: "Spin zoom",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'rotate(-180deg) scale(0)', opacity: 0 },",
        "    { transform: 'rotate(8deg) scale(1.05)', opacity: 1, offset: 0.8 },",
        "    { transform: 'rotate(0deg) scale(1)', opacity: 1 }",
        "  ], { duration: 520, easing: 'cubic-bezier(0.34, 1.56, 0.64, 1)', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'rotate(0deg) scale(1)', opacity: 1 },",
        "    { transform: 'rotate(90deg) scale(0)', opacity: 0 }",
        "  ], { duration: 280, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "rotate(-180deg) scale(0)", opacity: 0 },
          { transform: "rotate(8deg) scale(1.05)", opacity: 1, offset: 0.8 },
          { transform: "rotate(0deg) scale(1)", opacity: 1 },
        ], { duration: 520, easing: "cubic-bezier(0.34, 1.56, 0.64, 1)", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "rotate(0deg) scale(1)", opacity: 1 },
          { transform: "rotate(90deg) scale(0)", opacity: 0 },
        ], { duration: 280, easing: "ease-in", fill: "both" });
      },
    },
    // ── drift: gentle float in from the left ─────────────────────────────────
    {
      id: "drift",
      name: "Drift",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'translateX(-40px) translateY(10px)', opacity: 0 },",
        "    { transform: 'translateX(0) translateY(0)', opacity: 1 }",
        "  ], { duration: 600, easing: 'cubic-bezier(0.25, 1, 0.5, 1)', fill: 'both' })",
        "    .finished.catch(function() {}).then(function() {",
        "      holdLoop(el, [",
        "        { transform: 'translateY(0px)' },",
        "        { transform: 'translateY(-5px)' },",
        "        { transform: 'translateY(0px)' }",
        "      ], { duration: 3000, easing: 'ease-in-out' });",
        "    });",
        "}",
        "",
        "function exit(el) {",
        "  stopHold(el);",
        "  return el.animate([",
        "    { transform: 'translateY(0)', opacity: 1 },",
        "    { transform: 'translateY(-12px)', opacity: 0 }",
        "  ], { duration: 300, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "translateX(-40px) translateY(10px)", opacity: 0 },
          { transform: "translateX(0) translateY(0)", opacity: 1 },
        ], { duration: 600, easing: "cubic-bezier(0.25, 1, 0.5, 1)", fill: "both" }).then(function () {
          holdLoop(el, [
            { transform: "translateY(0px)" },
            { transform: "translateY(-5px)" },
            { transform: "translateY(0px)" },
          ], { duration: 3000, easing: "ease-in-out" });
        });
      },
      exit: function (el) {
        stopHold(el);
        return animate(el, [
          { transform: "translateY(0)", opacity: 1 },
          { transform: "translateY(-12px)", opacity: 0 },
        ], { duration: 300, easing: "ease-in", fill: "both" });
      },
    },
    // ── heartbeat: double-pulse thump ────────────────────────────────────────
    {
      id: "heartbeat",
      name: "Heartbeat",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scale(0)', opacity: 0 },",
        "    { transform: 'scale(1.25)', opacity: 1, offset: 0.35 },",
        "    { transform: 'scale(0.95)', offset: 0.5 },",
        "    { transform: 'scale(1.1)', offset: 0.65 },",
        "    { transform: 'scale(1)', opacity: 1 }",
        "  ], { duration: 580, easing: 'ease-out', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scale(1.05)', opacity: 0.5, offset: 0.3 },",
        "    { transform: 'scale(0)', opacity: 0 }",
        "  ], { duration: 300, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "scale(0)", opacity: 0 },
          { transform: "scale(1.25)", opacity: 1, offset: 0.35 },
          { transform: "scale(0.95)", offset: 0.5 },
          { transform: "scale(1.1)", offset: 0.65 },
          { transform: "scale(1)", opacity: 1 },
        ], { duration: 580, easing: "ease-out", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(1.05)", opacity: 0.5, offset: 0.3 },
          { transform: "scale(0)", opacity: 0 },
        ], { duration: 300, easing: "ease-in", fill: "both" });
      },
    },
    // ── morph: squash → stretch → settle ─────────────────────────────────────
    {
      id: "morph",
      name: "Morph",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  return el.animate([",
        "    { transform: 'scaleX(0.2) scaleY(2.4)', opacity: 0, transformOrigin: 'bottom center' },",
        "    { transform: 'scaleX(1.2) scaleY(0.8)', opacity: 1, offset: 0.5, transformOrigin: 'bottom center' },",
        "    { transform: 'scaleX(0.95) scaleY(1.05)', offset: 0.75 },",
        "    { transform: 'scaleX(1) scaleY(1)', opacity: 1 }",
        "  ], { duration: 520, easing: 'ease-out', fill: 'both' }).finished.catch(function(){});",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { transform: 'scale(1)', opacity: 1 },",
        "    { transform: 'scaleX(1.1) scaleY(0.6)', opacity: 0.4, offset: 0.4 },",
        "    { transform: 'scaleX(0) scaleY(0)', opacity: 0 }",
        "  ], { duration: 280, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        return animate(el, [
          { transform: "scaleX(0.2) scaleY(2.4)", opacity: 0, transformOrigin: "bottom center" },
          { transform: "scaleX(1.2) scaleY(0.8)", opacity: 1, offset: 0.5, transformOrigin: "bottom center" },
          { transform: "scaleX(0.95) scaleY(1.05)", offset: 0.75 },
          { transform: "scaleX(1) scaleY(1)", opacity: 1 },
        ], { duration: 520, easing: "ease-out", fill: "both" });
      },
      exit: function (el) {
        return animate(el, [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scaleX(1.1) scaleY(0.6)", opacity: 0.4, offset: 0.4 },
          { transform: "scaleX(0) scaleY(0)", opacity: 0 },
        ], { duration: 280, easing: "ease-in", fill: "both" });
      },
    },
    // ── cascade: staggered children reveal ────────────────────────────────────
    {
      id: "cascade",
      name: "Cascade",
      template: ANIM_HEADER + [
        "function enter(el) {",
        "  var children = Array.prototype.slice.call(el.children);",
        "  var promises = children.map(function(child, i) {",
        "    return child.animate([",
        "      { transform: 'translateY(16px)', opacity: 0 },",
        "      { transform: 'translateY(0)', opacity: 1 }",
        "    ], { duration: 380, delay: i * 80, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', fill: 'both' }).finished.catch(function(){});",
        "  });",
        "  return Promise.all(promises);",
        "}",
        "",
        "function exit(el) {",
        "  return el.animate([",
        "    { opacity: 1, transform: 'translateY(0)' },",
        "    { opacity: 0, transform: 'translateY(-10px)' }",
        "  ], { duration: 250, easing: 'ease-in', fill: 'both' }).finished.catch(function(){});",
        "}",
      ].join("\n"),
      enter: function (el) {
        var children = Array.prototype.slice.call(el.children);
        var promises = children.map(function (child, i) {
          return animate(child, [
            { transform: "translateY(16px)", opacity: 0 },
            { transform: "translateY(0)", opacity: 1 },
          ], { duration: 380, delay: i * 80, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" });
        });
        return Promise.all(promises);
      },
      exit: function (el) {
        return animate(el, [
          { opacity: 1, transform: "translateY(0)" },
          { opacity: 0, transform: "translateY(-10px)" },
        ], { duration: 250, easing: "ease-in", fill: "both" });
      },
    },
  ];

  var FALLBACK_FADE = { id: "fade", name: "Fade", enter: fadeEnter, exit: fadeExit };

  // Custom override maps: populated at runtime by the panel or overlay after loading config.
  var customPresets = {}; // id → { enter, exit }
  var customStyleCss = {}; // styleId → css string

  // Compile user-supplied animation code into { enter, exit } functions.
  // Injects holdLoop / stopHold / burstConfetti into scope so templates work as-is.
  // Throws on parse errors or if enter/exit are not defined.
  function compileAnim(code) {
    var factory = new Function("holdLoop", "stopHold", "burstConfetti",
      '"use strict"; ' + code + "; " +
      "if (typeof enter !== 'function' || typeof exit !== 'function') " +
      "throw new Error('enter(el) and exit(el) must be defined');  " +
      "return { enter: enter, exit: exit };"
    );
    return factory(holdLoop, stopHold, burstConfetti);
  }

  // Rebuild the single <style id="va-style-overrides"> from the customStyleCss map.
  function _rebuildStyleTag() {
    var parts = [];
    for (var id in customStyleCss) {
      if (Object.prototype.hasOwnProperty.call(customStyleCss, id) && customStyleCss[id]) {
        parts.push(customStyleCss[id]);
      }
    }
    var el = document.getElementById("va-style-overrides");
    if (!el) {
      el = document.createElement("style");
      el.id = "va-style-overrides";
      document.head.appendChild(el);
    }
    el.textContent = parts.join("\n\n");
  }

  function get(id) {
    // Reduced motion wins over everything: every preset degrades to a plain fade.
    if (reducedMotion()) return FALLBACK_FADE;
    // Custom override takes priority over the built-in preset.
    if (customPresets[id]) {
      var base = null;
      for (var j = 0; j < presets.length; j++) { if (presets[j].id === id) { base = presets[j]; break; } }
      return { id: id, name: (base ? base.name : id), enter: customPresets[id].enter, exit: customPresets[id].exit };
    }
    for (var i = 0; i < presets.length; i++) {
      if (presets[i].id === id) return presets[i];
    }
    return presets[0] || FALLBACK_FADE;
  }

  // ── card style presets (classes implemented in alert-styles.css) ─────────────
  var styles = [
    { id: "minimal-dark", name: "Minimal dark", className: "va-style-minimal-dark", css: [
      ".va-style-minimal-dark {",
      "  background: rgba(14, 17, 22, 0.92);",
      "  border: 1px solid #232936;",
      "  color: #E6EAF2;",
      "  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);",
      "}",
      ".va-style-minimal-dark .va-badge { color: #8A93A6; }",
      ".va-style-minimal-dark .va-tpl-author { color: #5B8CFF; }",
    ].join("\n") },
    { id: "neon-outline", name: "Neon outline", className: "va-style-neon-outline", css: [
      ".va-style-neon-outline {",
      "  background: rgba(7, 10, 16, 0.78);",
      "  border: 2px solid #29F0FF;",
      "  border-radius: 10px;",
      "  color: #FFFFFF;",
      "  box-shadow: 0 0 18px rgba(41, 240, 255, 0.55), inset 0 0 14px rgba(41, 240, 255, 0.18);",
      "}",
      ".va-style-neon-outline .va-badge {",
      "  color: #29F0FF;",
      "  text-shadow: 0 0 8px rgba(41, 240, 255, 0.8);",
      "}",
      ".va-style-neon-outline .va-text { text-shadow: 0 0 10px rgba(41, 240, 255, 0.45); }",
      ".va-style-neon-outline .va-tpl-author { color: #29F0FF; }",
    ].join("\n") },
    { id: "gradient-glass", name: "Gradient glass", className: "va-style-gradient-glass", css: [
      ".va-style-gradient-glass {",
      "  background: linear-gradient(135deg, rgba(91, 140, 255, 0.38), rgba(176, 91, 255, 0.34));",
      "  border: 1px solid rgba(255, 255, 255, 0.28);",
      "  border-radius: 18px;",
      "  color: #FFFFFF;",
      "  backdrop-filter: blur(10px) saturate(1.3);",
      "  -webkit-backdrop-filter: blur(10px) saturate(1.3);",
      "  box-shadow: 0 10px 36px rgba(20, 24, 48, 0.5), inset 0 1px 0 rgba(255, 255, 255, 0.35);",
      "  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.45);",
      "}",
      ".va-style-gradient-glass .va-badge { color: rgba(255, 255, 255, 0.82); }",
    ].join("\n") },
    { id: "gold-epic", name: "Gold epic", className: "va-style-gold-epic", css: [
      ".va-style-gold-epic {",
      "  background: linear-gradient(180deg, #2A1F06 0%, #4A3508 60%, #2A1F06 100%);",
      "  border: 2px solid #F5C84B;",
      "  border-radius: 12px;",
      "  color: #FFEFC2;",
      "  padding: 24px 44px;",
      "  box-shadow:",
      "    0 0 26px rgba(245, 200, 75, 0.55),",
      "    inset 0 1px 0 rgba(255, 236, 168, 0.55),",
      "    inset 0 -10px 24px rgba(0, 0, 0, 0.4);",
      "  letter-spacing: 0.01em;",
      "}",
      ".va-style-gold-epic .va-badge {",
      "  color: #F5C84B;",
      "  text-shadow: 0 0 10px rgba(245, 200, 75, 0.7);",
      "}",
      ".va-style-gold-epic .va-text { font-weight: 800; text-shadow: 0 2px 4px rgba(0, 0, 0, 0.5); }",
      ".va-style-gold-epic .va-tpl-author,",
      ".va-style-gold-epic .va-tpl-count,",
      ".va-style-gold-epic .va-tpl-viewers,",
      ".va-style-gold-epic .va-tpl-months { color: #FFD96B; }",
    ].join("\n") },
    { id: "retro-arcade", name: "Retro arcade", className: "va-style-retro-arcade", css: [
      ".va-style-retro-arcade {",
      "  background: #14045A;",
      "  border: 3px solid #FF2E88;",
      "  border-radius: 0;",
      "  color: #FFFFFF;",
      "  font-family: ui-monospace, \"SF Mono\", Menlo, Consolas, monospace;",
      "  text-transform: uppercase;",
      "  box-shadow: 7px 7px 0 #16E3FF;",
      "}",
      ".va-style-retro-arcade .va-badge { color: #16E3FF; letter-spacing: 0.3em; }",
      ".va-style-retro-arcade .va-text { font-size: 22px; letter-spacing: 0.06em; }",
      ".va-style-retro-arcade .va-tpl-author { color: #FF2E88; }",
    ].join("\n") },
    { id: "clean-light", name: "Clean light", className: "va-style-clean-light", css: [
      ".va-style-clean-light {",
      "  background: rgba(255, 255, 255, 0.97);",
      "  border: 1px solid #D8DEE9;",
      "  color: #14181F;",
      "  box-shadow: 0 12px 32px rgba(20, 24, 31, 0.22);",
      "}",
      ".va-style-clean-light .va-badge { color: #5B8CFF; }",
      ".va-style-clean-light .va-tpl-author { color: #3D6CE0; }",
    ].join("\n") },
    { id: "synthwave", name: "Synthwave", className: "va-style-synthwave", css: [
      ".va-style-synthwave {",
      "  background: linear-gradient(160deg, #1A0533 0%, #2D0A5F 50%, #0D0028 100%);",
      "  border: 1px solid #CC29FF;",
      "  border-radius: 10px;",
      "  color: #F5D6FF;",
      "  box-shadow: 0 0 30px rgba(204, 41, 255, 0.45), inset 0 0 18px rgba(204, 41, 255, 0.12);",
      "}",
      ".va-style-synthwave .va-badge { color: #FF6BFF; text-shadow: 0 0 10px rgba(255, 107, 255, 0.8); }",
      ".va-style-synthwave .va-tpl-author { color: #CC29FF; }",
    ].join("\n") },
    { id: "aurora", name: "Aurora", className: "va-style-aurora", css: [
      ".va-style-aurora {",
      "  background: linear-gradient(135deg, rgba(0, 200, 140, 0.28), rgba(100, 120, 255, 0.28), rgba(200, 60, 220, 0.22));",
      "  border: 1px solid rgba(120, 220, 200, 0.38);",
      "  border-radius: 16px;",
      "  color: #E0FFF8;",
      "  backdrop-filter: blur(12px) saturate(1.4);",
      "  box-shadow: 0 8px 32px rgba(0, 160, 120, 0.3), inset 0 1px 0 rgba(255, 255, 255, 0.3);",
      "}",
      ".va-style-aurora .va-badge { color: #00E5C4; text-shadow: 0 0 8px rgba(0, 229, 196, 0.7); }",
      ".va-style-aurora .va-tpl-author { color: #7BE8D5; }",
    ].join("\n") },
    { id: "fire", name: "Fire", className: "va-style-fire", css: [
      ".va-style-fire {",
      "  background: linear-gradient(180deg, #1A0700 0%, #4A1200 50%, #7A2000 100%);",
      "  border: 2px solid #FF5A00;",
      "  border-radius: 8px;",
      "  color: #FFE0C0;",
      "  box-shadow: 0 0 28px rgba(255, 90, 0, 0.6), inset 0 -1px 20px rgba(255, 30, 0, 0.3);",
      "}",
      ".va-style-fire .va-badge { color: #FF9E00; text-shadow: 0 0 10px rgba(255, 158, 0, 0.9); }",
      ".va-style-fire .va-tpl-author { color: #FFCC44; }",
    ].join("\n") },
    { id: "ocean", name: "Ocean", className: "va-style-ocean", css: [
      ".va-style-ocean {",
      "  background: linear-gradient(180deg, #021830 0%, #033B5C 60%, #021830 100%);",
      "  border: 1px solid rgba(0, 180, 240, 0.5);",
      "  border-radius: 14px;",
      "  color: #C0EEFF;",
      "  box-shadow: 0 8px 28px rgba(0, 120, 200, 0.4), inset 0 1px 0 rgba(100, 220, 255, 0.2);",
      "}",
      ".va-style-ocean .va-badge { color: #00C8F0; text-shadow: 0 0 8px rgba(0, 200, 240, 0.7); }",
      ".va-style-ocean .va-tpl-author { color: #55D8FF; }",
    ].join("\n") },
    { id: "sakura", name: "Sakura", className: "va-style-sakura", css: [
      ".va-style-sakura {",
      "  background-color: rgba(255, 240, 246, 0.88);",
      "  background: linear-gradient(140deg, rgba(255, 180, 200, 0.22), rgba(255, 220, 235, 0.18));",
      "  border: 1px solid rgba(255, 160, 190, 0.5);",
      "  border-radius: 20px;",
      "  color: #3D1825;",
      "  backdrop-filter: blur(8px);",
      "  box-shadow: 0 6px 24px rgba(220, 80, 120, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.5);",
      "}",
      ".va-style-sakura .va-badge { color: #C84078; }",
      ".va-style-sakura .va-tpl-author { color: #A0204A; }",
    ].join("\n") },
    { id: "midnight", name: "Midnight", className: "va-style-midnight", css: [
      ".va-style-midnight {",
      "  background: rgba(8, 10, 20, 0.96);",
      "  border: 1px solid #2A3060;",
      "  border-radius: 10px;",
      "  color: #C8D0F0;",
      "  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.7), inset 0 1px 0 rgba(80, 100, 200, 0.2);",
      "}",
      ".va-style-midnight .va-badge { color: #6070E0; }",
      ".va-style-midnight .va-tpl-author { color: #8898FF; }",
    ].join("\n") },
    { id: "cyber", name: "Cyber", className: "va-style-cyber", css: [
      ".va-style-cyber {",
      "  background: #0A0F18;",
      "  border: 2px solid #E8FF00;",
      "  border-radius: 4px;",
      "  color: #E8FF00;",
      "  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;",
      "  text-transform: uppercase;",
      "  letter-spacing: 0.08em;",
      "  box-shadow: 0 0 24px rgba(232, 255, 0, 0.35), 4px 4px 0 #00FFEE;",
      "}",
      ".va-style-cyber .va-badge { color: #00FFEE; text-shadow: 0 0 8px rgba(0, 255, 238, 0.9); }",
      ".va-style-cyber .va-tpl-author { color: #FFFFFF; }",
    ].join("\n") },
    { id: "sunset", name: "Sunset", className: "va-style-sunset", css: [
      ".va-style-sunset {",
      "  background: linear-gradient(135deg, #3A0A18 0%, #7A1A30 40%, #C04820 80%, #D47A00 100%);",
      "  border: 1px solid rgba(255, 140, 60, 0.5);",
      "  border-radius: 12px;",
      "  color: #FFE5C8;",
      "  box-shadow: 0 10px 32px rgba(180, 60, 30, 0.5);",
      "}",
      ".va-style-sunset .va-badge { color: #FFAD60; text-shadow: 0 0 8px rgba(255, 173, 96, 0.7); }",
      ".va-style-sunset .va-tpl-author { color: #FFD080; }",
    ].join("\n") },
    { id: "neon-pink", name: "Neon pink", className: "va-style-neon-pink", css: [
      ".va-style-neon-pink {",
      "  background: rgba(12, 2, 18, 0.9);",
      "  border: 2px solid #FF2D92;",
      "  border-radius: 10px;",
      "  color: #FFFFFF;",
      "  box-shadow: 0 0 22px rgba(255, 45, 146, 0.6), inset 0 0 16px rgba(255, 45, 146, 0.15);",
      "}",
      ".va-style-neon-pink .va-badge { color: #FF2D92; text-shadow: 0 0 10px rgba(255, 45, 146, 0.9); }",
      ".va-style-neon-pink .va-tpl-author { color: #FF80C2; }",
    ].join("\n") },
    { id: "crystal", name: "Crystal", className: "va-style-crystal", css: [
      ".va-style-crystal {",
      "  background: linear-gradient(135deg, rgba(180, 220, 255, 0.22), rgba(220, 200, 255, 0.18), rgba(180, 255, 240, 0.2));",
      "  border: 1px solid rgba(200, 230, 255, 0.55);",
      "  border-radius: 18px;",
      "  color: #E8F4FF;",
      "  backdrop-filter: blur(14px) saturate(1.6) brightness(1.1);",
      "  box-shadow: 0 8px 28px rgba(100, 160, 255, 0.25), inset 0 1px 0 rgba(255, 255, 255, 0.7);",
      "}",
      ".va-style-crystal .va-badge { color: #A0C8FF; }",
      ".va-style-crystal .va-tpl-author { color: #C0D8FF; }",
    ].join("\n") },
    { id: "vapor", name: "Vapor", className: "va-style-vapor", css: [
      ".va-style-vapor {",
      "  background: linear-gradient(140deg, #FF77E9 0%, #8B5CF6 50%, #38BDF8 100%);",
      "  border: none;",
      "  border-radius: 16px;",
      "  color: #FFFFFF;",
      "  box-shadow: 0 8px 32px rgba(139, 92, 246, 0.5), inset 0 1px 0 rgba(255, 255, 255, 0.4);",
      "  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);",
      "}",
      ".va-style-vapor .va-badge { color: rgba(255, 255, 255, 0.9); letter-spacing: 0.15em; }",
      ".va-style-vapor .va-tpl-author { color: #FFE6FF; }",
    ].join("\n") },
    { id: "obsidian", name: "Obsidian", className: "va-style-obsidian", css: [
      ".va-style-obsidian {",
      "  background: linear-gradient(180deg, #0E0E0E 0%, #1A1010 100%);",
      "  border: 1px solid #3A1A1A;",
      "  border-radius: 6px;",
      "  color: #E8D8C8;",
      "  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.8), inset 0 1px 0 rgba(200, 100, 50, 0.15);",
      "}",
      ".va-style-obsidian .va-badge { color: #C04020; letter-spacing: 0.1em; }",
      ".va-style-obsidian .va-tpl-author { color: #E06040; }",
    ].join("\n") },
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

  var DEFAULT_SOUND   = { src: "", volume: 0.8, muted: false };
  var DEFAULT_TTS     = { enabled: false, rate: 1.0, pitch: 1.0, template: "" };
  var DEFAULT_MEDIA   = { src: "", fit: "contain", position: "top" };
  var DEFAULT_WEBHOOK = { url: "", enabled: false };

  function defaultTypeRule(anim, style, template, tiers) {
    return {
      enabled: true, animation: anim, style: style,
      durationMs: DEFAULT_DURATION_MS, template: template,
      badge: "", animPool: [], stylePool: [],
      sound:   Object.assign({}, DEFAULT_SOUND),
      tts:     Object.assign({}, DEFAULT_TTS),
      media:   Object.assign({}, DEFAULT_MEDIA),
      webhook: Object.assign({}, DEFAULT_WEBHOOK),
      cooldownMs: 0, queueMax: 0,
      tiers: tiers || [],
    };
  }

  function defaultRules() {
    return {
      version: 1,
      enabled: true,
      position: "bottom",
      gapMs: 500,
      types: {
        sub:      defaultTypeRule("slide-bounce", "gradient-glass", "{author} just subscribed!"),
        resub:    defaultTypeRule("fade-zoom",    "gradient-glass", "{author} resubscribed for {months} months!", [{ min: 12, animation: "neon-pulse", style: "neon-outline" }]),
        giftsub:  defaultTypeRule("pop",          "neon-outline",   "{author} gifted {count} subs!",              [{ min: 5, animation: "shake" }, { min: 20, animation: "confetti", style: "gold-epic" }]),
        raid:     defaultTypeRule("slide-up",     "minimal-dark",   "{author} is raiding with {viewers} viewers!", [{ min: 50, animation: "pop", style: "neon-outline" }, { min: 200, animation: "confetti", style: "gold-epic" }]),
        host:     defaultTypeRule("slide-up",     "minimal-dark",   "{author} is hosting with {viewers} viewers!"),
        follow:   defaultTypeRule("slide-up",     "minimal-dark",   "{author} just followed!"),
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
  function normSound(s) {
    if (!s || typeof s !== "object") return Object.assign({}, DEFAULT_SOUND);
    return {
      src:    typeof s.src === "string" ? s.src : "",
      volume: typeof s.volume === "number" ? Math.max(0, Math.min(1, s.volume)) : 0.8,
      muted:  !!s.muted,
    };
  }

  function normTts(t) {
    if (!t || typeof t !== "object") return Object.assign({}, DEFAULT_TTS);
    return {
      enabled:  !!t.enabled,
      rate:     typeof t.rate === "number" ? Math.max(0.1, Math.min(10, t.rate)) : 1.0,
      pitch:    typeof t.pitch === "number" ? Math.max(0, Math.min(2, t.pitch)) : 1.0,
      template: typeof t.template === "string" ? t.template : "",
    };
  }

  function normMedia(m) {
    if (!m || typeof m !== "object") return Object.assign({}, DEFAULT_MEDIA);
    return {
      src:      typeof m.src === "string" ? m.src : "",
      fit:      m.fit === "cover" || m.fit === "fill" ? m.fit : "contain",
      position: m.position === "bottom" || m.position === "bg" ? m.position : "top",
    };
  }

  function normWebhook(w) {
    if (!w || typeof w !== "object") return Object.assign({}, DEFAULT_WEBHOOK);
    return {
      url:     typeof w.url === "string" ? w.url : "",
      enabled: !!w.enabled,
    };
  }

  function normalizeRules(raw) {
    var def = defaultRules();
    if (!raw || typeof raw !== "object") return def;
    var gapMs = typeof raw.gapMs === "number" && raw.gapMs >= 0 ? Math.round(raw.gapMs) : def.gapMs;
    var out = {
      version: 1,
      enabled: typeof raw.enabled === "boolean" ? raw.enabled : def.enabled,
      position: raw.position === "top" || raw.position === "center" || raw.position === "bottom" ? raw.position : def.position,
      gapMs: gapMs,
      types: {},
    };
    eventTypes.forEach(function (t) {
      var d = def.types[t.id];
      var r = raw.types && raw.types[t.id];
      if (!r || typeof r !== "object") { out.types[t.id] = d; return; }
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
          if (typeof tier.badge === "string" && tier.badge) clean.badge = tier.badge;
          var dur = Number(tier.durationMs);
          if (isFinite(dur) && dur > 0) clean.durationMs = clampDuration(dur, d.durationMs);
          tiers.push(clean);
        });
        tiers.sort(function (a, b) { return a.min - b.min; });
      }
      var animPool = Array.isArray(r.animPool) ? r.animPool.filter(function(s){ return typeof s === "string" && s; }) : [];
      var stylePool = Array.isArray(r.stylePool) ? r.stylePool.filter(function(s){ return typeof s === "string" && s; }) : [];
      out.types[t.id] = {
        enabled:    typeof r.enabled === "boolean" ? r.enabled : d.enabled,
        animation:  typeof r.animation === "string" && r.animation ? r.animation : d.animation,
        style:      typeof r.style === "string" && r.style ? r.style : d.style,
        durationMs: clampDuration(r.durationMs, d.durationMs),
        template:   typeof r.template === "string" && r.template ? r.template : d.template,
        badge:      typeof r.badge === "string" ? r.badge : "",
        animPool:   animPool,
        stylePool:  stylePool,
        sound:      normSound(r.sound),
        tts:        normTts(r.tts),
        media:      normMedia(r.media),
        webhook:    normWebhook(r.webhook),
        cooldownMs: typeof r.cooldownMs === "number" && r.cooldownMs >= 0 ? Math.round(r.cooldownMs) : 0,
        queueMax:   typeof r.queueMax === "number" && r.queueMax >= 0 ? Math.round(r.queueMax) : 0,
        tiers:      tiers,
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
  function pickPool(base, pool) {
    var candidates = [base].concat(Array.isArray(pool) ? pool : []).filter(function (s) { return typeof s === "string" && s; });
    return candidates.length ? candidates[(Math.random() * candidates.length) | 0] : (base || "");
  }

  function resolveRule(rule, magnitude) {
    rule = rule || {};
    var eff = {
      animation: pickPool(rule.animation || "slide-up", rule.animPool),
      style:     pickPool(rule.style || "minimal-dark", rule.stylePool),
      durationMs: clampDuration(rule.durationMs, DEFAULT_DURATION_MS),
      template: rule.template || "{author}",
      badge:    typeof rule.badge === "string" ? rule.badge : "",
      sound:    rule.sound   || DEFAULT_SOUND,
      tts:      rule.tts     || DEFAULT_TTS,
      media:    rule.media   || DEFAULT_MEDIA,
      webhook:  rule.webhook || DEFAULT_WEBHOOK,
    };
    if (typeof magnitude === "number" && Array.isArray(rule.tiers)) {
      var best = null;
      rule.tiers.forEach(function (t) {
        if (!t || typeof t.min !== "number" || magnitude < t.min) return;
        if (!best || t.min > best.min) best = t;
      });
      if (best) {
        if (best.animation) eff.animation = pickPool(best.animation, best.animPool);
        if (best.style)     eff.style     = pickPool(best.style, best.stylePool);
        if (best.template)  eff.template  = best.template;
        if (best.badge)     eff.badge     = best.badge;
        if (best.durationMs) eff.durationMs = clampDuration(best.durationMs, eff.durationMs);
      }
    }
    return eff;
  }

  // ── card building & playback ─────────────────────────────────────────────────
  // Matches any {identifier} — known built-ins handled first, then arbitrary payload fields
  // for custom hook events. applyTemplate uses textContent throughout so injection is impossible.
  var PLACEHOLDER_RE = /\{([a-zA-Z_][a-zA-Z0-9_]*)\}/g;

  function placeholderValue(key, data) {
    switch (key) {
      case "author": return data.author || "Someone";
      case "count": return String(data.count != null ? data.count : 1);
      case "viewers": return String(data.viewers != null ? data.viewers : 0);
      case "months": return String(data.months != null ? data.months : 0);
      case "tier": return data.tier != null ? String(data.tier) : "";
      default:
        // Custom payload field from a webhook event (stored on data._ext).
        var ext = data._ext;
        if (ext && Object.prototype.hasOwnProperty.call(ext, key) && ext[key] != null) {
          return String(ext[key]);
        }
        return "";
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

  // ── sound engine ─────────────────────────────────────────────────────────────
  var _audioCtx = null;
  function audioCtx() {
    if (!_audioCtx) {
      try { _audioCtx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) {}
    }
    if (_audioCtx && _audioCtx.state === "suspended") {
      try { _audioCtx.resume(); } catch (e) {}
    }
    return _audioCtx;
  }

  var BUILTIN_SOUNDS = {
    chime: function (ctx, vol) {
      [523, 659, 784].forEach(function (f, i) {
        var o = ctx.createOscillator(), g = ctx.createGain();
        o.connect(g); g.connect(ctx.destination);
        o.frequency.value = f; o.type = "sine";
        var t = ctx.currentTime + i * 0.13;
        g.gain.setValueAtTime(vol * 0.38, t);
        g.gain.exponentialRampToValueAtTime(0.001, t + 0.55);
        o.start(t); o.stop(t + 0.55);
      });
    },
    pop: function (ctx, vol) {
      var len = Math.floor(ctx.sampleRate * 0.12);
      var buf = ctx.createBuffer(1, len, ctx.sampleRate);
      var d = buf.getChannelData(0);
      for (var i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.pow(1 - i / len, 3);
      var s = ctx.createBufferSource(), g = ctx.createGain();
      s.buffer = buf; s.connect(g); g.connect(ctx.destination);
      g.gain.value = vol * 0.7; s.start();
    },
    whoosh: function (ctx, vol) {
      var o = ctx.createOscillator(), g = ctx.createGain();
      o.connect(g); g.connect(ctx.destination);
      o.type = "sawtooth";
      o.frequency.setValueAtTime(60, ctx.currentTime);
      o.frequency.exponentialRampToValueAtTime(900, ctx.currentTime + 0.38);
      g.gain.setValueAtTime(0, ctx.currentTime);
      g.gain.linearRampToValueAtTime(vol * 0.18, ctx.currentTime + 0.08);
      g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.38);
      o.start(); o.stop(ctx.currentTime + 0.38);
    },
    bell: function (ctx, vol) {
      [440, 880, 1320].forEach(function (f, i) {
        var o = ctx.createOscillator(), g = ctx.createGain();
        o.connect(g); g.connect(ctx.destination);
        o.frequency.value = f; o.type = "sine";
        var amp = vol * [0.45, 0.22, 0.1][i];
        g.gain.setValueAtTime(amp, ctx.currentTime);
        g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 1.6);
        o.start(); o.stop(ctx.currentTime + 1.6);
      });
    },
    notify: function (ctx, vol) {
      [420, 560].forEach(function (f, i) {
        var o = ctx.createOscillator(), g = ctx.createGain();
        o.connect(g); g.connect(ctx.destination);
        o.frequency.value = f; o.type = "sine";
        var t = ctx.currentTime + i * 0.16;
        g.gain.setValueAtTime(vol * 0.32, t);
        g.gain.exponentialRampToValueAtTime(0.001, t + 0.22);
        o.start(t); o.stop(t + 0.22);
      });
    },
    cash: function (ctx, vol) {
      // Ascending arpeggio + register sound
      [330, 415, 523, 659].forEach(function (f, i) {
        var o = ctx.createOscillator(), g = ctx.createGain();
        o.connect(g); g.connect(ctx.destination);
        o.frequency.value = f; o.type = "triangle";
        var t = ctx.currentTime + i * 0.09;
        g.gain.setValueAtTime(vol * 0.3, t);
        g.gain.exponentialRampToValueAtTime(0.001, t + 0.18);
        o.start(t); o.stop(t + 0.18);
      });
    },
  };

  function playSound(soundCfg) {
    if (!soundCfg || !soundCfg.src || soundCfg.muted) return;
    var vol = typeof soundCfg.volume === "number" ? Math.max(0, Math.min(1, soundCfg.volume)) : 0.8;
    if (soundCfg.src.indexOf("builtin:") === 0) {
      var name = soundCfg.src.slice(8);
      var fn = BUILTIN_SOUNDS[name];
      if (!fn) return;
      var ctx = audioCtx();
      if (!ctx) return;
      try { fn(ctx, vol); } catch (e) { /* AudioContext may be blocked */ }
    } else {
      try {
        var audio = new Audio(soundCfg.src);
        audio.volume = vol;
        audio.play().catch(function () {});
      } catch (e) {}
    }
  }

  // ── text-to-speech ───────────────────────────────────────────────────────────
  function templateText(template, data) {
    var tpl = String(template || "");
    PLACEHOLDER_RE.lastIndex = 0;
    return tpl.replace(PLACEHOLDER_RE, function (_, key) { return placeholderValue(key, data); });
  }

  function speakAlert(text, ttsCfg) {
    if (!ttsCfg || !ttsCfg.enabled || !text || !window.speechSynthesis) return;
    try {
      window.speechSynthesis.cancel();
      var utt = new SpeechSynthesisUtterance(text);
      utt.rate  = typeof ttsCfg.rate  === "number" ? ttsCfg.rate  : 1.0;
      utt.pitch = typeof ttsCfg.pitch === "number" ? ttsCfg.pitch : 1.0;
      window.speechSynthesis.speak(utt);
    } catch (e) {}
  }

  // ── outbound webhook (fire-and-forget, no-cors, overlay only) ───────────────
  function fireWebhook(url, data, eff) {
    if (!url) return;
    try {
      fetch(url, {
        method: "POST",
        mode: "no-cors",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          event:  data.type  || "alert",
          author: data.author || "",
          text:   templateText(eff.template, data),
          badge:  eff.badge || data.badge || (typeInfo(data.type) ? typeInfo(data.type).badge : "ALERT"),
        }),
      }).catch(function () { /* fire and forget */ });
    } catch (e) { /* never wedge the queue */ }
  }

  // ── card rendering ───────────────────────────────────────────────────────────
  // Build the alert card, play enter → hold → exit inside `container`, then remove it.
  // alertData: { type, author, count?, viewers?, months?, tier? }. rule: the per-type rule
  // (tiers included — resolution happens here so panel preview and overlay can't diverge).
  // onCard is an optional callback invoked synchronously after the card is appended to container.
  // opts: { sound: bool, tts: bool, webhook: bool } — panel passes false for all.
  function renderAlert(container, alertData, rule, onCard, opts) {
    opts = opts || {};
    var data = alertData || {};
    var eff = resolveRule(rule, magnitudeOf(data.type, data));
    var style = getStyle(eff.style);
    var info = typeInfo(data.type);

    var card = document.createElement("div");
    card.className = "va-card va-type-" + (data.type || "sub") + " " + style.className;

    // Badge
    var badge = document.createElement("span");
    badge.className = "va-badge";
    badge.textContent = eff.badge || data.badge || (info ? info.badge : "ALERT");
    card.appendChild(badge);

    // Optional media image
    var mediaImg = null;
    if (eff.media && eff.media.src) {
      if (eff.media.position === "bg") {
        card.style.backgroundImage = "url(" + eff.media.src + ")";
        card.style.backgroundSize = "cover";
        card.style.backgroundPosition = "center";
        card.classList.add("va-has-media-bg");
      } else {
        mediaImg = document.createElement("img");
        mediaImg.src = eff.media.src;
        mediaImg.className = "va-media";
        mediaImg.alt = "";
        mediaImg.style.objectFit = eff.media.fit || "contain";
        if (eff.media.position !== "bottom") card.appendChild(mediaImg); // top: after badge
      }
    }

    // Text
    var text = document.createElement("div");
    text.className = "va-text";
    text.appendChild(applyTemplate(eff.template, data));
    card.appendChild(text);
    if (mediaImg && eff.media.position === "bottom") card.appendChild(mediaImg);

    container.appendChild(card);
    if (onCard) onCard(card);

    var preset = get(eff.animation);
    return Promise.resolve()
      .then(function () {
        if (opts.sound !== false) playSound(eff.sound);
        if (opts.webhook !== false && eff.webhook && eff.webhook.enabled && eff.webhook.url) {
          fireWebhook(eff.webhook.url, data, eff);
        }
        var ttsText = (eff.tts && eff.tts.template) ? templateText(eff.tts.template, data) : templateText(eff.template, data);
        if (opts.tts !== false) speakAlert(ttsText, eff.tts);
        return preset.enter(card);
      })
      .then(function () { return wait(eff.durationMs); })
      .then(function () { return preset.exit(card); })
      .catch(function () { /* a broken preset must not wedge the queue */ })
      .then(function () {
        stopHold(card);
        if (card.parentNode) card.parentNode.removeChild(card);
      });
  }

  // Strictly-serial alert player: one alert at a time, configurable gap, queue capped at 20.
  var QUEUE_CAP = 20;

  // opts: { gapMs, sound, tts, webhook }
  //   gapMs   – ms pause between alerts (default 500)
  //   sound   – pass false to silence all audio (panel live-preview player)
  //   tts     – pass false to suppress TTS (panel live-preview player)
  //   webhook – pass false to suppress outbound webhooks (panel live-preview player)
  function createPlayer(container, opts) {
    opts = opts || {};
    var gapMs      = typeof opts.gapMs === "number" && opts.gapMs >= 0 ? opts.gapMs : 500;
    var doSound    = opts.sound   !== false;
    var doTts      = opts.tts     !== false;
    var doWebhook  = opts.webhook !== false;

    var queue = [];
    var playing = false;
    var currentCard = null;
    // Incremented on every flash/cancel; each pump run captures its value so stale promise
    // chains bail out rather than interfering with the freshly started alert.
    var epoch = 0;
    // Cooldown tracking: type → last-fired timestamp (ms).
    var lastFired = {};
    // Per-type items currently in queue (for queueMax).
    var typeCount = {};

    function cancelActive() {
      epoch++;
      queue.length = 0;
      typeCount = {};
      playing = false;
      if (currentCard) {
        (currentCard.getAnimations ? currentCard.getAnimations() : [])
          .forEach(function (a) { try { a.cancel(); } catch (e) {} });
        stopHold(currentCard);
        if (currentCard.parentNode) currentCard.parentNode.removeChild(currentCard);
        currentCard = null;
      }
    }

    function pump() {
      if (playing) return;
      var next = queue.shift();
      if (!next) return;
      var t = next.data && next.data.type;
      if (t && typeCount[t]) typeCount[t] = Math.max(0, typeCount[t] - 1);
      playing = true;
      var myEpoch = epoch;
      var p = renderAlert(container, next.data, next.rule, function (card) {
        if (epoch === myEpoch) currentCard = card;
      }, { sound: doSound, tts: doTts, webhook: doWebhook });
      if (t) lastFired[t] = Date.now();
      p.then(function () {
        if (epoch !== myEpoch) return;
        currentCard = null;
        return wait(gapMs);
      }).then(function () {
        if (epoch !== myEpoch) return;
        playing = false;
        pump();
      });
    }

    return {
      enqueue: function (data, rule) {
        var t = data && data.type;
        // Cooldown: skip if this type fired too recently.
        if (t && rule && rule.cooldownMs > 0) {
          if (Date.now() - (lastFired[t] || 0) < rule.cooldownMs) return;
        }
        // Per-type queue cap.
        if (t && rule && rule.queueMax > 0) {
          if ((typeCount[t] || 0) >= rule.queueMax) return;
        }
        if (queue.length >= QUEUE_CAP) queue.shift();
        queue.push({ data: data, rule: rule });
        if (t) typeCount[t] = (typeCount[t] || 0) + 1;
        pump();
      },
      // Immediately cancel whatever is playing, clear the queue, and start the new alert.
      flash: function (data, rule) {
        cancelActive();
        queue.push({ data: data, rule: rule });
        pump();
      },
      clear: function () { cancelActive(); },
      // Update gap dynamically when the panel saves new settings.
      setGap: function (ms) { gapMs = typeof ms === "number" && ms >= 0 ? ms : 500; },
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
    playSound: playSound,
    speakAlert: speakAlert,
    templateText: templateText,
    BUILTIN_SOUNDS: BUILTIN_SOUNDS,
    stopHold: stopHold,
    holdLoop: holdLoop,
    burstConfetti: burstConfetti,
    wait: wait,
    // Custom overrides — used by the panel and overlay to apply saved customisations.
    compileAnim: compileAnim,
    setCustomPreset: function (id, code) {
      var fns = compileAnim(code);
      customPresets[id] = fns;
    },
    removeCustomPreset: function (id) {
      delete customPresets[id];
    },
    setStyleOverride: function (id, css) {
      customStyleCss[id] = css || "";
      _rebuildStyleTag();
    },
    clearStyleOverride: function (id) {
      delete customStyleCss[id];
      _rebuildStyleTag();
    },
    clearAllOverrides: function () {
      for (var k in customPresets) { if (Object.prototype.hasOwnProperty.call(customPresets, k)) delete customPresets[k]; }
      for (var s in customStyleCss) { if (Object.prototype.hasOwnProperty.call(customStyleCss, s)) delete customStyleCss[s]; }
      _rebuildStyleTag();
    },
  };
})();

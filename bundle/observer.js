// Virta X broadcast chat observer. Injected by the x-bridge binary into the broadcast page.
// Connects a MutationObserver to the chat container, extracts each new message, and sends it
// back via the bridge socket as a FrameMessage (newline-delimited JSON).
// Versioned in bundle.json; the x-bridge verifies the version before injecting.
(function (cfg) {
  'use strict';

  // --- Selector self-test: emit FrameError(selector_missing) if any required selector is absent.
  const REQUIRED = ['chatRow', 'authorHandle', 'messageText'];
  for (const key of REQUIRED) {
    if (!cfg.selectors[key]) {
      bridge.send({ type: 'error', payload: { code: 'selector_missing', detail: key } });
      return;
    }
    if (!document.querySelector(cfg.selectors[key])) {
      // Selector valid but nothing matched yet — that's OK (chat may not have started).
      // The observer will catch new rows as they appear.
    }
  }

  // Detect broadcast-ended and notify.
  function checkBroadcastEnded() {
    if (document.querySelector(cfg.selectors.broadcastEnded)) {
      bridge.send({ type: 'status', payload: { state: 'x_broadcast_ended' } });
    }
  }

  // Extract one message row into a MessagePayload.
  function extractRow(el) {
    const authorEl = el.querySelector(cfg.selectors.authorHandle);
    const textEl = el.querySelector(cfg.selectors.messageText);
    if (!authorEl || !textEl) return null;
    const author = (authorEl.textContent || '').trim().replace(/^@/, '');
    const text = (textEl.textContent || '').trim();
    if (!author || !text) return null;
    const verified = !!el.querySelector(cfg.selectors.verifiedBadge);
    const contentHash = author + '\x00' + text; // de-duplication key (hashed by Go side)
    return {
      broadcast_id: cfg.broadcastId || location.pathname.replace(/\//g, '-').replace(/^-/, ''),
      author,
      verified,
      text,
      content_hash: contentHash,
      timestamp_ms: Date.now(),
    };
  }

  // Observe the document for new chat rows (X appends them to a scroll container).
  const seen = new WeakSet();
  const observer = new MutationObserver((mutations) => {
    for (const mut of mutations) {
      for (const node of mut.addedNodes) {
        if (node.nodeType !== 1) continue;
        const rows = node.matches(cfg.selectors.chatRow)
          ? [node]
          : Array.from(node.querySelectorAll(cfg.selectors.chatRow));
        for (const row of rows) {
          if (seen.has(row)) continue;
          seen.add(row);
          const msg = extractRow(row);
          if (msg) bridge.send({ type: 'message', payload: msg });
        }
      }
    }
    checkBroadcastEnded();
  });

  bridge.send({ type: 'status', payload: { state: 'x_bridge_connected' } });
  observer.observe(document.body, { childList: true, subtree: true });
})(BUNDLE_CONFIG);

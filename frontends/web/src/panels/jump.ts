// Routes "jump to this message" requests to whichever open feed panel still has the message
// buffered. A module-level registry (not context) because the requester and the feed panels render
// in separate dockview trees. Handlers claim a request by returning their panel id; the requester
// activates that panel and the panel's feed scrolls. Unclaimed requests fall back to the caller.
export type JumpRequest = { channel: string; id: string };
export type JumpClaim = { panelId?: string };
type Handler = (req: JumpRequest) => JumpClaim | null;

const handlers = new Set<Handler>();

export function onJumpRequest(h: Handler): () => void {
  handlers.add(h);
  return () => handlers.delete(h);
}

/** Offer the request to every registered feed; the first claim wins. Null = nobody has it. */
export function requestJump(req: JumpRequest): JumpClaim | null {
  for (const h of handlers) {
    const claim = h(req);
    if (claim) return claim;
  }
  return null;
}

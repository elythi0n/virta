/** Fetch and consume the pending virta:// deep-link URL set via CLI argument. Returns "" if none. */
export async function fetchPendingDeepLink(): Promise<string> {
  try {
    const r = await fetch('/__deeplink');
    if (!r.ok) return '';
    const d = await r.json() as { url?: string };
    return d.url ?? '';
  } catch {
    return '';
  }
}

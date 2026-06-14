import { pluginHttp } from '../daemon/plugins';
import type { ChatLine } from './types';

// Twitch VOD chat: fetches a video's stored comments from the public GQL endpoint, paged by content
// offset / cursor, and keeps a buffer sorted by offset so the reviewer can render chat synced to the
// playback clock. The desktop shell injects permissive CORS headers for this host so the fetch works
// from the renderer.

const GQL_URL = 'https://gql.twitch.tv/gql';
const CLIENT_ID = 'kimne78kx3ncx6brgo4mv6wki5h1ko'; // public web client id
const COMMENTS_HASH = 'b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a';

export function parseVodId(input: string): string {
  const s = (input || '').trim();
  if (/^\d+$/.test(s)) return s;
  const m = s.match(/twitch\.tv\/videos\/(\d+)/i);
  return m ? m[1] : '';
}

// Lighten very dark user colors so names stay legible on a dark overlay.
function clampColor(hex: string | null | undefined): string {
  if (!hex || !/^#?[0-9a-f]{6}$/i.test(hex)) return '#b9c2d0';
  const h = hex[0] === '#' ? hex.slice(1) : hex;
  let r = parseInt(h.slice(0, 2), 16);
  let g = parseInt(h.slice(2, 4), 16);
  let b = parseInt(h.slice(4, 6), 16);
  const lum = 0.299 * r + 0.587 * g + 0.114 * b;
  if (lum < 70) {
    const f = 70 / Math.max(lum, 1);
    r = Math.min(255, Math.round(r * f));
    g = Math.min(255, Math.round(g * f));
    b = Math.min(255, Math.round(b * f));
  }
  return `#${[r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('')}`;
}

interface Page {
  lines: ChatLine[];
  cursor: string;
  hasNext: boolean;
  lengthSeconds: number;
}

const VOD_PLUGIN_ID = 'com.virta.vod-replay';

// Runs the GQL request the way the VOD replayer does: through the daemon's HTTP bridge (no CORS,
// proven path). Falls back to a direct fetch when the bridge isn't available (e.g. the VOD replay
// plugin isn't enabled).
async function gqlPost(body: unknown, signal?: AbortSignal): Promise<unknown> {
  const payload = JSON.stringify(body);
  const headers = { 'Client-Id': CLIENT_ID, 'Content-Type': 'application/json' };
  // 1) Desktop main process — most reliable (no CORS, no plugin dependency).
  const gql = typeof window !== 'undefined' ? window.virta?.twitchGql : undefined;
  if (gql) {
    try {
      return await gql(body);
    } catch {
      /* fall through */
    }
  }
  // 2) Daemon HTTP bridge via the VOD replay plugin (the proven path when that plugin is enabled).
  try {
    const res = await pluginHttp(VOD_PLUGIN_ID, { url: GQL_URL, method: 'POST', headers, body: payload });
    if (res.status >= 200 && res.status < 300 && res.body) return JSON.parse(res.body);
  } catch {
    /* fall through */
  }
  // 3) Direct fetch (browser dev, or desktop with CORS headers injected).
  const direct = await fetch(GQL_URL, { method: 'POST', headers, body: payload, signal });
  if (!direct.ok) throw new Error(`Twitch GQL ${direct.status}`);
  return direct.json();
}

async function fetchPage(vodId: string, at: { offset: number } | { cursor: string }, signal?: AbortSignal): Promise<Page> {
  const variables: Record<string, unknown> =
    'cursor' in at ? { videoID: vodId, cursor: at.cursor } : { videoID: vodId, contentOffsetSeconds: at.offset };
  const body = [
    {
      operationName: 'VideoCommentsByOffsetOrCursor',
      variables,
      extensions: { persistedQuery: { version: 1, sha256Hash: COMMENTS_HASH } },
    },
  ];
  const json = (await gqlPost(body, signal)) as Array<{ data?: { video?: Record<string, unknown> } }>;
  const video = json?.[0]?.data?.video as Record<string, unknown> | undefined;
  if (!video) throw new Error('VOD not found or unavailable');
  const comments = video.comments as
    | { edges?: unknown[]; pageInfo?: { hasNextPage?: boolean } }
    | undefined;
  const edges: unknown[] = comments?.edges ?? [];
  let cursor = '';
  const lines: ChatLine[] = [];
  for (const e of edges as Array<{ cursor?: string; node?: Record<string, unknown> }>) {
    if (e.cursor) cursor = e.cursor;
    const n = e.node as Record<string, unknown> | undefined;
    if (!n) continue;
    const msg = n.message as { userColor?: string; fragments?: Array<{ text?: string }> } | undefined;
    const commenter = n.commenter as { displayName?: string } | undefined;
    const text = (msg?.fragments ?? []).map((f) => f.text ?? '').join('');
    lines.push({
      id: String(n.id ?? `${n.contentOffsetSeconds}-${lines.length}`),
      offset: Number(n.contentOffsetSeconds ?? 0),
      author: commenter?.displayName ?? '',
      color: clampColor(msg?.userColor),
      text,
    });
  }
  lines.sort((a, b) => a.offset - b.offset);
  return {
    lines,
    cursor,
    hasNext: Boolean(comments?.pageInfo?.hasNextPage),
    lengthSeconds: Number((video.lengthSeconds as number) ?? 0),
  };
}

const BUFFER_AHEAD = 120; // seconds of chat to keep loaded ahead of the clock

// VodChatBuffer owns the loaded comment window and fetches more as the clock advances or seeks.
export class VodChatBuffer {
  readonly vodId: string;
  lengthSeconds = 0;
  private lines: ChatLine[] = [];
  private cursor = '';
  private hasNext = true;
  private horizon = -1; // highest offset loaded
  private gen = 0; // bumped on seek so stale fetches are dropped
  private fetching = false;

  constructor(vodId: string) {
    this.vodId = vodId;
  }

  all(): ChatLine[] {
    return this.lines;
  }

  // linesUpTo returns up to `max` lines whose offset <= clock, newest last.
  linesUpTo(clock: number, max: number): ChatLine[] {
    const out: ChatLine[] = [];
    for (let i = this.lines.length - 1; i >= 0 && out.length < max; i--) {
      if (this.lines[i].offset <= clock) out.push(this.lines[i]);
    }
    return out.reverse();
  }

  async seek(offset: number): Promise<void> {
    const gen = ++this.gen;
    this.lines = [];
    this.cursor = '';
    this.hasNext = true;
    this.horizon = offset;
    const page = await fetchPage(this.vodId, { offset }, undefined).catch(() => null);
    if (!page || gen !== this.gen) return;
    if (page.lengthSeconds) this.lengthSeconds = page.lengthSeconds;
    this.ingest(page);
  }

  // pump loads more pages while the buffer horizon is within BUFFER_AHEAD of the clock.
  async pump(clock: number): Promise<void> {
    if (this.fetching || !this.hasNext || !this.cursor) return;
    if (this.horizon >= clock + BUFFER_AHEAD) return;
    const gen = this.gen;
    this.fetching = true;
    const page = await fetchPage(this.vodId, { cursor: this.cursor }, undefined).catch(() => null);
    this.fetching = false;
    if (!page || gen !== this.gen) return;
    this.ingest(page);
  }

  private ingest(page: Page) {
    if (page.lines.length) {
      this.lines = this.lines.concat(page.lines);
      this.horizon = Math.max(this.horizon, page.lines[page.lines.length - 1].offset);
    }
    this.cursor = page.cursor || this.cursor;
    this.hasNext = page.hasNext;
    if (!page.hasNext) this.horizon = Number.POSITIVE_INFINITY;
  }
}

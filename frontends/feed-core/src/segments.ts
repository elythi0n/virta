// A message body is rendered as a list of segments. Parsing happens once per message (off the
// render path); the row just maps segments to DOM. Plain text stays coalesced so a normal message
// is a single text segment.

export type Segment =
  | { type: 'text'; text: string }
  | { type: 'emote'; code: string; url: string }
  | { type: 'mention'; user: string }
  | { type: 'link'; href: string; text: string }
  | { type: 'decoration'; decoType: string; text: string };

export interface EmoteMeta {
  url: string;
}

const LINK = /^https?:\/\/\S+$/;
const MENTION = /^@(\w+)$/;

export function parseSegments(text: string, emotes: Record<string, EmoteMeta> = {}): Segment[] {
  const out: Segment[] = [];
  let buffer = '';
  const flush = () => {
    if (buffer) {
      out.push({ type: 'text', text: buffer });
      buffer = '';
    }
  };

  // Split keeps whitespace runs as their own tokens, so spacing around emotes/mentions/links is
  // preserved in the surrounding text segments.
  for (const token of text.split(/(\s+)/)) {
    if (token === '') continue;
    if (token.trim() === '') {
      buffer += token;
      continue;
    }
    const emote = Object.prototype.hasOwnProperty.call(emotes, token) ? emotes[token] : undefined;
    const mention = MENTION.exec(token);
    if (emote) {
      flush();
      out.push({ type: 'emote', code: token, url: emote.url });
    } else if (LINK.test(token)) {
      flush();
      out.push({ type: 'link', href: token, text: token });
    } else if (mention) {
      flush();
      out.push({ type: 'mention', user: mention[1] });
    } else {
      buffer += token;
    }
  }
  flush();
  return out;
}

/**
 * Applies decorator patterns (contributed by plugins) to text segments, splitting matching
 * runs into decoration segments. Non-text segments pass through unchanged.
 */
export function applyDecorations(
  segments: Segment[],
  patterns: { regex: string; type: string }[],
): Segment[] {
  if (patterns.length === 0) return segments;
  const compiled = patterns.flatMap(p => {
    try { return [{ re: new RegExp(p.regex, 'gi'), type: p.type }]; }
    catch { return []; }
  });
  if (compiled.length === 0) return segments;

  const result: Segment[] = [];
  for (const seg of segments) {
    if (seg.type !== 'text') { result.push(seg); continue; }
    // Start with the text segment; each pattern may split further.
    let pending: Segment[] = [seg];
    for (const { re, type } of compiled) {
      const next: Segment[] = [];
      for (const s of pending) {
        if (s.type !== 'text') { next.push(s); continue; }
        re.lastIndex = 0;
        let last = 0;
        let matched = false;
        let m: RegExpExecArray | null;
        while ((m = re.exec(s.text)) !== null) {
          matched = true;
          if (m.index > last) next.push({ type: 'text', text: s.text.slice(last, m.index) });
          next.push({ type: 'decoration', decoType: type, text: m[0] });
          last = m.index + m[0].length;
        }
        if (!matched) { next.push(s); continue; }
        if (last < s.text.length) next.push({ type: 'text', text: s.text.slice(last) });
      }
      pending = next;
    }
    result.push(...pending);
  }
  return result;
}

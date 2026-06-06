// A message body is rendered as a list of segments. Parsing happens once per message (off the
// render path); the row just maps segments to DOM. Plain text stays coalesced so a normal message
// is a single text segment.

export type Segment =
  | { type: 'text'; text: string }
  | { type: 'emote'; code: string; url: string }
  | { type: 'mention'; user: string }
  | { type: 'link'; href: string; text: string };

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

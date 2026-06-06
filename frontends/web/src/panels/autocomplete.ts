export type Suggestion = { value: string; label: string; image?: string; hint?: string };
export type EmoteEntry = { code: string; url: string; lc: string };

// The slash commands the daemon's send parser understands (mirrors sendHelpText), with arg hints.
export const SLASH_COMMANDS: { name: string; hint: string }[] = [
  { name: '/ban', hint: '<user> [reason]' },
  { name: '/unban', hint: '<user>' },
  { name: '/timeout', hint: '<user> [seconds] [reason]' },
  { name: '/untimeout', hint: '<user>' },
  { name: '/delete', hint: '<message-id>' },
  { name: '/clear', hint: 'clear chat' },
  { name: '/slow', hint: '[seconds]' },
  { name: '/followers', hint: '[minutes]' },
  { name: '/emoteonly', hint: 'emote-only mode' },
  { name: '/uniquechat', hint: 'unique-chat mode' },
  { name: '/me', hint: '<action>' },
  { name: '/help', hint: 'list commands' },
];

// Slash-command suggestions for a leading "/token" (only valid at the start of a message).
export function suggestCommands(token: string, max = 8): Suggestion[] {
  const q = token.toLowerCase();
  return SLASH_COMMANDS.filter((c) => c.name.startsWith(q) && c.name !== q)
    .slice(0, max)
    .map((c) => ({ value: c.name, label: c.name, hint: c.hint }));
}

// The whitespace-delimited word ending at the caret, and where it starts in the text.
export function tokenAt(text: string, caret: number): { token: string; start: number } {
  const upto = text.slice(0, caret);
  const m = /(\S+)$/.exec(upto);
  if (!m) return { token: '', start: caret };
  return { token: m[1], start: caret - m[1].length };
}

// Suggestions for the current token: "@x" → recent chatters; otherwise (≥2 chars) → emotes,
// prefix matches first then substring. Emote codes are pre-lowercased (`lc`) so matching stays
// cheap on a large set. Exact matches are skipped (nothing to complete).
export function suggest(token: string, emotes: EmoteEntry[], chatters: string[], max = 7): Suggestion[] {
  if (token.startsWith('@')) {
    const q = token.slice(1).toLowerCase();
    const out: Suggestion[] = [];
    for (const c of chatters) {
      if (c.toLowerCase().startsWith(q)) out.push({ value: '@' + c, label: c });
      if (out.length >= max) break;
    }
    return out;
  }
  if (token.length < 2) return [];
  const q = token.toLowerCase();
  const prefix: Suggestion[] = [];
  const sub: Suggestion[] = [];
  for (const e of emotes) {
    if (e.lc === q) continue;
    if (e.lc.startsWith(q)) prefix.push({ value: e.code, label: e.code, image: e.url });
    else if (e.lc.includes(q)) sub.push({ value: e.code, label: e.code, image: e.url });
    if (prefix.length >= max) break;
  }
  return prefix.concat(sub).slice(0, max);
}

// Replace the token under the caret with `value` (plus a trailing space); returns the new text and
// where the caret should land.
export function applySuggestion(text: string, caret: number, value: string): { text: string; caret: number } {
  const { start } = tokenAt(text, caret);
  const next = text.slice(0, start) + value + ' ' + text.slice(caret);
  return { text: next, caret: start + value.length + 1 };
}

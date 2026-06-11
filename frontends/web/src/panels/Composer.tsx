import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, Popover, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import Icon from '../Icon';
import { useFeedDisplay } from '../feedDisplay';
import { previewSend, sendMessage, useEmotes } from '../daemon';
import SignInDialog, { type SignInPlatform } from '../shell/SignInDialog';
import type { SendTarget } from '../daemon/wire.gen';
import { applySuggestion, suggest, suggestCommands, tokenAt, type Suggestion } from './autocomplete';
import styles from './Composer.module.css';

// Platforms with a working sign-in flow. Send reachability itself comes from the daemon's
// previewSend (can_send per target); this set only gates the "sign in" offer for unreachable
// targets. YouTube is deliberately absent: it is read-only until its auth/send support ships.
const SIGNABLE = new Set(['twitch', 'kick']);
const platformOf = (channel: string) => channel.split(':')[0];
const label = (channel: string) => channel.split(':')[1] ?? channel;
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const sent = (status: string) => status === 'sent' || status === 'queued';

type Props = {
  /** Channel keys ("platform:slug") this composer can post to. */
  targets: string[];
  /** Recent chatter names, for @mention autocomplete. */
  chatters?: string[];
  /** When set, the composer is in reply mode: the next send goes only to this channel as a reply. */
  replyTo?: { channel: string; parentId: string; author: string } | null;
  onCancelReply?: () => void;
};

// Compose and cross-post to the feed's channels. Target chips show where a message will land:
// reachable ones solid, unreachable ones (platforms you're not signed in to) dimmed with a ⊘; a
// single passive line offers to sign in. After a send, each chip carries its per-target result.
// Typing offers emote/@mention autocomplete.
export default function Composer({ targets, chatters = [], replyTo = null, onCancelReply }: Props) {
  const [text, setText] = useState('');
  const [preview, setPreview] = useState<SendTarget[] | null>([]); // null = daemon unreachable
  const [sending, setSending] = useState(false);
  const [signIn, setSignIn] = useState<SignInPlatform | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const { quickReplies } = useFeedDisplay();
  const [results, setResults] = useState<Record<string, string>>({});
  const [reload, setReload] = useState(0);

  // Autocomplete state.
  const inputRef = useRef<HTMLInputElement>(null);
  const pendingCaret = useRef<number | null>(null);
  const [caret, setCaret] = useState(0);
  const [acIndex, setAcIndex] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [focused, setFocused] = useState(false);
  const emotes = useEmotes();
  const prepared = useMemo(() => emotes.map((e) => ({ code: e.code, url: e.url, lc: e.code.toLowerCase() })), [emotes]);
  const suggestions = useMemo(() => {
    if (dismissed) return [];
    const { token, start } = tokenAt(text, caret);
    // A leading "/word" is a slash command; otherwise emotes / @mentions.
    if (token.startsWith('/') && start === 0) return suggestCommands(token);
    return suggest(token, prepared, chatters);
  }, [dismissed, text, caret, prepared, chatters]);
  const acOpen = focused && suggestions.length > 0;

  const accept = (s: Suggestion) => {
    const next = applySuggestion(text, caret, s.value);
    setText(next.text);
    pendingCaret.current = next.caret;
    setDismissed(false);
    setAcIndex(0);
  };

  // Restore the caret after an accept rewrites the text.
  useLayoutEffect(() => {
    if (pendingCaret.current === null || !inputRef.current) return;
    const pos = pendingCaret.current;
    pendingCaret.current = null;
    inputRef.current.focus();
    inputRef.current.setSelectionRange(pos, pos);
    setCaret(pos);
  }, [text]);
  const key = [...targets].sort().join(',');

  useEffect(() => {
    if (targets.length === 0) {
      setPreview([]);
      return;
    }
    let cancelled = false;
    previewSend(targets)
      .then((t) => !cancelled && setPreview(t))
      .catch(() => !cancelled && setPreview(null));
    return () => {
      cancelled = true;
    };
  }, [key, reload]);

  const reachable = (preview ?? []).filter((t) => t.can_send);
  const unreachable = (preview ?? []).filter((t) => !t.can_send);
  const offline = preview === null;
  const signable = [...new Set(unreachable.map((t) => platformOf(t.channel)).filter((p) => SIGNABLE.has(p)))] as SignInPlatform[];
  const canSend = (replyTo ? true : reachable.length > 0) && text.trim() !== '' && !sending;

  const submit = async () => {
    if (!canSend) return;
    setSending(true);
    setResults({});
    try {
      // In reply mode the message goes only to the parent's channel, carrying the parent id.
      const channels = replyTo ? [replyTo.channel] : reachable.map((r) => r.channel);
      const res = await sendMessage(channels, text.trim(), replyTo?.parentId ?? '');
      setText('');
      setResults(Object.fromEntries(res.map((r) => [r.channel, r.status])));
      if (replyTo) onCancelReply?.();
    } catch {
      // a transient failure; the feed reflects what actually sent
    } finally {
      setSending(false);
    }
  };

  return (
    <div className={styles.composer}>
      {replyTo && (
        <div className={styles.replyBar}>
          <Icon name="reply" size={13} />
          <span className={styles.replyText}>
            Replying to <b>{replyTo.author}</b>
          </span>
          <button type="button" className={styles.replyCancel} aria-label="Cancel reply" onClick={() => onCancelReply?.()}>
            ✕
          </button>
        </div>
      )}
      {preview && preview.length > 0 && (
        <div className={styles.chips}>
          {reachable.map((t) => (
            <span key={t.channel} className={styles.chip}>
              <PlatformGlyph platform={platformOf(t.channel) as Platform} className={styles.chipGlyph} />
              {label(t.channel)}
              {results[t.channel] && (
                <span className={sent(results[t.channel]) ? styles.ok : styles.bad} title={results[t.channel]}>
                  {sent(results[t.channel]) ? '✓' : '✗'}
                </span>
              )}
            </span>
          ))}
          {unreachable.map((t) => (
            <span key={t.channel} className={`${styles.chip} ${styles.off}`} title="Not signed in">
              <span className={styles.block} aria-hidden>
                ⊘
              </span>
              <PlatformGlyph platform={platformOf(t.channel) as Platform} className={styles.chipGlyph} />
              {label(t.channel)}
            </span>
          ))}
        </div>
      )}

      <div className={styles.acWrap}>
        {acOpen && (
          <ul className={styles.popup} role="listbox" aria-label="Suggestions">
            {suggestions.map((s, i) => (
              <li key={s.value}>
                <button
                  type="button"
                  role="option"
                  aria-selected={i === acIndex}
                  className={`${styles.option} ${i === acIndex ? styles.optionOn : ''}`}
                  onMouseDown={(e) => {
                    e.preventDefault(); // keep focus in the input
                    accept(s);
                  }}
                >
                  {s.image && <img className={styles.optionImg} src={s.image} alt="" loading="lazy" />}
                  <span className={styles.optionLabel}>{s.label}</span>
                  {s.hint && <span className={styles.optionHint}>{s.hint}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className={styles.row}>
          <Input
            ref={inputRef}
            placeholder={offline ? 'Not connected' : 'Send a message'}
            aria-label="Message"
            disabled={offline}
            value={text}
            onChange={(e) => {
              setText(e.currentTarget.value);
              setCaret(e.currentTarget.selectionStart ?? e.currentTarget.value.length);
              setDismissed(false);
              setAcIndex(0);
            }}
            onSelect={(e) => setCaret(e.currentTarget.selectionStart ?? 0)}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            onKeyDown={(e) => {
              if (acOpen) {
                if (e.key === 'ArrowDown') {
                  e.preventDefault();
                  setAcIndex((i) => (i + 1) % suggestions.length);
                  return;
                }
                if (e.key === 'ArrowUp') {
                  e.preventDefault();
                  setAcIndex((i) => (i - 1 + suggestions.length) % suggestions.length);
                  return;
                }
                if (e.key === 'Enter' || e.key === 'Tab') {
                  e.preventDefault();
                  accept(suggestions[acIndex]);
                  return;
                }
                if (e.key === 'Escape') {
                  e.preventDefault();
                  setDismissed(true);
                  return;
                }
              }
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                void submit();
              }
            }}
          />
          {quickReplies.length > 0 && (
            <Popover
              open={qrOpen}
              onOpenChange={setQrOpen}
              align="end"
              side="top"
              trigger={
                <button type="button" className={styles.quickBtn} aria-label="Quick replies" disabled={offline}>
                  <Icon name="zap" size={16} />
                </button>
              }
            >
              <div className={styles.signinMenu} role="menu" aria-label="Quick replies">
                {quickReplies.map((q) => (
                  <button
                    key={q}
                    type="button"
                    className={styles.signinItem}
                    onClick={() => {
                      setText(q);
                      pendingCaret.current = q.length;
                      setQrOpen(false);
                    }}
                  >
                    {q}
                  </button>
                ))}
              </div>
            </Popover>
          )}
          <Button variant="solid" size="md" disabled={!canSend} onClick={() => void submit()}>
            Send
          </Button>
        </div>
      </div>

      {signable.length > 0 && (
        <Text variant="meta" tone="subtle" as="p" className={styles.note}>
          Won&rsquo;t reach {signable.map(cap).join(', ')} — not signed in.{' '}
          {signable.length === 1 ? (
            <button type="button" className={styles.link} onClick={() => setSignIn(signable[0])}>
              Sign in
            </button>
          ) : (
            <Popover
              open={menuOpen}
              onOpenChange={setMenuOpen}
              align="start"
              side="top"
              trigger={
                <button type="button" className={styles.link}>
                  Sign in
                </button>
              }
            >
              <div className={styles.signinMenu} role="menu" aria-label="Sign in">
                {signable.map((p) => (
                  <button
                    key={p}
                    type="button"
                    className={styles.signinItem}
                    onClick={() => {
                      setMenuOpen(false);
                      setSignIn(p);
                    }}
                  >
                    Sign in to {cap(p)}
                  </button>
                ))}
              </div>
            </Popover>
          )}
        </Text>
      )}

      <SignInDialog platform={signIn} onClose={() => setSignIn(null)} onAuthorized={() => setReload((r) => r + 1)} />
    </div>
  );
}

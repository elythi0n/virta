import { useEffect, useMemo, useState } from 'react';
import { Input, Select, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import Icon from '../Icon';
import { searchMessages, useChannels } from '../daemon';
import { useOpenChannel } from '../openChannel';
import type { LoggedMessage } from '../daemon/wire.gen';
import styles from './SearchPanel.module.css';

const platformOf = (channel: string) => channel.split(':')[0] as Platform;
const slugOf = (channel: string) => channel.split(':')[1] ?? channel;
const fmtTime = (ms: number) => (ms > 0 ? new Date(ms).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' }) : '');

// Splits a body into [before, hit, after] around the first case-insensitive match of q, so the
// matched span can be emphasized in the result row.
function highlight(body: string, q: string): [string, string, string] {
  const i = body.toLowerCase().indexOf(q.toLowerCase());
  if (q === '' || i < 0) return [body, '', ''];
  return [body.slice(0, i), body.slice(i, i + q.length), body.slice(i + q.length)];
}

// Full-text search across the logged history: a query, optional channel/author filters, and a list
// of matches newest-first. Click a result to open that channel's chat. Logging is opt-in, so this
// is empty until it's enabled in Settings → Storage.
export default function SearchPanel() {
  const { channels } = useChannels();
  const openChannel = useOpenChannel();
  const [q, setQ] = useState('');
  const ALL = '__all__'; // sentinel for "all channels" — Radix Select forbids empty-string values
  const [channel, setChannel] = useState(ALL);
  const [author, setAuthor] = useState('');
  const [results, setResults] = useState<LoggedMessage[]>([]);
  const [status, setStatus] = useState<'idle' | 'searching' | 'done' | 'error'>('idle');

  const channelOptions = useMemo(
    () => [{ value: ALL, label: 'All channels' }, ...channels.map((c) => ({ value: `${c.platform}:${c.slug}`, label: c.slug }))],
    [channels],
  );

  // Debounce so typing doesn't fire a request per keystroke; cancel both the pending timer and any
  // in-flight response when the query changes, so a slow earlier search can't overwrite a newer one.
  useEffect(() => {
    const text = q.trim();
    if (text === '') {
      setResults([]);
      setStatus('idle');
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      setStatus('searching');
      searchMessages({ q: text, channel: channel === ALL ? undefined : channel, author: author.trim() || undefined, limit: 200 })
        .then((rows) => {
          if (cancelled) return;
          setResults(rows);
          setStatus('done');
        })
        .catch(() => {
          if (!cancelled) setStatus('error');
        });
    }, 220);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [q, channel, author]);

  return (
    <div className={styles.panel}>
      <div className={styles.controls}>
        <div className={styles.queryRow}>
          <Icon name="search" size={15} />
          <Input
            aria-label="Search messages"
            placeholder="Search logged messages"
            value={q}
            autoFocus
            onChange={(e) => setQ(e.currentTarget.value)}
          />
        </div>
        <div className={styles.filters}>
          <Select ariaLabel="Channel" value={channel} onValueChange={setChannel} options={channelOptions} />
          <Input aria-label="From author" placeholder="From author (optional)" value={author} onChange={(e) => setAuthor(e.currentTarget.value)} />
        </div>
      </div>

      <div className={styles.results}>
        {status === 'idle' && (
          <Text variant="body" tone="subtle" as="p" className={styles.note}>
            Search your logged chat history. Message logging must be enabled for history to exist; turn it on in your active workspace profile.
          </Text>
        )}
        {status === 'searching' && (
          <Text variant="body" tone="subtle" as="p" className={styles.note}>
            Searching…
          </Text>
        )}
        {status === 'error' && (
          <Text variant="body" tone="subtle" as="p" className={styles.note}>
            Couldn't reach the daemon. Is virtad running?
          </Text>
        )}
        {status === 'done' && results.length === 0 && (
          <Text variant="body" tone="subtle" as="p" className={styles.note}>
            No matches{q.trim() ? ` for "${q.trim()}"` : ''}.
          </Text>
        )}
        {results.length > 0 && (
          <>
            <Text variant="meta" tone="subtle" as="p" className={styles.count}>
              {results.length} match{results.length === 1 ? '' : 'es'}
            </Text>
            <ul className={styles.list}>
              {results.map((m) => {
                const [before, hit, after] = highlight(m.body, q.trim());
                return (
                  <li key={m.id}>
                    <button type="button" className={styles.row} onClick={() => openChannel(m.channel, slugOf(m.channel))} title="Open this channel's chat">
                      <div className={styles.meta}>
                        <PlatformGlyph platform={platformOf(m.channel)} className={styles.glyph} />
                        <span className={styles.channel}>{slugOf(m.channel)}</span>
                        <span className={styles.author}>{m.author}</span>
                        <span className={styles.time}>{fmtTime(m.sent_at_ms)}</span>
                      </div>
                      <div className={`${styles.body} ${m.deleted ? styles.deleted : ''}`}>
                        {before}
                        {hit && <mark className={styles.mark}>{hit}</mark>}
                        {after}
                      </div>
                    </button>
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </div>
    </div>
  );
}

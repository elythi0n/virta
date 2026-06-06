import { useCallback, useMemo, useState } from 'react';
import {
  EVENT_LABEL,
  eventCountLabel,
  isBigEvent,
  isEventType,
  PlatformGlyph,
  type FeedMessage,
  type Platform,
} from '@virta/feed-core';
import { ContextMenu, Dialog, Text } from '@virta/ui-kit';
import { channelKey, useChannels, useDaemonStream } from '../daemon';
import styles from './CelebrationsPane.module.css';

const MAX = 60; // events are rare; a small capped list needs no virtualization

const TYPE_OPTIONS = [
  { value: 'sub', label: 'Subs' },
  { value: 'resub', label: 'Resubs' },
  { value: 'giftsub', label: 'Gifts' },
  { value: 'raid', label: 'Raids' },
  { value: 'host', label: 'Hosts' },
  { value: 'follow', label: 'Follows' },
  { value: 'announcement', label: 'Announcements' },
  { value: 'moderation', label: 'Moderation' },
  { value: 'system', label: 'System' },
];
const ALL_TYPES = TYPE_OPTIONS.map((o) => o.value);

// A filter stored per pane. Empty list means "all" for that dimension.
type Filter = { types: string[]; sources: string[] };
const EMPTY: Filter = { types: [], sources: [] };

const matches = (value: string, list: string[]) => list.length === 0 || list.includes(value);
const checked = (list: string[], value: string) => list.length === 0 || list.includes(value);

// Toggle membership with "empty = all" semantics: unchecking from the all-state makes the set
// explicit; re-checking everything collapses back to empty (all).
function toggle(current: string[], all: string[], value: string): string[] {
  const effective = current.length === 0 ? all : current;
  const next = effective.includes(value) ? effective.filter((x) => x !== value) : [...effective, value];
  return next.length === all.length ? [] : next;
}

const filterKey = (id?: string) => `virta.celeb.filter.${id ?? 'default'}`;
function loadFilter(id?: string): Filter {
  try {
    const raw = localStorage.getItem(filterKey(id));
    if (raw) return { ...EMPTY, ...JSON.parse(raw) };
  } catch {
    // ignore
  }
  return EMPTY;
}

type Props = { panelId?: string };

// A calm shoutout wall: subs, gifts, raids, and announcements across every channel, each arriving
// as a card with a subtle one-shot entrance. Right-click → Filtering to choose which event types
// and which sources this pane shows (saved to the pane), e.g. only your own channel's gifts.
export default function CelebrationsPane({ panelId }: Props) {
  const { channels } = useChannels();
  const [events, setEvents] = useState<FeedMessage[]>([]);
  const [filter, setFilter] = useState<Filter>(() => loadFilter(panelId));
  const [filterOpen, setFilterOpen] = useState(false);

  const onMessage = useCallback((m: FeedMessage) => {
    if (isEventType(m.type)) setEvents((prev) => [m, ...prev].slice(0, MAX));
  }, []);
  useDaemonStream({ onMessage });

  const sourceOptions = channels.map((c) => ({ value: channelKey(c.platform, c.slug), label: c.slug }));
  const allSources = sourceOptions.map((s) => s.value);

  const visible = useMemo(
    () => events.filter((m) => matches(m.type ?? 'system', filter.types) && matches(m.channel ?? '', filter.sources)),
    [events, filter],
  );

  const update = (next: Filter) => {
    setFilter(next);
    try {
      localStorage.setItem(filterKey(panelId), JSON.stringify(next));
    } catch {
      // storage unavailable; the filter just won't persist across reloads
    }
  };

  const filtered = filter.types.length > 0 || filter.sources.length > 0;

  return (
    <>
      <ContextMenu items={[{ kind: 'item', label: 'Filtering…', onSelect: () => setFilterOpen(true) }]} trigger={<div className={styles.root}>
        {events.length === 0 ? (
          <div className={styles.empty}>
            <Text variant="ui" tone="subtle">
              Subs, gifts, raids, and announcements will celebrate here. Right-click to filter.
            </Text>
          </div>
        ) : visible.length === 0 ? (
          <div className={styles.empty}>
            <Text variant="meta" tone="subtle">
              Nothing matches this pane's filter.
            </Text>
          </div>
        ) : (
          <div className={styles.pane}>
            {visible.map((m) => {
              const type = m.type ?? 'system';
              const count = eventCountLabel(m);
              return (
                <article key={m.id} className={styles.card} data-type={type} data-big={isBigEvent(m)}>
                  <span className={styles.glow} aria-hidden />
                  <div className={styles.head}>
                    <PlatformGlyph platform={m.platform as Platform} className={styles.glyph} />
                    <span className={styles.label}>{EVENT_LABEL[type] ?? 'EVENT'}</span>
                    {count && <span className={styles.count}>{count}</span>}
                    <span className={styles.meta}>
                      {m.source?.label ?? ''}
                      {m.source ? ' · ' : ''}
                      {m.ts}
                    </span>
                  </div>
                  <div className={styles.body}>
                    {m.author && <span className={styles.author}>{m.author}</span>}
                    {m.body && <span className={styles.text}>{m.body}</span>}
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </div>} />

      <Dialog
        open={filterOpen}
        onOpenChange={setFilterOpen}
        title="Celebration filters"
        description="Choose which celebrations this pane shows. Saved to this pane."
      >
        <div className={styles.filterBody}>
          <section>
            <Text variant="meta" tone="subtle" as="h3" className={styles.groupLabel}>
              Types
            </Text>
            <div className={styles.chips}>
              {TYPE_OPTIONS.map((o) => (
                <button
                  key={o.value}
                  type="button"
                  className={`${styles.chip} ${checked(filter.types, o.value) ? styles.chipOn : ''}`}
                  aria-pressed={checked(filter.types, o.value)}
                  onClick={() => update({ ...filter, types: toggle(filter.types, ALL_TYPES, o.value) })}
                >
                  {o.label}
                </button>
              ))}
            </div>
          </section>

          <section>
            <Text variant="meta" tone="subtle" as="h3" className={styles.groupLabel}>
              Sources
            </Text>
            {sourceOptions.length === 0 ? (
              <Text variant="meta" tone="subtle">
                No channels joined yet.
              </Text>
            ) : (
              <div className={styles.chips}>
                {sourceOptions.map((o) => (
                  <button
                    key={o.value}
                    type="button"
                    className={`${styles.chip} ${checked(filter.sources, o.value) ? styles.chipOn : ''}`}
                    aria-pressed={checked(filter.sources, o.value)}
                    onClick={() => update({ ...filter, sources: toggle(filter.sources, allSources, o.value) })}
                  >
                    {o.label}
                  </button>
                ))}
              </div>
            )}
          </section>

          {filtered && (
            <button type="button" className={styles.clear} onClick={() => update(EMPTY)}>
              Reset to all
            </button>
          )}
        </div>
      </Dialog>
    </>
  );
}

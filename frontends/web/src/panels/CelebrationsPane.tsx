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
import { Button, ContextMenu, Dialog, Text } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import { channelKey, useChannels, useDaemonStream } from '../daemon';
import styles from './CelebrationsPane.module.css';

const MAX = 60; // events are rare; a small capped list needs no virtualization

// Each celebration type carries an icon; the row tints it to match the event band's colour
// (driven by data-type in CSS), so the filter reads like the cards it controls.
const TYPE_OPTIONS: { value: string; label: string; icon: IconName }[] = [
  { value: 'sub', label: 'Subs', icon: 'star' },
  { value: 'resub', label: 'Resubs', icon: 'star' },
  { value: 'giftsub', label: 'Gifts', icon: 'gift' },
  { value: 'raid', label: 'Raids', icon: 'stream' },
  { value: 'host', label: 'Hosts', icon: 'stream' },
  { value: 'follow', label: 'Follows', icon: 'user-plus' },
  { value: 'announcement', label: 'Announcements', icon: 'megaphone' },
  { value: 'moderation', label: 'Moderation', icon: 'ban' },
  { value: 'system', label: 'System', icon: 'settings' },
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

  const sourceOptions = channels.map((c) => ({ value: channelKey(c.platform, c.slug), label: c.slug, platform: c.platform as Platform }));
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
        footer={
          <>
            {filtered && (
              <Button variant="ghost" size="md" onClick={() => update(EMPTY)}>
                Reset to all
              </Button>
            )}
            <Button variant="solid" size="md" onClick={() => setFilterOpen(false)}>
              Done
            </Button>
          </>
        }
      >
        <div className={styles.filterBody}>
          <section className={styles.group}>
            <Text variant="meta" tone="subtle" as="h3" className={styles.groupLabel}>
              Types
            </Text>
            <div className={styles.typeGrid}>
              {TYPE_OPTIONS.map((o) => {
                const on = checked(filter.types, o.value);
                return (
                  <button
                    key={o.value}
                    type="button"
                    data-type={o.value}
                    className={`${styles.option} ${on ? styles.optionOn : ''}`}
                    aria-pressed={on}
                    onClick={() => update({ ...filter, types: toggle(filter.types, ALL_TYPES, o.value) })}
                  >
                    <span className={styles.optIcon}>
                      <Icon name={o.icon} size={16} />
                    </span>
                    <span className={styles.optLabel}>{o.label}</span>
                    <span className={styles.optCheck}>{on && <Icon name="check" size={14} />}</span>
                  </button>
                );
              })}
            </div>
          </section>

          <section className={styles.group}>
            <Text variant="meta" tone="subtle" as="h3" className={styles.groupLabel}>
              Sources
            </Text>
            {sourceOptions.length === 0 ? (
              <Text variant="meta" tone="subtle">
                No channels joined yet.
              </Text>
            ) : (
              <div className={styles.sourceList}>
                {sourceOptions.map((o) => {
                  const on = checked(filter.sources, o.value);
                  return (
                    <button
                      key={o.value}
                      type="button"
                      className={`${styles.option} ${on ? styles.optionOn : ''}`}
                      aria-pressed={on}
                      onClick={() => update({ ...filter, sources: toggle(filter.sources, allSources, o.value) })}
                    >
                      <span className={styles.optIcon}>
                        <PlatformGlyph platform={o.platform} className={styles.optGlyph} />
                      </span>
                      <span className={styles.optLabel}>{o.label}</span>
                      <span className={styles.optCheck}>{on && <Icon name="check" size={14} />}</span>
                    </button>
                  );
                })}
              </div>
            )}
          </section>
        </div>
      </Dialog>
    </>
  );
}

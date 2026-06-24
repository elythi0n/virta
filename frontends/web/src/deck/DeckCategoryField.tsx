import { useCallback, useEffect, useRef, useState } from 'react';
import { Input, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { searchDeckCategories, type DeckCategory } from '../daemon/deck';
import styles from './DeckCategoryField.module.css';

// A category that exists on one or more platforms, merged by lowercased name so a single user
// selection resolves to per-platform ids in one shot. Box art is Twitch-only; Kick doesn't ship
// thumbnails on its category API.
export interface SelectedCategory {
  name: string;
  perPlatform: Record<string, { id: string; name: string; boxArt?: string }>;
}

interface GroupedCategory {
  key: string;
  name: string;
  perPlatform: SelectedCategory['perPlatform'];
}

type Props = {
  platforms: string[];
  selected: SelectedCategory | null;
  onChange: (c: SelectedCategory | null) => void;
  /** Disable input + interactions (e.g. while syncing). */
  disabled?: boolean;
};

// Cross-platform category typeahead: as the user types, fans out to every signed-in platform's
// category search, merges results by name, and lets the user pick one — which resolves to a per-
// platform id map the backend can use directly without re-searching.
export default function DeckCategoryField({ platforms, selected, onChange, disabled }: Props) {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<GroupedCategory[]>([]);
  const [searching, setSearching] = useState(false);
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  // Close the dropdown when the user clicks outside the field.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  // Debounced parallel search. Each platform is queried independently; failures (e.g. no auth)
  // silently degrade so a single platform's outage doesn't break the typeahead.
  useEffect(() => {
    if (selected || !query.trim() || platforms.length === 0) {
      setResults([]);
      setSearching(false);
      return;
    }
    const q = query.trim();
    let alive = true;
    setSearching(true);
    const t = window.setTimeout(async () => {
      const lists = await Promise.all(
        platforms.map((p) => searchDeckCategories(p, q).catch(() => [] as DeckCategory[])),
      );
      if (!alive) return;
      setResults(mergeResults(lists.flat()));
      setSearching(false);
    }, 250);
    return () => {
      alive = false;
      window.clearTimeout(t);
      setSearching(false);
    };
  }, [query, selected, platforms]);

  const pick = useCallback(
    (g: GroupedCategory) => {
      onChange({ name: g.name, perPlatform: g.perPlatform });
      setQuery('');
      setResults([]);
      setOpen(false);
    },
    [onChange],
  );

  const clear = useCallback(() => {
    onChange(null);
    setQuery('');
    setResults([]);
  }, [onChange]);

  if (selected) {
    const platformChips = Object.keys(selected.perPlatform).sort();
    return (
      <div className={styles.chipRow}>
        <div className={styles.chip}>
          <span className={styles.chipName}>{selected.name}</span>
          <div className={styles.chipPlatforms}>
            {platformChips.map((p) => (
              <span key={p} className={styles.chipPlatform}>{p}</span>
            ))}
          </div>
          <button
            type="button"
            className={styles.chipClear}
            onClick={clear}
            aria-label="Clear category"
            disabled={disabled}
          >
            <Icon name="x" size={12} />
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.wrap} ref={wrapRef}>
      <Input
        placeholder="Search categories…"
        value={query}
        onChange={(e) => {
          setQuery(e.currentTarget.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        disabled={disabled}
      />
      {open && (query.trim() || searching) && (
        <div className={styles.dropdown} role="listbox">
          {searching && results.length === 0 ? (
            <div className={styles.empty}>
              <Text variant="meta" tone="subtle">Searching…</Text>
            </div>
          ) : results.length === 0 ? (
            <div className={styles.empty}>
              <Text variant="meta" tone="subtle">No matches.</Text>
            </div>
          ) : (
            results.map((g) => {
              const firstWithArt = Object.values(g.perPlatform).find((p) => p.boxArt);
              return (
                <button
                  key={g.key}
                  type="button"
                  className={styles.row}
                  onClick={() => pick(g)}
                  role="option"
                >
                  <span className={styles.thumb}>
                    {firstWithArt?.boxArt ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={firstWithArt.boxArt.replace('{width}', '40').replace('{height}', '56')}
                        alt=""
                        loading="lazy"
                      />
                    ) : (
                      <Icon name="grid" size={16} />
                    )}
                  </span>
                  <span className={styles.rowMain}>
                    <span className={styles.rowName}>{g.name}</span>
                    <span className={styles.rowPlatforms}>
                      {Object.keys(g.perPlatform).sort().join(' · ')}
                    </span>
                  </span>
                </button>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

// mergeResults groups categories by lowercased name across platforms — so a "Just Chatting" entry
// returned by both Twitch and Kick becomes one row with both per-platform ids. When two platforms
// disagree on capitalization, the first seen wins for display.
function mergeResults(list: DeckCategory[]): GroupedCategory[] {
  const byKey = new Map<string, GroupedCategory>();
  for (const c of list) {
    const key = c.name.toLowerCase();
    let g = byKey.get(key);
    if (!g) {
      g = { key, name: c.name, perPlatform: {} };
      byKey.set(key, g);
    }
    g.perPlatform[c.platform] = { id: c.id, name: c.name, boxArt: c.box_art };
  }
  // Two-platform matches surface first (they're the safe cross-post picks), then alphabetical.
  return Array.from(byKey.values()).sort((a, b) => {
    const ax = Object.keys(a.perPlatform).length;
    const bx = Object.keys(b.perPlatform).length;
    if (ax !== bx) return bx - ax;
    return a.name.localeCompare(b.name);
  });
}

// Allow callers to build a SelectedCategory from a pre-known display name + id map (e.g. a
// localStorage draft restore or a current-state prefill). Both paths skip the search step.
export function selectedFromDraft(name: string, ids: Record<string, string>): SelectedCategory | null {
  if (!name || Object.keys(ids).length === 0) return null;
  const perPlatform: SelectedCategory['perPlatform'] = {};
  for (const [p, id] of Object.entries(ids)) {
    perPlatform[p] = { id, name };
  }
  return { name, perPlatform };
}

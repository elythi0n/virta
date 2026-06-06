import { useMemo, useState, type ReactNode } from 'react';
import { Input, Segmented, Select, Text, formatShortcut } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import { useActions } from '../actions';
import { useDensity } from '../density';
import { useTheme } from '../theme';
import { DENSITIES } from './DensityControl';
import styles from './Settings.module.css';

type CategoryId =
  | 'appearance'
  | 'connections'
  | 'chat'
  | 'filters'
  | 'notifications'
  | 'shortcuts'
  | 'storage'
  | 'advanced'
  | 'about';

type Category = { id: CategoryId; label: string; keywords: string };

const CATEGORIES: Category[] = [
  { id: 'appearance', label: 'Appearance', keywords: 'theme dark light density font color' },
  { id: 'connections', label: 'Connections', keywords: 'accounts sign in twitch kick channels platform' },
  { id: 'chat', label: 'Chat', keywords: 'feed messages events emotes timestamps' },
  { id: 'filters', label: 'Filters', keywords: 'rules block hide highlight keywords' },
  { id: 'notifications', label: 'Notifications', keywords: 'alerts sounds mentions' },
  { id: 'shortcuts', label: 'Shortcuts', keywords: 'keyboard keymap hotkeys bindings' },
  { id: 'storage', label: 'Storage', keywords: 'database sqlite postgres logging retention' },
  { id: 'advanced', label: 'Advanced', keywords: 'relay daemon address experimental' },
  { id: 'about', label: 'About', keywords: 'version license credits' },
];

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className={styles.field}>
      <div className={styles.fieldText}>
        <Text variant="ui">{label}</Text>
        {hint && (
          <Text variant="meta" tone="subtle">
            {hint}
          </Text>
        )}
      </div>
      <div className={styles.fieldControl}>{children}</div>
    </div>
  );
}

function Placeholder({ children }: { children: ReactNode }) {
  return (
    <Text variant="ui" tone="subtle" as="p">
      {children}
    </Text>
  );
}

function Appearance() {
  const { theme, setTheme } = useTheme();
  const { density, setDensity } = useDensity();
  return (
    <>
      <Field label="Theme" hint="Color palette for the whole app.">
        <Segmented
          ariaLabel="Theme"
          value={theme}
          onValueChange={setTheme}
          options={[
            { value: 'graphite-dark', label: 'Graphite' },
            { value: 'light', label: 'Light' },
          ]}
        />
      </Field>
      <Field label="Default text size" hint="Starting size for new feeds; each chat tab can override it.">
        <Select
          ariaLabel="Default feed text size"
          value={density}
          onValueChange={(v) => setDensity(v as Density)}
          options={DENSITIES.map((d) => ({ value: d.value, label: d.label }))}
        />
      </Field>
    </>
  );
}

function Shortcuts() {
  const actions = useActions();
  const bound = actions.filter((a) => a.shortcut);
  if (bound.length === 0) return <Placeholder>No shortcuts registered.</Placeholder>;
  return (
    <ul className={styles.shortcutList}>
      {bound.map((a) => (
        <li key={a.id} className={styles.shortcutRow}>
          <Text variant="ui">{a.title}</Text>
          <kbd className={styles.kbd}>{formatShortcut(a.shortcut!)}</kbd>
        </li>
      ))}
    </ul>
  );
}

function About() {
  return (
    <>
      <Text variant="title" as="p">
        Virta
      </Text>
      <Placeholder>
        Unified live chat for Twitch, Kick, and X. Your connections and tokens stay on your machine.
      </Placeholder>
    </>
  );
}

function CategoryBody({ id }: { id: CategoryId }) {
  switch (id) {
    case 'appearance':
      return <Appearance />;
    case 'shortcuts':
      return <Shortcuts />;
    case 'about':
      return <About />;
    case 'connections':
      return (
        <Placeholder>
          Sign in to platforms and add channels from the Sources panel. Per-platform connection
          settings land here next.
        </Placeholder>
      );
    case 'storage':
      return (
        <Placeholder>
          The daemon stores data in SQLite by default; switching to Postgres and logging/retention
          controls land here (needs a daemon settings API).
        </Placeholder>
      );
    default:
      return <Placeholder>{CATEGORIES.find((c) => c.id === id)?.label} settings land in a later step.</Placeholder>;
  }
}

export default function Settings() {
  const [query, setQuery] = useState('');
  const [active, setActive] = useState<CategoryId>('appearance');

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return CATEGORIES;
    return CATEGORIES.filter((c) => c.label.toLowerCase().includes(q) || c.keywords.includes(q));
  }, [query]);

  // Keep a valid selection: if the active category is filtered out, show the first match.
  const shown = filtered.some((c) => c.id === active) ? active : filtered[0]?.id;
  const shownLabel = CATEGORIES.find((c) => c.id === shown)?.label ?? '';

  return (
    <div className={styles.settings}>
      <div className={styles.toolbar}>
        <Input
          type="search"
          placeholder="Search settings"
          aria-label="Search settings"
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
        />
      </div>
      <div className={styles.main}>
        <nav className={styles.nav} aria-label="Settings categories">
          {filtered.length === 0 ? (
            <Text variant="meta" tone="subtle" className={styles.navEmpty}>
              No matches
            </Text>
          ) : (
            filtered.map((c) => (
              <button
                key={c.id}
                type="button"
                className={`${styles.navItem} ${shown === c.id ? styles.navActive : ''}`}
                aria-current={shown === c.id}
                onClick={() => setActive(c.id)}
              >
                {c.label}
              </button>
            ))
          )}
        </nav>
        <div className={styles.content}>
          {shown && (
            <>
              <Text as="h2" variant="title" className={styles.heading}>
                {shownLabel}
              </Text>
              <CategoryBody id={shown} />
            </>
          )}
        </div>
      </div>
    </div>
  );
}

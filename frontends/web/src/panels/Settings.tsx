import { useMemo, useState, type ReactNode } from 'react';
import { Input, Segmented, Select, Text, formatShortcut } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import { useA11y } from '../a11y';
import { useActions } from '../actions';
import { useDensity } from '../density';
import { useFeedDisplay } from '../feedDisplay';
import { useTheme, type ThemeMode } from '../theme';
import { DENSITIES } from './DensityControl';
import Connections from './Connections';
import styles from './Settings.module.css';

type CategoryId =
  | 'appearance'
  | 'accessibility'
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
  { id: 'accessibility', label: 'Accessibility', keywords: 'motion reduce dyslexia font contrast screen reader a11y' },
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
  const { mode, setMode } = useTheme();
  const { density, setDensity } = useDensity();
  const { showTimestamps, setShowTimestamps, mentionNames, setMentionNames } = useFeedDisplay();
  return (
    <>
      <Field label="Appearance" hint="Follow the system, or pin light or dark.">
        <Segmented
          ariaLabel="Appearance"
          value={mode}
          onValueChange={(v) => setMode(v as ThemeMode)}
          options={[
            { value: 'system', label: 'System' },
            { value: 'light', label: 'Light' },
            { value: 'dark', label: 'Dark' },
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
      <Field label="Timestamps" hint="Show the time on each message.">
        <Segmented
          ariaLabel="Timestamps"
          value={showTimestamps ? 'on' : 'off'}
          onValueChange={(v) => setShowTimestamps(v === 'on')}
          options={[
            { value: 'on', label: 'On' },
            { value: 'off', label: 'Off' },
          ]}
        />
      </Field>
      <Field label="Highlight my names" hint="Comma-separated. Messages mentioning these collect in the Mentions inbox.">
        <Input
          aria-label="Highlight names"
          placeholder="e.g. yourname, your_handle"
          defaultValue={mentionNames.join(', ')}
          onBlur={(e) => setMentionNames(e.currentTarget.value.split(',').map((s) => s.trim()).filter(Boolean))}
        />
      </Field>
    </>
  );
}

function OnOff({ label, hint, value, onChange }: { label: string; hint: string; value: boolean; onChange: (v: boolean) => void }) {
  return (
    <Field label={label} hint={hint}>
      <Segmented
        ariaLabel={label}
        value={value ? 'on' : 'off'}
        onValueChange={(v) => onChange(v === 'on')}
        options={[
          { value: 'on', label: 'On' },
          { value: 'off', label: 'Off' },
        ]}
      />
    </Field>
  );
}

function Accessibility() {
  const { reduceMotion, setReduceMotion, dyslexicFont, setDyslexicFont } = useA11y();
  return (
    <>
      <OnOff
        label="Reduce motion"
        hint="Turn off animations and transitions across the app, beyond your system setting."
        value={reduceMotion}
        onChange={setReduceMotion}
      />
      <OnOff
        label="Dyslexia-friendly font"
        hint="Use the bundled OpenDyslexic typeface for the interface and chat."
        value={dyslexicFont}
        onChange={setDyslexicFont}
      />
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
    case 'accessibility':
      return <Accessibility />;
    case 'shortcuts':
      return <Shortcuts />;
    case 'about':
      return <About />;
    case 'connections':
      return <Connections />;
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

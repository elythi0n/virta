import { useState } from 'react';
import { Input, Segmented, Select, Text } from '@virta/ui-kit';
import { useTheme } from '../theme';
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

const CATEGORIES: { id: CategoryId; label: string }[] = [
  { id: 'appearance', label: 'Appearance' },
  { id: 'connections', label: 'Connections' },
  { id: 'chat', label: 'Chat' },
  { id: 'filters', label: 'Filters' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'shortcuts', label: 'Shortcuts' },
  { id: 'storage', label: 'Storage' },
  { id: 'advanced', label: 'Advanced' },
  { id: 'about', label: 'About' },
];

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
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

function Appearance() {
  const { theme, setTheme } = useTheme();
  const [density, setDensity] = useState('cozy');
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
      <Field label="Feed density" hint="Row spacing in the message feed.">
        <Select
          ariaLabel="Feed density"
          value={density}
          onValueChange={setDensity}
          options={[
            { value: 'compact', label: 'Compact' },
            { value: 'cozy', label: 'Cozy' },
            { value: 'comfortable', label: 'Comfortable' },
          ]}
        />
      </Field>
    </>
  );
}

export default function Settings() {
  const [active, setActive] = useState<CategoryId>('appearance');
  const activeLabel = CATEGORIES.find((c) => c.id === active)?.label ?? '';

  return (
    <div className={styles.settings}>
      <div className={styles.toolbar}>
        <Input type="search" placeholder="Search settings" aria-label="Search settings" />
      </div>
      <div className={styles.main}>
        <nav className={styles.nav} aria-label="Settings categories">
          {CATEGORIES.map((c) => (
            <button
              key={c.id}
              type="button"
              className={`${styles.navItem} ${active === c.id ? styles.navActive : ''}`}
              aria-current={active === c.id}
              onClick={() => setActive(c.id)}
            >
              {c.label}
            </button>
          ))}
        </nav>
        <div className={styles.content}>
          <Text as="h2" variant="title" className={styles.heading}>
            {activeLabel}
          </Text>
          {active === 'appearance' ? (
            <Appearance />
          ) : (
            <Text variant="ui" tone="subtle">
              {activeLabel} settings land in a later step.
            </Text>
          )}
        </div>
      </div>
    </div>
  );
}

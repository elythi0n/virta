import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Badge, Input, Segmented, Select, Text, formatShortcut } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import { useA11y } from '../a11y';
import { useActions } from '../actions';
import { useDensity } from '../density';
import { useFeedDisplay } from '../feedDisplay';
import { useIntegration, listTokens, mintToken, revokeToken } from '../daemon';
import type { TokenInfo } from '../daemon/wire.gen';
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
  | 'integrations'
  | 'integration'
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
  { id: 'integrations', label: 'Integrations', keywords: 'api tokens scoped token third party stream deck bot webhook' },
  { id: 'integration', label: 'Platform integration', keywords: 'native tray hotkeys notifications sounds wayland os desktop' },
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

const AUTO_CALM_OPTIONS = [
  { value: '0', label: 'Off' },
  { value: '30', label: '30/s' },
  { value: '60', label: '60/s' },
  { value: '100', label: '100/s' },
  { value: '150', label: '150/s' },
];

function ChatSettings() {
  const { showDeleted, setShowDeleted, quickReplies, setQuickReplies, autoCalmRate, setAutoCalmRate } = useFeedDisplay();
  return (
    <>
      <OnOff
        label="Show deleted messages"
        hint="Mod view: keep deleted messages visible (struck through) instead of a tombstone."
        value={showDeleted}
        onChange={setShowDeleted}
      />
      <Field label="Auto calm mode" hint="Engage calm mode automatically when a feed's combined rate passes this many messages per second.">
        <Select
          ariaLabel="Auto calm mode threshold"
          value={String(autoCalmRate)}
          onValueChange={(v) => setAutoCalmRate(Number(v))}
          options={AUTO_CALM_OPTIONS}
        />
      </Field>
      <Field label="Quick replies" hint="One canned message per line; offered in the composer's ⚡ menu.">
        <textarea
          className={styles.textarea}
          aria-label="Quick replies"
          rows={4}
          placeholder={'gg!\nThanks for the follow'}
          defaultValue={quickReplies.join('\n')}
          onBlur={(e) => setQuickReplies(e.currentTarget.value.split('\n').map((s) => s.trim()).filter(Boolean))}
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

// User-facing copy for each feature and the rung it's running on (the machine code → language
// mapping lives here per the disclosure rules; the raw code stays visible as a quiet tag). Falls
// back to the bare code for any rung not spelled out, so a new rung never renders blank.
const FEATURE_LABEL: Record<string, string> = {
  window: 'Window',
  theme: 'System theme',
  quicklaunch: 'Profile quick-launch',
  hotkeys: 'Global hotkeys',
  notifications: 'Notifications',
  tray: 'System tray',
  sounds: 'Alert sounds',
};
const RUNG: Record<string, { label: string; tone: 'ok' | 'warn' | 'neutral' }> = {
  'window:native': { label: 'Native window', tone: 'ok' },
  'window:browser': { label: 'Browser tab', tone: 'neutral' },
  'theme:native': { label: 'Follows the system', tone: 'ok' },
  'quicklaunch:in_app': { label: 'In-app switcher (Ctrl+K)', tone: 'ok' },
  'hotkeys:native': { label: 'Native global hotkeys', tone: 'ok' },
  'hotkeys:portal': { label: 'Via desktop portal', tone: 'ok' },
  'hotkeys:in_app': { label: 'In-app shortcuts only', tone: 'neutral' },
  'notifications:native': { label: 'Native notifications', tone: 'ok' },
  'notifications:in_app': { label: 'In-app banner', tone: 'neutral' },
  'tray:native': { label: 'Native tray', tone: 'ok' },
  'tray:none': { label: 'Not available · close-to-quit', tone: 'warn' },
  'sounds:native': { label: 'Native sound', tone: 'ok' },
  'sounds:visual': { label: 'Visual flash only', tone: 'warn' },
};
const DETAIL_NOTE: Record<string, string> = {
  wayland_restricted: 'Wayland restricts global hotkeys',
  wayland_no_self_position: 'Wayland places windows; size is remembered',
  not_implemented: 'native path not wired yet',
  browser: 'running in a browser',
};

const ALL_SCOPES = ['read', 'send', 'moderate', 'control', 'admin'] as const;
const SCOPE_DESC: Record<string, string> = {
  read: 'Read the feed, history, stats, capabilities, and profiles',
  send: 'Send messages via connected accounts',
  moderate: 'Moderation actions where the account is permitted',
  control: 'Switch profiles, join/leave channels, read settings',
  admin: 'Settings write, token management, sign-in flows',
};

function Integrations() {
  const [tokens, setTokens] = useState<TokenInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [newName, setNewName] = useState('');
  const [newScopes, setNewScopes] = useState<string[]>(['read']);
  const [minted, setMinted] = useState<{ token: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reload = () => {
    setLoading(true);
    listTokens().then(t => { setTokens(t ?? []); setLoading(false); }).catch(() => setLoading(false));
  };
  useEffect(() => { reload(); }, []);

  const mint = async () => {
    if (!newName.trim() || newScopes.length === 0) return;
    setError(null);
    try {
      const m = await mintToken(newName.trim(), newScopes);
      setMinted({ token: m.token });
      setNewName('');
      setNewScopes(['read']);
      reload();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const revoke = async (id: string) => {
    await revokeToken(id).catch(() => {});
    reload();
  };

  const toggleScope = (sc: string) =>
    setNewScopes(s => s.includes(sc) ? s.filter(x => x !== sc) : [...s, sc]);

  return (
    <>
      <Text variant="meta" tone="subtle" as="p" className={styles.introNote}>
        Mint scoped tokens for integrations (Stream Deck, bots, OBS). Tokens are shown once at creation — revoke and re-mint if lost. The full API is at{' '}
        <a href="/docs" target="_blank" rel="noreferrer" className={styles.link}>/docs</a>.
      </Text>

      <Field label="Existing tokens">
        {loading ? (
          <Text variant="meta" tone="subtle">Loading…</Text>
        ) : tokens.length === 0 ? (
          <Text variant="meta" tone="subtle">No tokens minted yet.</Text>
        ) : (
          <div className={styles.tokenList}>
            {tokens.map(t => (
              <div key={t.id} className={styles.tokenRow}>
                <div className={styles.tokenMeta}>
                  <span className={styles.tokenName}>{t.name}</span>
                  <span className={styles.tokenScopes}>{t.scopes.join(', ')}</span>
                  {t.last_used_at_ms ? (
                    <span className={styles.tokenUsed}>last used {new Date(t.last_used_at_ms).toLocaleDateString()}</span>
                  ) : (
                    <span className={styles.tokenUsed}>never used</span>
                  )}
                </div>
                <button type="button" className={styles.revokeBtn} onClick={() => void revoke(t.id)}>
                  Revoke
                </button>
              </div>
            ))}
          </div>
        )}
      </Field>

      <Field label="Mint new token">
        <div className={styles.mintForm}>
          <Input
            aria-label="Token name"
            placeholder="Name (e.g. Stream Deck)"
            value={newName}
            onChange={e => setNewName(e.currentTarget.value)}
          />
          <div className={styles.scopeGrid}>
            {ALL_SCOPES.map(sc => (
              <label key={sc} className={`${styles.scopeChip} ${newScopes.includes(sc) ? styles.scopeOn : ''}`}>
                <input
                  type="checkbox"
                  className={styles.srOnly}
                  checked={newScopes.includes(sc)}
                  onChange={() => toggleScope(sc)}
                />
                <span className={styles.scopeLabel}>{sc}</span>
                <span className={styles.scopeHint}>{SCOPE_DESC[sc]}</span>
              </label>
            ))}
          </div>
          {error && <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>{error}</Text>}
          <button
            type="button"
            className={styles.mintBtn}
            disabled={!newName.trim() || newScopes.length === 0}
            onClick={() => void mint()}
          >
            Mint token
          </button>
        </div>
      </Field>

      {minted && (
        <Field label="Token created — copy it now">
          <div className={styles.mintedBox}>
            <code className={styles.mintedToken}>{minted.token}</code>
            <Text variant="meta" tone="subtle" as="p">
              This is shown once. If you lose it, revoke and mint a new one.
            </Text>
            <button type="button" className={styles.mintBtn} onClick={() => setMinted(null)}>
              Done
            </button>
          </div>
        </Field>
      )}
    </>
  );
}

function PlatformIntegration() {
  const report = useIntegration();
  return (
    <>
      <Text variant="meta" tone="subtle" as="p" className={styles.introNote}>
        How Virta integrates with this {report.os === 'web' ? 'browser' : report.os}
        {report.session ? ` (${report.session})` : ''}. The lowest rung is always a working in-app
        fallback, never a missing feature.
      </Text>
      {report.features.map((f) => {
        const copy = RUNG[`${f.id}:${f.rung}`] ?? { label: f.rung, tone: 'neutral' as const };
        const note = f.detail ? DETAIL_NOTE[f.detail] ?? f.detail : '';
        return (
          <Field key={f.id} label={FEATURE_LABEL[f.id] ?? f.id} hint={note}>
            <span className={styles.integrationRung}>
              <Badge tone={copy.tone}>{copy.label}</Badge>
              <code className={styles.rawCode}>
                {f.rung}
                {f.detail ? `·${f.detail}` : ''}
              </code>
            </span>
          </Field>
        );
      })}
    </>
  );
}

function CategoryBody({ id }: { id: CategoryId }) {
  switch (id) {
    case 'appearance':
      return <Appearance />;
    case 'accessibility':
      return <Accessibility />;
    case 'chat':
      return <ChatSettings />;
    case 'shortcuts':
      return <Shortcuts />;
    case 'integrations':
      return <Integrations />;
    case 'integration':
      return <PlatformIntegration />;
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

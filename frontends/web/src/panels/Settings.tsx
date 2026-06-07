import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Badge, Input, Segmented, Select, Text, formatShortcut } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import { useA11y } from '../a11y';
import { useActions } from '../actions';
import { useDensity } from '../density';
import { useFeedDisplay } from '../feedDisplay';
import { useIntegration, listTokens, mintToken, revokeToken, getIntelConfig, setIntelConfig, listModels } from '../daemon';
import type { TokenInfo } from '../daemon/wire.gen';
import { useTheme, type ThemeMode } from '../theme';
import Icon from '../Icon';
import { DENSITIES } from './DensityControl';
import Connections from './Connections';
import ThemeEditor from './ThemeEditor';
import WebhooksPanel from './WebhooksPanel';
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
  | 'intelligence'
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
  { id: 'notifications', label: 'Notifications & Webhooks', keywords: 'alerts sounds mentions webhooks outbound delivery hmac' },
  { id: 'shortcuts', label: 'Shortcuts', keywords: 'keyboard keymap hotkeys bindings' },
  { id: 'intelligence', label: 'Intelligence', keywords: 'ai llm ask anthropic openai ollama search translation provider budget' },
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
      <Field label="Themes" hint="Import a .vtheme file or paste a vtheme1: share string to add a custom theme.">
        <ThemeEditor currentTheme={mode === 'dark' ? 'graphite-dark' : 'light'} onApply={() => {}} />
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
    listTokens()
      .then(t => { setTokens(t ?? []); setLoading(false); })
      .catch((e: unknown) => {
        setError(e instanceof Error ? e.message : 'Could not load tokens');
        setLoading(false);
      });
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

type IntelCfgState = { enabled?: boolean; selected_model?: string; daily_limit_usd?: number; provider_keys?: Record<string, string> };

// Providers the user can connect. Ollama is always auto-detected (no key); the rest need an API key.
const PROVIDERS = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    placeholder: 'sk-ant-api03-…',
    hint: 'Get your key at console.anthropic.com',
    // Anthropic logo mark (simplified wordmark "A")
    logo: (
      <svg viewBox="0 0 24 24" fill="currentColor" width={18} height={18} aria-hidden>
        <path d="M13.83 2h-3.66L3 22h3.57l1.43-4.1h8l1.43 4.1H21L13.83 2zm-4.72 12.9 2.89-8.29 2.89 8.29H9.11z"/>
      </svg>
    ),
  },
  {
    id: 'openai',
    name: 'OpenAI',
    placeholder: 'sk-proj-…',
    hint: 'Get your key at platform.openai.com',
    logo: (
      <svg viewBox="0 0 24 24" fill="currentColor" width={18} height={18} aria-hidden>
        <path d="M22.28 9.93a5.8 5.8 0 0 0-.5-4.76 5.9 5.9 0 0 0-6.35-2.83 5.84 5.84 0 0 0-9.94 1.6 5.82 5.82 0 0 0-3.87 2.82 5.9 5.9 0 0 0 .73 6.9 5.8 5.8 0 0 0 .5 4.77 5.9 5.9 0 0 0 6.35 2.82 5.81 5.81 0 0 0 4.38 1.96 5.9 5.9 0 0 0 5.62-4.1 5.82 5.82 0 0 0 3.87-2.81 5.9 5.9 0 0 0-.79-6.37zm-8.8 12.34a4.37 4.37 0 0 1-2.81-1.02l.14-.08 4.66-2.7a.76.76 0 0 0 .39-.67v-6.58l1.97 1.14a.07.07 0 0 1 .04.05v5.46a4.4 4.4 0 0 1-4.39 4.4zm-9.44-4.04a4.37 4.37 0 0 1-.52-2.95l.14.08 4.67 2.7a.77.77 0 0 0 .77 0l5.7-3.3V16.4a.07.07 0 0 1-.03.06l-4.72 2.72a4.4 4.4 0 0 1-6.01-1.95zm-1.23-10.2a4.36 4.36 0 0 1 2.29-1.92v5.57a.77.77 0 0 0 .38.67l5.68 3.28-1.97 1.14a.07.07 0 0 1-.07 0L4.66 13.6a4.4 4.4 0 0 1-.85-5.57zm16.17 3.78-5.7-3.3 1.98-1.13a.07.07 0 0 1 .07 0l4.72 2.72a4.39 4.39 0 0 1-.68 7.93v-5.57a.76.76 0 0 0-.39-.65zm1.96-2.97-.14-.08-4.65-2.72a.77.77 0 0 0-.78 0L10.67 9.3V7.6a.07.07 0 0 1 .03-.06l4.72-2.72a4.4 4.4 0 0 1 6.52 4.55v.47zm-12.34 4.06-1.97-1.14a.07.07 0 0 1-.04-.05V5.25a4.4 4.4 0 0 1 7.21-3.38l-.14.08-4.66 2.7a.76.76 0 0 0-.38.66l-.02 6.59zm1.07-2.31 2.54-1.46 2.54 1.46v2.93l-2.54 1.46-2.54-1.46V10.6z"/>
      </svg>
    ),
  },
  {
    id: 'xai',
    name: 'xAI (Grok)',
    placeholder: 'xai-…',
    hint: 'Get your key at console.x.ai',
    logo: (
      <svg viewBox="0 0 24 24" fill="currentColor" width={18} height={18} aria-hidden>
        <path d="M4 4l16 16M20 4L4 20"/>
      </svg>
    ),
  },
  {
    id: 'ollama',
    name: 'Ollama',
    placeholder: 'http://localhost:11434',
    hint: 'No key needed — just the base URL of your running Ollama instance',
    isUrl: true,
    logo: (
      <svg viewBox="0 0 24 24" fill="currentColor" width={18} height={18} aria-hidden>
        <circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M2 12h3M19 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12"/>
      </svg>
    ),
  },
] as const;

function ProviderKeys({ keys, onSave }: { keys: Record<string, string>; onSave: (k: Record<string, string>) => void }) {
  const [showKey, setShowKey] = useState<Record<string, boolean>>({});
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  const commit = (id: string, val: string) => {
    const trimmed = val.trim();
    if (trimmed === '') return; // blank = keep existing; clearing requires explicit delete
    onSave({ ...keys, [id]: trimmed });
    setDrafts(d => { const n = { ...d }; delete n[id]; return n; });
  };

  const clear = (id: string) => {
    const next = { ...keys };
    delete next[id];
    onSave(next);
  };

  return (
    <Field label="AI providers" hint="Keys are stored in the OS keychain; never in the database. Leave a field blank to keep the existing key.">
      <div className={styles.providerGrid}>
        {PROVIDERS.map(p => {
          const masked = keys[p.id];
          const draft = drafts[p.id] ?? '';
          const connected = !!masked;
          return (
            <div key={p.id} className={`${styles.providerCard} ${connected ? styles.providerOn : ''}`}>
              <div className={styles.providerHead}>
                <span className={styles.providerLogo} data-provider={p.id}>
                  {p.logo}
                </span>
                <span className={styles.providerName}>{p.name}</span>
                {connected && (
                  <>
                    <span className={styles.connectedPill}>Connected</span>
                    <button
                      type="button"
                      className={styles.clearKeyBtn}
                      aria-label={`Disconnect ${p.name}`}
                      onClick={() => clear(p.id)}
                    >
                      ×
                    </button>
                  </>
                )}
              </div>
              <div className={styles.providerInputRow}>
                <Input
                  aria-label={`${p.name} ${'isUrl' in p ? 'base URL' : 'API key'}`}
                  type={('isUrl' in p) || showKey[p.id] ? 'text' : 'password'}
                  placeholder={connected ? '•••• (already set — paste to replace)' : p.placeholder}
                  value={draft}
                  onChange={e => setDrafts(d => ({ ...d, [p.id]: e.currentTarget.value }))}
                  onBlur={e => commit(p.id, e.currentTarget.value)}
                  onKeyDown={e => e.key === 'Enter' && commit(p.id, (e.target as HTMLInputElement).value)}
                />
                {!('isUrl' in p) && (
                  <button
                    type="button"
                    className={styles.showHideBtn}
                    aria-label={showKey[p.id] ? 'Hide key' : 'Show key'}
                    onClick={() => setShowKey(s => ({ ...s, [p.id]: !s[p.id] }))}
                  >
                    <Icon name={showKey[p.id] ? 'eye-off' : 'eye'} size={14} />
                  </button>
                )}
              </div>
              <Text variant="meta" tone="subtle" as="p" className={styles.providerHint}>{p.hint}</Text>
            </div>
          );
        })}
      </div>
    </Field>
  );
}

function IntelligenceSettings() {
  const [cfg, setCfg] = useState<IntelCfgState | null>(null);
  const [models, setModels] = useState<{ provider_id: string; display_name: string; models: { id: string; display_name: string; supports_tools: boolean }[] }[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveErr, setSaveErr] = useState('');
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    getIntelConfig()
      .then(c => setCfg(c as never))
      .catch(() => setUnavailable(true));
    listModels().then(setModels as never).catch(() => {});
  }, []);

  // Merge patch over current state using a functional updater so we always write the latest cfg.
  const save = async (patch: Partial<IntelCfgState>) => {
    setSaving(true);
    setSaveErr('');
    let next: IntelCfgState = {};
    setCfg(c => { next = { ...c, ...patch }; return next; });
    try {
      await setIntelConfig(next as never);
    } catch (e: unknown) {
      setSaveErr(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  if (unavailable) {
    return (
      <Placeholder>
        Could not load Intelligence settings. Make sure virtad is running and try reopening this panel.
      </Placeholder>
    );
  }

  // Show a loading skeleton until the config arrives.
  if (cfg === null) {
    return <Text variant="meta" tone="subtle">Loading…</Text>;
  }

  return (
    <>
      <Text variant="meta" tone="subtle" as="p" className={styles.introNote}>
        AI features are opt-in, off by default. When enabled, only configured providers receive data;
        local-only mode (Ollama) involves no external calls at all.
      </Text>
      <OnOff
        label="Enable AI features"
        hint="Master kill switch — off means zero external AI calls regardless of other settings."
        value={!!(cfg?.enabled)}
        onChange={v => void save({ enabled: v })}
      />
      {cfg?.enabled && (
        <>
          <ProviderKeys keys={cfg?.provider_keys ?? {}} onSave={keys => void save({ provider_keys: keys })} />
          {models.length > 0 && (
            <Field label="Default model" hint="Saved per-profile. Tool-incapable models are disabled.">
              <Select
                ariaLabel="Default model"
                value={cfg?.selected_model || (models[0]?.models[0]?.id ?? '')}
                onValueChange={v => void save({ selected_model: v })}
                options={models.flatMap(g => g.models.map(m => ({ value: m.id, label: `${g.display_name} — ${m.display_name}` })))}
              />
            </Field>
          )}
          <Field label="Daily budget (USD)" hint="0 = no limit. Stops new calls when the day's spend reaches this amount.">
            <Input
              aria-label="Daily budget"
              type="number"
              defaultValue={String(cfg?.daily_limit_usd ?? 0)}
              onBlur={(e) => void save({ daily_limit_usd: Number(e.currentTarget.value) })}
            />
          </Field>
        </>
      )}
      {saving && <Text variant="meta" tone="subtle">Saving…</Text>}
      {saveErr && <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>{saveErr}</Text>}
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
    case 'notifications':
      return <WebhooksPanel />;
    case 'intelligence':
      return <IntelligenceSettings />;
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
      return <Placeholder>{CATEGORIES.find((c) => c.id === id)?.label} settings are coming soon.</Placeholder>;
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

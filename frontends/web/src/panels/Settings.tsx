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
import { useHostedAuth } from '../daemon/hostedAuth';
import Icon from '../Icon';
import ProviderIcon from '../ProviderIcon';
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

// Field: side-by-side label + compact control (toggles, selects, badges).
// Pass `stack` when the control needs full width (text inputs, grids).
function Field({ label, hint, children, stack }: { label: string; hint?: string; children: ReactNode; stack?: boolean }) {
  return (
    <div className={`${styles.field} ${stack ? styles.fieldStack : ''}`}>
      <div className={styles.fieldText}>
        <Text variant="ui" className={styles.fieldLabel}>{label}</Text>
        {hint && <Text variant="meta" tone="subtle" className={styles.fieldHint}>{hint}</Text>}
      </div>
      <div className={styles.fieldControl}>{children}</div>
    </div>
  );
}

// Section: a labelled block for full-width content (provider cards, token lists, etc.)
// that must not be constrained to the side-by-side field max-width.
function Section({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className={styles.section}>
      <div className={styles.sectionHead}>
        <Text variant="ui" className={styles.fieldLabel}>{label}</Text>
        {hint && <Text variant="meta" tone="subtle" className={styles.fieldHint}>{hint}</Text>}
      </div>
      {children}
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
      <Field label="Highlight my names" hint="Comma-separated. Messages mentioning these collect in the Mentions inbox." stack>
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
  const [version, setVersion] = useState<string | null>(null);
  useEffect(() => {
    fetch('/v1/health')
      .then(r => r.json())
      .then((d: { version?: string }) => setVersion(d.version ?? null))
      .catch(() => {});
  }, []);

  return (
    <div className={styles.aboutBlock}>
      <div className={styles.aboutHeader}>
        <img src="/favicon-96x96.png" alt="Virta" width={48} height={48} className={styles.aboutLogo} />
        <div>
          <Text variant="title" as="p" className={styles.aboutName}>Virta</Text>
          {version && <Text variant="meta" tone="subtle" className={styles.aboutVersion}>{version}</Text>}
        </div>
      </div>
      <Text variant="body" tone="subtle" as="p" className={styles.aboutDesc}>
        Virta is a unified live chat dashboard for streamers — Twitch, Kick, and X in one feed.
        Mentions inbox, filter rules, AutoMod queue, OBS overlays, search over chat history,
        and an AI assistant that can answer questions about your community.
      </Text>
      <div className={styles.aboutLinks}>
        <a className={styles.aboutLink} href="https://virta.lol" target="_blank" rel="noopener noreferrer">virta.lol</a>
        <span className={styles.aboutSep}>·</span>
        <a className={styles.aboutLink} href="https://github.com/elythi0n/virta" target="_blank" rel="noopener noreferrer">GitHub</a>
      </div>
    </div>
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
      <Text variant="body" tone="subtle" as="p" className={styles.introNote}>
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
                  <span className={styles.tokenScopes}>{(t.scopes ?? []).join(', ')}</span>
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

// Providers the user can connect. Ollama needs only a base URL; the rest need an API key.
const PROVIDERS = [
  { id: 'anthropic',   name: 'Anthropic',   placeholder: 'sk-ant-api03-…',          hint: 'Get your key at console.anthropic.com' },
  { id: 'openai',      name: 'OpenAI',      placeholder: 'sk-proj-…',               hint: 'Get your key at platform.openai.com' },
  { id: 'xai',         name: 'xAI / Grok',  placeholder: 'xai-…',                   hint: 'Get your key at console.x.ai' },
  { id: 'openrouter',  name: 'OpenRouter',  placeholder: 'sk-or-…',                 hint: 'Route to any model at openrouter.ai. Get your key at openrouter.ai/keys' },
  { id: 'ollama',      name: 'Ollama',      placeholder: 'http://localhost:11434',   hint: 'No key needed — /v1 is added automatically. Docker Compose: http://ollama:11434. Native: http://localhost:11434.', isUrl: true },
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
    <Section label="AI providers" hint="Keys are stored securely on the server — never shown again after saving. Leave a field blank to keep the existing value.">
      <div className={styles.providerGrid}>
        {PROVIDERS.map(p => {
          const saved = keys[p.id];
          const isUrl = 'isUrl' in p;
          // URLs come back unmasked so we can show them directly in the input.
          // For API keys the saved value is a masked token; we show a placeholder instead.
          const draft = drafts[p.id] ?? (isUrl && saved ? saved : '');
          const connected = !!saved;
          return (
            <div key={p.id} className={`${styles.providerCard} ${connected ? styles.providerOn : ''}`}>
              <div className={styles.providerHead}>
                <span className={styles.providerLogo} data-provider={p.id}>
                  <ProviderIcon provider={p.id} size={18} />
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
                  aria-label={`${p.name} ${isUrl ? 'base URL' : 'API key'}`}
                  type={isUrl || showKey[p.id] ? 'text' : 'password'}
                  placeholder={connected && !isUrl ? '•••• (already set — paste to replace)' : p.placeholder}
                  value={draft}
                  onChange={e => { const v = e.currentTarget.value; setDrafts(d => ({ ...d, [p.id]: v })); }}
                  onBlur={e => commit(p.id, e.currentTarget.value)}
                  onKeyDown={e => e.key === 'Enter' && commit(p.id, (e.currentTarget as HTMLInputElement).value)}
                />
                {!isUrl && (
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
    </Section>
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
    // Merge the patch into a stable snapshot before the async call so concurrent saves
    // never silently overwrite each other. We read current cfg synchronously here rather
    // than inside a setState updater so `next` is always the true merged value.
    const next: IntelCfgState = { ...cfg, ...patch } as IntelCfgState;
    setCfg(next);
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
      <Text variant="body" tone="subtle" as="p" className={styles.introNote}>
        AI features are opt-in and off by default. Only the providers you configure receive data.
        Ollama runs locally with no external calls.
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
          {/* Default model — always shown when AI is enabled; placeholder until models load. */}
          <Field label="Default model" hint="Used when no model is explicitly chosen. Add a provider above to populate this list.">
            {models.length > 0 ? (
              <Select
                ariaLabel="Default model"
                value={cfg?.selected_model || '__auto__'}
                onValueChange={v => void save({ selected_model: v === '__auto__' ? '' : v })}
                options={[
                  { value: '__auto__', label: 'Auto (first available)' },
                  ...models.flatMap(g => (g.models ?? []).map(m => ({
                    value: m.id,
                    label: `${g.display_name} — ${m.display_name}`,
                  }))),
                ]}
              />
            ) : (
              <Text variant="meta" tone="subtle">No models available — configure a provider above.</Text>
            )}
          </Field>
          <Field label="Daily budget (USD)" hint="0 = no limit. Stops new calls when the day's spend reaches this amount." stack>
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
      <Text variant="body" tone="subtle" as="p" className={styles.introNote}>
        How Virta integrates with this {report.os === 'web' ? 'browser' : report.os}
        {report.session ? ` (${report.session})` : ''}. The lowest rung is always a working in-app
        fallback, never a missing feature.
      </Text>
      {(report.features ?? []).map((f) => {
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

// Categories hidden in hosted mode: the server manages storage; users cannot configure it.
const HOSTED_HIDDEN: Set<CategoryId> = new Set(['storage']);

export default function Settings() {
  const [query, setQuery] = useState('');
  const [active, setActive] = useState<CategoryId>('appearance');
  const { hosted } = useHostedAuth();

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const base = hosted ? CATEGORIES.filter(c => !HOSTED_HIDDEN.has(c.id)) : CATEGORIES;
    if (!q) return base;
    return base.filter((c) => c.label.toLowerCase().includes(q) || c.keywords.includes(q));
  }, [query, hosted]);

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
            <Text variant="ui" tone="subtle" className={styles.navEmpty}>
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

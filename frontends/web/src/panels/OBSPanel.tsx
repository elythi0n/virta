import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Button, Segmented, Select, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, discover } from '../daemon';
import { buildOverlayUrl, type OverlayConfig, type OverlayKind, type OverlayTheme, type OverlayDensity, type OverlayAlign } from '../overlay/overlayConfig';
import styles from './OBSPanel.module.css';

// Map platform ids to the Icon names that exist in the icon set.
const PLATFORM_ICON: Record<string, 'x' | 'chat' | 'stream' | 'stream'> = {
  twitch: 'stream',
  kick: 'stream',
  x: 'x',
};

// Overlay presets — pre-configured bundles for common streaming use-cases.
// Each maps to a partial OverlayConfig; the user can still customise after selecting one.
const PRESETS = [
  {
    id: 'chat-minimal',
    name: 'Clean chat',
    description: 'Transparent, comfortable size. Classic look.',
    icon: 'chat',
    cfg: { kind: 'chat' as OverlayKind, theme: 'transparent' as OverlayTheme, density: 'comfortable' as OverlayDensity, textShadow: true, maxMessages: 60 },
  },
  {
    id: 'chat-frosted',
    name: 'Frosted chat',
    description: 'Semi-transparent dark glass panel.',
    icon: 'chat',
    cfg: { kind: 'chat' as OverlayKind, theme: 'frosted-dark' as OverlayTheme, density: 'cozy' as OverlayDensity, textShadow: false, maxMessages: 80 },
  },
  {
    id: 'events',
    name: 'Events only',
    description: 'Subs, raids, gifts — no chat noise.',
    icon: 'gift',
    cfg: { kind: 'events' as OverlayKind, theme: 'transparent' as OverlayTheme, density: 'comfortable' as OverlayDensity, textShadow: true, maxMessages: 20 },
  },
  {
    id: 'chat-fade',
    name: 'Fading chat',
    description: 'Messages fade out after 8 s — minimal screen clutter.',
    icon: 'chat',
    cfg: { kind: 'chat' as OverlayKind, theme: 'transparent' as OverlayTheme, density: 'comfortable' as OverlayDensity, textShadow: true, maxMessages: 30, fadeMs: 8000 },
  },
  {
    id: 'compact-left',
    name: 'Compact left rail',
    description: 'Compact chat docked to the left edge.',
    icon: 'chat',
    cfg: { kind: 'chat' as OverlayKind, theme: 'frosted-dark' as OverlayTheme, density: 'compact' as OverlayDensity, align: 'left' as OverlayAlign, width: 360, textShadow: false, maxMessages: 100 },
  },
  {
    id: 'celebrations',
    name: 'Celebrations',
    description: 'Sub/raid/gift events with animation — great in a corner.',
    icon: 'gift',
    cfg: { kind: 'celebrations' as OverlayKind, theme: 'transparent' as OverlayTheme, density: 'comfortable' as OverlayDensity, textShadow: true, maxMessages: 10 },
  },
] as const;

const THEME_LABELS: Record<OverlayTheme, string> = {
  'transparent': 'Transparent',
  'frosted-dark': 'Frosted dark',
  'frosted-light': 'Frosted light',
  'solid-dark': 'Solid dark',
  'solid-light': 'Solid light',
};
const KIND_LABELS: Record<OverlayKind, string> = {
  chat: 'Chat messages',
  mentions: 'Mentions',
  celebrations: 'Events & celebrations',
  events: 'Events only (no chat)',
};
const DENSITY_LABELS: Record<OverlayDensity, string> = {
  compact: 'Compact', cozy: 'Cozy', comfortable: 'Comfortable', large: 'Large',
};

type Draft = Partial<Omit<OverlayConfig, 'token' | 'channels'>>;

export default function OBSPanel() {
  const { channels } = useChannels();
  const [token, setToken] = useState('');
  const [baseUrl, setBaseUrl] = useState('');
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [activePreset, setActivePreset] = useState<string>('chat-minimal');
  const [draft, setDraft] = useState<Draft>({});
  const [copied, setCopied] = useState<string | null>(null);

  // Discover the daemon address and token.
  useEffect(() => {
    discover().then(d => {
      if (!d) return;
      setToken(d.token);
      const base = d.addr ? `http://${d.addr}` : location.origin;
      setBaseUrl(base);
    }).catch(() => {});
  }, []);

  const preset = useMemo(() => PRESETS.find(p => p.id === activePreset) ?? PRESETS[0], [activePreset]);
  const effectiveCfg = useMemo((): Draft & { kind: OverlayKind } => ({
    ...preset.cfg,
    ...draft,
  }), [preset.cfg, draft]);

  const overlayUrl = useMemo(() => {
    if (!token || !baseUrl) return '';
    return buildOverlayUrl(baseUrl, {
      ...effectiveCfg,
      token,
      channels: selectedChannels,
    });
  }, [effectiveCfg, token, baseUrl, selectedChannels]);

  // Pre-compute all gallery URLs once per meaningful change so the gallery map never rebuilds URLs inline.
  const galleryUrls = useMemo(() =>
    PRESETS.map(p => baseUrl && token ? buildOverlayUrl(baseUrl, { ...p.cfg, token, channels: selectedChannels }) : ''),
    [baseUrl, token, selectedChannels],
  );

  // Store the active copied-state reset timer so we can cancel it on unmount or rapid re-clicks.
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (copyTimer.current) clearTimeout(copyTimer.current); }, []);

  const copyUrl = useCallback((id: string, url: string) => {
    const doCopy = () => {
      setCopied(id);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(null), 2000);
    };
    // navigator.clipboard is only available in secure contexts (HTTPS or localhost).
    // Provide a graceful fallback via execCommand for HTTP local-network access.
    if (navigator.clipboard) {
      navigator.clipboard.writeText(url).then(doCopy).catch(() => {
        try { document.execCommand?.('copy'); doCopy(); } catch { /* no feedback available */ }
      });
    } else {
      try {
        const ta = document.createElement('textarea');
        ta.value = url;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        doCopy();
      } catch { /* clipboard unavailable */ }
    }
  }, []);

  const openPreview = useCallback((url: string) => {
    window.open(url, '_blank', 'width=480,height=720,toolbar=no,menubar=no');
  }, []);



  const patch = (p: Draft) => setDraft(d => ({ ...d, ...p }));

  return (
    <div className={styles.panel}>
      {/* Header */}
      <header className={styles.head}>
        <Icon name="stream" size={20} />
        <div>
          <Text variant="title" as="h2" className={styles.title}>OBS Overlays</Text>
          <Text variant="body" tone="subtle" as="p" className={styles.lead}>
            Drop any overlay into OBS as a Browser Source. Pick a preset, configure it, copy the URL.
          </Text>
        </div>
      </header>

      {!token && (
        <div className={styles.noToken}>
          <Icon name="settings" size={16} />
          <Text variant="body" tone="subtle">
            Daemon not reachable. Run <code>make serve</code> first.
          </Text>
        </div>
      )}

      {token && (
        <>
          {/* Channel selector */}
          <section className={styles.section}>
            <Text variant="heading" tone="subtle" as="h3" className={styles.sectionLabel}>Channels</Text>
            <div className={styles.channelPills}>
              <button
                type="button"
                className={`${styles.channelPill} ${selectedChannels.length === 0 ? styles.pillOn : ''}`}
                onClick={() => setSelectedChannels([])}
              >
                All channels
              </button>
              {channels.map(c => {
                const key = `${c.platform}:${c.slug}`;
                const on = selectedChannels.includes(key);
                return (
                  <button
                    key={key}
                    type="button"
                    className={`${styles.channelPill} ${on ? styles.pillOn : ''}`}
                    onClick={() => setSelectedChannels(prev =>
                      on ? prev.filter(k => k !== key) : [...prev, key]
                    )}
                  >
                    <Icon name={PLATFORM_ICON[c.platform] ?? 'stream'} size={12} />
                    {c.slug}
                  </button>
                );
              })}
            </div>
          </section>

          {/* Presets */}
          <section className={styles.section}>
            <Text variant="heading" tone="subtle" as="h3" className={styles.sectionLabel}>Preset</Text>
            <div className={styles.presetGrid}>
              {PRESETS.map(p => (
                <button
                  key={p.id}
                  type="button"
                  className={`${styles.presetCard} ${activePreset === p.id ? styles.presetOn : ''}`}
                  onClick={() => { setActivePreset(p.id); setDraft({}); }}
                >
                  <Icon name={p.icon as 'chat' | 'gift'} size={16} />
                  <span className={styles.presetName}>{p.name}</span>
                  <span className={styles.presetDesc}>{p.description}</span>
                </button>
              ))}
            </div>
          </section>

          {/* Custom settings */}
          <section className={styles.section}>
            <Text variant="heading" tone="subtle" as="h3" className={styles.sectionLabel}>
              Customise
              {Object.keys(draft).length > 0 && (
                <button type="button" className={styles.resetBtn} onClick={() => setDraft({})}>
                  Reset to preset
                </button>
              )}
            </Text>
            <div className={styles.customGrid}>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Content</span>
                <Select
                  ariaLabel="Overlay content"
                  value={effectiveCfg.kind ?? 'chat'}
                  onValueChange={v => patch({ kind: v as OverlayKind })}
                  options={Object.entries(KIND_LABELS).map(([v, l]) => ({ value: v, label: l }))}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Theme</span>
                <Select
                  ariaLabel="Overlay theme"
                  value={effectiveCfg.theme ?? 'transparent'}
                  onValueChange={v => patch({ theme: v as OverlayTheme })}
                  options={Object.entries(THEME_LABELS).map(([v, l]) => ({ value: v, label: l }))}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Density</span>
                <Select
                  ariaLabel="Text density"
                  value={effectiveCfg.density ?? 'comfortable'}
                  onValueChange={v => patch({ density: v as OverlayDensity })}
                  options={Object.entries(DENSITY_LABELS).map(([v, l]) => ({ value: v, label: l }))}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Align</span>
                <Segmented
                  ariaLabel="Message alignment"
                  value={effectiveCfg.align ?? 'bottom'}
                  onValueChange={v => patch({ align: v as OverlayAlign })}
                  options={[
                    { value: 'top', label: '↑ Top' },
                    { value: 'bottom', label: '↓ Bottom' },
                    { value: 'left', label: '← Left' },
                    { value: 'right', label: '→ Right' },
                  ]}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Text shadow</span>
                <Segmented
                  ariaLabel="Text shadow"
                  value={effectiveCfg.textShadow !== false ? 'on' : 'off'}
                  onValueChange={v => patch({ textShadow: v === 'on' })}
                  options={[{ value: 'on', label: 'On' }, { value: 'off', label: 'Off' }]}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Timestamps</span>
                <Segmented
                  ariaLabel="Show timestamps"
                  value={effectiveCfg.showTimestamps ? 'on' : 'off'}
                  onValueChange={v => patch({ showTimestamps: v === 'on' })}
                  options={[{ value: 'on', label: 'On' }, { value: 'off', label: 'Off' }]}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Fade after</span>
                <Select
                  ariaLabel="Message fade"
                  value={String(effectiveCfg.fadeMs ?? 0)}
                  onValueChange={v => patch({ fadeMs: parseInt(v, 10) })}
                  options={[
                    { value: '0', label: 'Never' },
                    { value: '5000', label: '5 seconds' },
                    { value: '8000', label: '8 seconds' },
                    { value: '15000', label: '15 seconds' },
                    { value: '30000', label: '30 seconds' },
                  ]}
                />
              </label>
              <label className={styles.customField}>
                <span className={styles.fieldLabel}>Max messages</span>
                <Select
                  ariaLabel="Max messages"
                  value={String(effectiveCfg.maxMessages ?? 80)}
                  onValueChange={v => patch({ maxMessages: parseInt(v, 10) })}
                  options={[
                    { value: '20', label: '20' },
                    { value: '40', label: '40' },
                    { value: '80', label: '80' },
                    { value: '150', label: '150' },
                  ]}
                />
              </label>
            </div>
          </section>

          {/* Generated URL + OBS instructions */}
          <section className={styles.section}>
            <Text variant="heading" tone="subtle" as="h3" className={styles.sectionLabel}>Browser Source URL</Text>
            <div className={styles.urlBox}>
              <code className={styles.urlText}>{overlayUrl}</code>
              <div className={styles.urlActions}>
                <Tooltip content={copied === 'main' ? 'Copied!' : 'Copy URL'} side="top">
                  <Button variant="solid" size="sm" onClick={() => copyUrl('main', overlayUrl)}>
                    <Icon name={copied === 'main' ? 'check' : 'popout'} size={14} />
                    {copied === 'main' ? 'Copied' : 'Copy'}
                  </Button>
                </Tooltip>
                <Tooltip content="Open preview window" side="top">
                  <Button variant="ghost" size="sm" onClick={() => openPreview(overlayUrl)}>
                    <Icon name="stream" size={14} /> Preview
                  </Button>
                </Tooltip>
              </div>
            </div>

            <div className={styles.obsInstructions}>
              <Text variant="heading" as="h4" className={styles.instructionTitle}>
                How to add in OBS
              </Text>
              <ol className={styles.steps}>
                <li><strong>Add Source</strong> → <strong>Browser</strong></li>
                <li>Paste the URL above</li>
                <li>Set Width/Height to match your canvas (e.g. 400 × 900 for a side strip)</li>
                <li>Check <strong>"Allow transparency"</strong> for transparent themes</li>
                <li>Optional: <strong>Custom CSS</strong> to tune position/margin</li>
              </ol>
            </div>
          </section>

          {/* Quick-add gallery — all presets as one-click copy */}
          <section className={styles.section}>
            <Text variant="heading" tone="subtle" as="h3" className={styles.sectionLabel}>
              Quick-add all presets
            </Text>
            <Text variant="body" tone="subtle" as="p" className={styles.galleryNote}>
              One-click copy for every preset using your current channel selection.
            </Text>
            <div className={styles.gallery}>
              {galleryUrls.map((url, idx) => {
                const p = PRESETS[idx];
                const id = `gallery-${p.id}`;
                return (
                  <div key={p.id} className={styles.galleryCard}>
                    <div className={styles.galleryCardHead}>
                      <Icon name={p.icon as 'chat' | 'gift'} size={16} />
                      <span className={styles.galleryName}>{p.name}</span>
                      <Badge tone="neutral">{THEME_LABELS[p.cfg.theme as OverlayTheme]}</Badge>
                    </div>
                    <Text variant="ui" tone="subtle" as="p">{p.description}</Text>
                    <div className={styles.galleryActions}>
                      <Button variant="ghost" size="sm" onClick={() => copyUrl(id, url)}>
                        <Icon name={copied === id ? 'check' : 'popout'} size={13} />
                        {copied === id ? 'Copied!' : 'Copy URL'}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => openPreview(url)}>
                        Preview
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        </>
      )}
    </div>
  );
}

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, StatusDot, Text, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { discover, useChannels } from '../daemon';
import {
  detectOBS, getOBSConfig, getOBSSources, getOBSStatus, setOBSConfig,
  type DataMapping, type OBSConfig, type OBSSourceInfo, type OBSStatus,
} from '../daemon/obsws';
import { buildOverlayUrl, type OverlayKind, type OverlayTheme } from '../overlay/overlayConfig';
import styles from './OBSPanel.module.css';

// ---- Constants -------------------------------------------------------------

type ContentType = { kind: OverlayKind; panelId?: undefined } | { kind?: undefined; panelId: string };

const CONTENT_OPTIONS: { label: string; value: string; sub: string; ct: ContentType }[] = [
  { label: 'All chat',      value: 'chat',         sub: 'Every message from all channels',  ct: { kind: 'chat' } },
  { label: 'Mentions only', value: 'mentions',      sub: 'Messages that mention you',        ct: { kind: 'mentions' } },
  { label: 'Subs & raids',  value: 'celebrations', sub: 'Sub / gift / raid events',          ct: { kind: 'celebrations' } },
  { label: 'Event feed',    value: 'events',        sub: 'All events including system msgs',  ct: { kind: 'events' } },
  { label: 'Stats panel',   value: 'panel:stats',   sub: 'Live viewer and chat stats',        ct: { panelId: 'stats' } },
  { label: 'Markets',       value: 'panel:markets', sub: 'Crypto market ticker',              ct: { panelId: 'markets' } },
];

const THEME_OPTIONS: { label: string; value: OverlayTheme; sub: string }[] = [
  { label: 'Transparent', value: 'transparent', sub: 'Enable "Allow transparency" in OBS Browser Source' },
  { label: 'Dark glass',  value: 'frosted-dark', sub: 'Semi-transparent dark blur'                       },
  { label: 'Dark solid',  value: 'solid-dark',   sub: 'Opaque; use with Window Capture'                  },
  { label: 'Light solid', value: 'solid-light',  sub: 'For light-coloured scenes'                        },
];

const FADE_OPTIONS = [
  { label: 'Off (messages stay)',  ms: 0     },
  { label: 'Fade after 10 s',     ms: 10000 },
  { label: 'Fade after 20 s',     ms: 20000 },
  { label: 'Fade after 30 s',     ms: 30000 },
];

// ---- Helpers ---------------------------------------------------------------

type Tab = 'overlay' | 'control';

function statusToDot(state: OBSStatus['state']): DotStatus {
  if (state === 'connected') return 'live';
  if (state === 'error') return 'offline';
  return 'idle';
}

function statusLabel(s: OBSStatus): string {
  if (s.state === 'connected') return `OBS ${s.obs_version ?? 'connected'}`;
  if (s.state === 'connecting') return 'Connecting…';
  if (s.state === 'error') return s.error ?? 'Error';
  return 'Not connected';
}

function copyText(text: string): Promise<void> {
  if (navigator.clipboard) return navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
  return Promise.resolve(fallbackCopy(text));
}
function fallbackCopy(text: string) {
  try {
    const ta = document.createElement('textarea');
    ta.value = text; ta.style.cssText = 'position:fixed;opacity:0;pointer-events:none';
    document.body.appendChild(ta); ta.select(); document.execCommand('copy');
    document.body.removeChild(ta);
  } catch { /* nothing */ }
}

// ---- Overlay tab -----------------------------------------------------------

function OverlayTab() {
  const { channels: joined } = useChannels();
  const [token, setToken] = useState('');
  const [baseUrl, setBaseUrl] = useState('');
  const [contentValue, setContentValue] = useState('chat');
  const [allChannels, setAllChannels] = useState(true);
  const [selectedChannels, setSelectedChannels] = useState<Set<string>>(new Set());
  const [theme, setTheme] = useState<OverlayTheme>('transparent');
  const [fade, setFade] = useState(0);
  const [width, setWidth] = useState('400');
  const [height, setHeight] = useState('600');
  const [copied, setCopied] = useState(false);
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    discover().then(d => {
      if (!d) return;
      setToken(d.token);
      setBaseUrl(d.addr ? `http://${d.addr}` : location.origin);
    }).catch(() => {});
    return () => { if (copyTimer.current) clearTimeout(copyTimer.current); };
  }, []);

  const toggleChannel = (key: string) => {
    setSelectedChannels(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const ct = CONTENT_OPTIONS.find(o => o.value === contentValue)?.ct ?? { kind: 'chat' as OverlayKind };
  const channelList = allChannels ? [] : [...selectedChannels];

  const overlayUrl = useMemo(() => {
    if (!token || !baseUrl) return '';
    return buildOverlayUrl(baseUrl, {
      kind: ct.kind ?? 'chat',
      panelId: ct.panelId,
      token,
      channels: channelList,
      theme,
      fadeMs: fade,
      transparent: theme === 'transparent',
      width: parseInt(width, 10) || 0,
      height: parseInt(height, 10) || 0,
    });
  }, [token, baseUrl, ct, channelList, theme, fade, width, height]);

  const handleCopy = useCallback(() => {
    if (!overlayUrl) return;
    void copyText(overlayUrl).then(() => {
      setCopied(true);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(false), 2000);
    });
  }, [overlayUrl]);

  const panelSlug = ct.panelId ?? ct.kind ?? 'feed';
  const tokenPreview = token ? token.slice(0, 8) + '...' : '<token>';
  const virta_cmd = `virta-overlay --panel ${panelSlug} --port 8344 --token ${tokenPreview} --width ${width || 400} --height ${height || 600}`;

  return (
    <div className={styles.tabBody}>

      {/* What to show */}
      <div className={styles.section}>
        <div className={styles.sectionLabel}>What to show</div>
        <div className={styles.optionGrid}>
          {CONTENT_OPTIONS.map(o => (
            <button
              key={o.value}
              type="button"
              className={styles.optionCard}
              data-selected={contentValue === o.value}
              onClick={() => setContentValue(o.value)}
            >
              <span className={styles.optionName}>{o.label}</span>
              <span className={styles.optionSub}>{o.sub}</span>
            </button>
          ))}
        </div>
      </div>

      {/* Channel filter */}
      <div className={styles.section}>
        <div className={styles.sectionLabel}>Channels</div>
        <label className={styles.checkLabel}>
          <input
            type="checkbox"
            className={styles.checkbox}
            checked={allChannels}
            onChange={e => setAllChannels(e.currentTarget.checked)}
          />
          <span>All channels</span>
        </label>
        {!allChannels && (
          <div className={styles.channelList}>
            {joined.length === 0 ? (
              <Text variant="meta" tone="subtle" as="p" className={styles.hintText}>No channels joined yet.</Text>
            ) : joined.map(ch => {
              const key = `${ch.platform}:${ch.slug}`;
              return (
                <label key={key} className={styles.checkLabel}>
                  <input
                    type="checkbox"
                    className={styles.checkbox}
                    checked={selectedChannels.has(key)}
                    onChange={() => toggleChannel(key)}
                  />
                  <span className={styles.channelKey}>{key}</span>
                </label>
              );
            })}
          </div>
        )}
      </div>

      {/* Appearance */}
      <div className={styles.section}>
        <div className={styles.sectionLabel}>Appearance</div>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>Background</span>
          <select
            className={styles.nativeSelect}
            aria-label="Theme"
            value={theme}
            onChange={e => setTheme(e.currentTarget.value as OverlayTheme)}
          >
            {THEME_OPTIONS.map(t => (
              <option key={t.value} value={t.value}>{t.label}</option>
            ))}
          </select>
        </div>
        <Text variant="meta" tone="subtle" as="p" className={styles.hintText}>
          {THEME_OPTIONS.find(t => t.value === theme)?.sub}
        </Text>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>Fade out</span>
          <select
            className={styles.nativeSelect}
            aria-label="Fade"
            value={fade}
            onChange={e => setFade(parseInt(e.currentTarget.value, 10))}
          >
            {FADE_OPTIONS.map(f => (
              <option key={f.ms} value={f.ms}>{f.label}</option>
            ))}
          </select>
        </div>
      </div>

      {/* Size */}
      <div className={styles.section}>
        <div className={styles.sectionLabel}>Size</div>
        <div className={styles.sizeRow}>
          <div className={styles.fieldRow}>
            <span className={styles.fieldLabel}>Width</span>
            <Input aria-label="Width" type="number" value={width} onChange={e => setWidth(e.currentTarget.value)} placeholder="400" />
          </div>
          <div className={styles.fieldRow}>
            <span className={styles.fieldLabel}>Height</span>
            <Input aria-label="Height" type="number" value={height} onChange={e => setHeight(e.currentTarget.value)} placeholder="600" />
          </div>
        </div>
      </div>

      {/* URL + copy */}
      {overlayUrl ? (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Overlay URL</div>
          <div className={styles.urlBox}>
            <code className={styles.urlText}>{overlayUrl}</code>
          </div>
          <div className={styles.actionRow}>
            <Button variant="solid" size="sm" onClick={handleCopy}>
              <Icon name={copied ? 'check' : 'popout'} size={13} />
              {copied ? 'Copied' : 'Copy URL'}
            </Button>
          </div>
        </div>
      ) : (
        <div className={styles.noticeBox}>
          <Text variant="meta" tone="subtle">Daemon not reachable. Run the server first.</Text>
        </div>
      )}

      {/* How to add */}
      <div className={styles.howToBlock}>
        <div className={styles.howToTitle}>How to add in OBS</div>

        <div className={styles.howToOption}>
          <div className={styles.howToOptionLabel}>
            <span className={styles.howToNum}>A</span>
            Browser Source
            <span className={styles.howToTag}>recommended</span>
          </div>
          <ol className={styles.steps}>
            <li>In OBS: <strong>Sources +</strong> then <strong>Browser</strong></li>
            <li>Paste the URL above. Set width/height to match.</li>
            {theme === 'transparent' && <li>Tick <strong>Allow transparency</strong> in the source settings.</li>}
          </ol>
        </div>

        <div className={styles.howToOption}>
          <div className={styles.howToOptionLabel}>
            <span className={styles.howToNum}>B</span>
            virta-overlay
            <span className={styles.howToTag}>no Browser Source / CEF needed</span>
          </div>
          <div className={styles.codeBlock}>
            <code>{virta_cmd}</code>
          </div>
          <Text variant="meta" tone="subtle" as="p" className={styles.hintText}>
            Then in OBS: <strong>Sources +</strong> → <strong>Window Capture (Xcomposite)</strong> → pick <em>virta-overlay</em>.
          </Text>
        </div>

        <Text variant="meta" tone="subtle" as="p" className={styles.tokenNote}>
          The URL contains a read token. Anyone with it can read this feed.
        </Text>
      </div>
    </div>
  );
}

// ---- OBS Control tab -------------------------------------------------------

function ControlTab({
  status,
  onStatusChange,
}: {
  status: OBSStatus;
  onStatusChange: () => void;
}) {
  const [host, setHost] = useState('localhost');
  const [port, setPort] = useState('4455');
  const [password, setPassword] = useState('');
  const [clearPw, setClearPw] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [detectResult, setDetectResult] = useState<string | null>(null);
  const [detecting, setDetecting] = useState(false);
  const [configLoaded, setConfigLoaded] = useState(false);
  const [mappings, setMappings] = useState<DataMapping[]>([]);
  const [updateInterval, setUpdateInterval] = useState('5');
  const [sources, setSources] = useState<OBSSourceInfo[]>([]);
  const [dataSaving, setDataSaving] = useState(false);
  const [dataError, setDataError] = useState('');

  const connected = status.state === 'connected';

  useEffect(() => {
    getOBSConfig().then(r => {
      setHost(r.config.host || 'localhost');
      setPort(String(r.config.port || 4455));
      setEnabled(r.config.enabled);
      setMappings(r.config.data_mappings ?? []);
      setUpdateInterval(String(r.config.update_interval_s ?? 5));
      setConfigLoaded(true);
    }).catch(() => { setConfigLoaded(true); });
  }, []);

  useEffect(() => {
    if (connected) {
      getOBSSources().then(r => setSources(r.sources ?? [])).catch(() => {});
    }
  }, [connected]);

  const handleSave = async () => {
    setSaving(true); setSaveError('');
    try {
      const r = await getOBSConfig();
      const cfg: OBSConfig = {
        ...r.config,
        enabled,
        host: host.trim() || 'localhost',
        port: parseInt(port, 10) || 4455,
        has_password: false,
      };
      await setOBSConfig(cfg, clearPw ? 'CLEAR' : (password || undefined));
      setPassword(''); setClearPw(false);
      onStatusChange();
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const handleDetect = async () => {
    setDetecting(true); setDetectResult(null);
    try {
      const r = await detectOBS();
      setDetectResult(r.detected ? 'OBS found on localhost:4455.' : 'OBS not found on localhost:4455.');
    } catch {
      setDetectResult('Detection failed. Is OBS running?');
    } finally {
      setDetecting(false);
    }
  };

  const handleSaveData = async () => {
    setDataSaving(true); setDataError('');
    try {
      const r = await getOBSConfig();
      await setOBSConfig({ ...r.config, data_mappings: mappings, update_interval_s: parseInt(updateInterval, 10) || 5 });
    } catch (e: unknown) {
      setDataError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setDataSaving(false);
    }
  };

  if (!configLoaded) {
    return <div className={styles.tabBody}><Text variant="meta" tone="subtle">Loading…</Text></div>;
  }

  return (
    <div className={styles.tabBody}>

      {/* Purpose callout */}
      <div className={styles.purposeBox}>
        <Text variant="meta" as="p" className={styles.purposeText}>
          Optional. Connects to OBS over obs-websocket v5 to push live stats into OBS text sources and trigger scene switches from chat events. You do not need this to get the chat overlay into OBS — use the Overlay tab for that.
        </Text>
      </div>

      {/* Status */}
      <div className={styles.statusRow}>
        <StatusDot status={statusToDot(status.state)} />
        <Text variant="ui" className={styles.statusText}>{statusLabel(status)}</Text>
        {status.state === 'connected' && status.websocket_version && (
          <Text variant="meta" tone="subtle">WS {status.websocket_version}</Text>
        )}
      </div>

      <label className={styles.checkLabel}>
        <input type="checkbox" className={styles.checkbox} checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} />
        <span>Enable OBS WebSocket integration</span>
      </label>

      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Host</span>
        <Input aria-label="OBS host" value={host} onChange={e => setHost(e.currentTarget.value)} placeholder="localhost" />
      </div>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Port</span>
        <Input aria-label="OBS port" type="number" value={port} onChange={e => setPort(e.currentTarget.value)} placeholder="4455" />
      </div>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Password</span>
        <Input
          aria-label="OBS WebSocket password"
          type="password"
          value={password}
          onChange={e => { setPassword(e.currentTarget.value); setClearPw(false); }}
          placeholder={clearPw ? '' : '••••••••'}
        />
      </div>
      <div className={styles.fieldHint}>
        Leave empty to keep existing.{' '}
        <button type="button" className={styles.linkBtn} onClick={() => { setPassword(''); setClearPw(true); }}>
          Clear password
        </button>
      </div>

      <div className={styles.actionRow}>
        <Button variant="solid" size="sm" onClick={() => void handleSave()} disabled={saving}>
          {saving ? 'Saving…' : 'Save and connect'}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => void handleDetect()} disabled={detecting}>
          <Icon name="search" size={13} />
          {detecting ? 'Detecting…' : 'Detect OBS'}
        </Button>
      </div>
      {detectResult && <Text variant="meta" tone="subtle" as="p" className={styles.inlineNote}>{detectResult}</Text>}
      {saveError && <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>{saveError}</Text>}

      {/* Stats in text sources */}
      <div className={styles.controlDivider}>
        <span className={styles.controlDividerLabel}>Push stats to OBS text sources</span>
      </div>

      {!connected && (
        <div className={styles.noticeBox}>
          <Icon name="stream" size={14} />
          <Text variant="meta" tone="subtle">Connect to OBS first to configure stat mappings.</Text>
        </div>
      )}

      {connected && (
        <Text variant="meta" tone="subtle" as="p" className={styles.hintText}>
          Create a Text (GDI+/FreeType) source in OBS. Enter its name and pick which stat to write there.
        </Text>
      )}

      <div className={styles.mappingList}>
        {mappings.map((m, i) => (
          <div key={i} className={styles.mappingRow}>
            <select
              className={styles.nativeSelect}
              aria-label="Stat"
              value={m.stat}
              disabled={!connected}
              onChange={e => setMappings(ms => ms.map((r, idx) => idx === i ? { ...r, stat: e.currentTarget.value as DataMapping['stat'] } : r))}
            >
              <option value="msgs_per_min">Msgs / min</option>
              <option value="unique_chatters">Unique chatters</option>
            </select>
            <SourceInput
              value={m.source_name}
              sources={sources}
              disabled={!connected}
              onChange={v => setMappings(ms => ms.map((r, idx) => idx === i ? { ...r, source_name: v } : r))}
            />
            <button type="button" className={styles.removeBtn} aria-label="Remove" onClick={() => setMappings(ms => ms.filter((_, idx) => idx !== i))}>
              <Icon name="x" size={12} />
            </button>
          </div>
        ))}
      </div>

      <button
        type="button"
        className={styles.addRowBtn}
        disabled={!connected}
        onClick={() => setMappings(m => [...m, { stat: 'msgs_per_min', source_name: '' }])}
      >
        <Icon name="plus" size={13} />
        Add stat mapping
      </button>

      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Update every</span>
        <div className={styles.intervalRow}>
          <Input aria-label="Interval" type="number" value={updateInterval} onChange={e => setUpdateInterval(e.currentTarget.value)} placeholder="5" />
          <Text variant="meta" tone="subtle">seconds</Text>
        </div>
      </div>

      {(mappings.length > 0 || connected) && (
        <div className={styles.actionRow}>
          <Button variant="solid" size="sm" onClick={() => void handleSaveData()} disabled={dataSaving || !connected}>
            {dataSaving ? 'Saving…' : 'Save mappings'}
          </Button>
        </div>
      )}
      {dataError && <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>{dataError}</Text>}
    </div>
  );
}

// Source name input with datalist autocomplete.
function SourceInput({
  value, sources, disabled, onChange,
}: {
  value: string;
  sources: OBSSourceInfo[];
  disabled?: boolean;
  onChange: (v: string) => void;
}) {
  const listId = useRef(`sl-${Math.random().toString(36).slice(2)}`).current;
  return (
    <div className={styles.sourceInputWrap}>
      <input
        className={styles.sourceInput}
        type="text"
        list={listId}
        value={value}
        disabled={disabled}
        onChange={e => onChange(e.currentTarget.value)}
        placeholder="OBS source name"
        aria-label="OBS text source name"
      />
      {sources.length > 0 && (
        <datalist id={listId}>
          {sources.filter(s => s.kind.toLowerCase().includes('text')).map(s => (
            <option key={s.name} value={s.name} />
          ))}
        </datalist>
      )}
    </div>
  );
}

// ---- Root ------------------------------------------------------------------

export default function OBSPanel() {
  const [tab, setTab] = useState<Tab>('overlay');
  const [status, setStatus] = useState<OBSStatus>({ state: 'disconnected' });

  const refreshStatus = useCallback(() => {
    getOBSStatus().then(setStatus).catch(() => {});
  }, []);

  useEffect(() => {
    refreshStatus();
    const id = setInterval(refreshStatus, 5000);
    return () => clearInterval(id);
  }, [refreshStatus]);

  return (
    <div className={styles.panel}>
      <div className={styles.tabBar} role="tablist" aria-label="OBS integration">
        {([
          { id: 'overlay' as Tab, label: 'Overlay' },
          { id: 'control' as Tab, label: 'OBS Control' },
        ]).map(t => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            className={`${styles.tabBtn} ${tab === t.id ? styles.tabActive : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
            {t.id === 'control' && (
              <span className={`${styles.tabDot} ${styles[`tabDot-${statusToDot(status.state)}`]}`} aria-hidden />
            )}
          </button>
        ))}
      </div>
      <div className={styles.tabContent} role="tabpanel">
        {tab === 'overlay' && <OverlayTab />}
        {tab === 'control' && <ControlTab status={status} onStatusChange={refreshStatus} />}
      </div>
    </div>
  );
}

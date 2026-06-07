import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, StatusDot, Text, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { discover } from '../daemon';
import {
  getOBSStatus, getOBSConfig, setOBSConfig, getOBSSources, detectOBS,
  type OBSConfig, type OBSStatus, type OBSSourceInfo, type DataMapping,
} from '../daemon/obsws';
import { buildOverlayUrl } from '../overlay/overlayConfig';
import styles from './OBSPanel.module.css';

// ---- Overlay panel options ------------------------------------------------

type OverlayPanelOption = { id: string; label: string };

const OVERLAY_PANELS: OverlayPanelOption[] = [
  { id: 'feed',         label: 'Chat' },
  { id: 'mentions',     label: 'Mentions' },
  { id: 'celebrations', label: 'Celebrations' },
  { id: 'stats',        label: 'Stats' },
  { id: 'markets',      label: 'Markets' },
];

// ---- Helpers ---------------------------------------------------------------

type Tab = 'connect' | 'data' | 'overlays';

function statusToDot(state: OBSStatus['state']): DotStatus {
  if (state === 'connected') return 'live';
  if (state === 'error') return 'offline';
  return 'idle';
}

function statusLabel(s: OBSStatus): string {
  if (s.state === 'connected') return 'Connected';
  if (s.state === 'connecting') return 'Connecting…';
  if (s.state === 'error') return s.error ?? 'Error';
  return 'Not connected';
}

function copyText(text: string): Promise<void> {
  if (navigator.clipboard) {
    return navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
  }
  return Promise.resolve(fallbackCopy(text));
}

function fallbackCopy(text: string): void {
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.cssText = 'position:fixed;opacity:0;pointer-events:none';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  } catch { /* nothing */ }
}

// ---- Connect tab -----------------------------------------------------------

function ConnectTab({
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
  const [loaded, setLoaded] = useState(false);

  // Load current config on mount.
  useEffect(() => {
    getOBSConfig().then(r => {
      setHost(r.config.host || 'localhost');
      setPort(String(r.config.port || 4455));
      setEnabled(r.config.enabled);
      setLoaded(true);
    }).catch(() => {
      setLoaded(true);
    });
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setSaveError('');
    try {
      const cfg: OBSConfig = {
        enabled,
        host: host.trim() || 'localhost',
        port: parseInt(port, 10) || 4455,
        has_password: false,
        data_mappings: [],
        event_rules: [],
        update_interval_s: 5,
      };
      const pw = clearPw ? '' : (password || undefined);
      await setOBSConfig(cfg, pw);
      setPassword('');
      setClearPw(false);
      onStatusChange();
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const handleDetect = async () => {
    setDetecting(true);
    setDetectResult(null);
    try {
      const r = await detectOBS();
      setDetectResult(r.detected ? 'OBS detected on localhost:4455.' : 'OBS not found on localhost:4455.');
    } catch {
      setDetectResult('Detection failed. Is OBS running?');
    } finally {
      setDetecting(false);
    }
  };

  if (!loaded) {
    return <div className={styles.tabBody}><Text variant="meta" tone="subtle">Loading…</Text></div>;
  }

  return (
    <div className={styles.tabBody}>
      {/* Status row */}
      <div className={styles.statusRow}>
        <StatusDot status={statusToDot(status.state)} label={statusLabel(status)} />
        <Text variant="ui" className={styles.statusText}>{statusLabel(status)}</Text>
        {status.state === 'connected' && status.obs_version && (
          <Text variant="meta" tone="subtle">
            OBS {status.obs_version} / WS {status.websocket_version}
          </Text>
        )}
      </div>

      {/* Enabled toggle */}
      <div className={styles.fieldRow}>
        <label className={styles.checkLabel}>
          <input
            type="checkbox"
            className={styles.checkbox}
            checked={enabled}
            onChange={e => setEnabled(e.currentTarget.checked)}
          />
          <span className={styles.fieldLabel}>Enable OBS WebSocket integration</span>
        </label>
      </div>

      {/* Host / Port */}
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Host</span>
        <Input
          aria-label="OBS host"
          value={host}
          onChange={e => setHost(e.currentTarget.value)}
          placeholder="localhost"
        />
      </div>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Port</span>
        <Input
          aria-label="OBS WebSocket port"
          type="number"
          value={port}
          onChange={e => setPort(e.currentTarget.value)}
          placeholder="4455"
        />
      </div>

      {/* Password */}
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
        Leave empty to keep existing password.{' '}
        <button
          type="button"
          className={styles.linkBtn}
          onClick={() => { setPassword(''); setClearPw(true); }}
        >
          Clear password
        </button>
      </div>

      {/* Actions */}
      <div className={styles.actionRow}>
        <Button variant="solid" size="sm" onClick={() => void handleSave()} disabled={saving}>
          {saving ? 'Saving…' : 'Save and connect'}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => void handleDetect()} disabled={detecting}>
          <Icon name="search" size={13} />
          {detecting ? 'Detecting…' : 'Detect OBS'}
        </Button>
      </div>

      {detectResult && (
        <Text variant="meta" tone="subtle" as="p" className={styles.inlineNote}>
          {detectResult}
        </Text>
      )}

      {saveError && (
        <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>
          {saveError}
        </Text>
      )}
    </div>
  );
}

// ---- Data tab --------------------------------------------------------------

const STAT_LABELS: Record<DataMapping['stat'], string> = {
  msgs_per_min: 'Msgs / min',
  unique_chatters: 'Unique chatters',
};

function DataTab({ obsConnected }: { obsConnected: boolean }) {
  const [mappings, setMappings] = useState<DataMapping[]>([]);
  const [interval, setInterval] = useState('5');
  const [sources, setSources] = useState<OBSSourceInfo[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    getOBSConfig().then(r => {
      setMappings(r.config.data_mappings ?? []);
      setInterval(String(r.config.update_interval_s ?? 5));
      setLoaded(true);
    }).catch(() => { setLoaded(true); });

    if (obsConnected) {
      getOBSSources().then(r => setSources(r.sources ?? [])).catch(() => {});
    }
  }, [obsConnected]);

  const addRow = () => setMappings(m => [...m, { stat: 'msgs_per_min', source_name: '' }]);

  const removeRow = (i: number) => setMappings(m => m.filter((_, idx) => idx !== i));

  const patchRow = (i: number, patch: Partial<DataMapping>) =>
    setMappings(m => m.map((r, idx) => idx === i ? { ...r, ...patch } : r));

  const handleSave = async () => {
    setSaving(true);
    setSaveError('');
    try {
      const r = await getOBSConfig();
      const cfg: OBSConfig = {
        ...r.config,
        data_mappings: mappings,
        update_interval_s: parseInt(interval, 10) || 5,
      };
      await setOBSConfig(cfg);
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  if (!loaded) {
    return <div className={styles.tabBody}><Text variant="meta" tone="subtle">Loading…</Text></div>;
  }

  return (
    <div className={styles.tabBody}>
      {!obsConnected && (
        <div className={styles.noticeBox}>
          <Icon name="stream" size={14} />
          <Text variant="meta" tone="subtle">Connect to OBS first to push live stats.</Text>
        </div>
      )}

      <Text variant="meta" tone="subtle" as="p" className={styles.descNote}>
        Push live stats to OBS text sources. Create a Text (GDI+) source in OBS and enter its name below.
      </Text>

      {/* Mapping rows */}
      <div className={styles.mappingList}>
        {mappings.map((m, i) => (
          <div key={i} className={styles.mappingRow}>
            <select
              className={styles.nativeSelect}
              aria-label="Stat"
              value={m.stat}
              onChange={e => patchRow(i, { stat: e.currentTarget.value as DataMapping['stat'] })}
            >
              {(Object.entries(STAT_LABELS) as [DataMapping['stat'], string][]).map(([v, l]) => (
                <option key={v} value={v}>{l}</option>
              ))}
            </select>
            <SourceInput
              value={m.source_name}
              sources={sources}
              onChange={v => patchRow(i, { source_name: v })}
            />
            <button
              type="button"
              className={styles.removeBtn}
              aria-label="Remove mapping"
              onClick={() => removeRow(i)}
            >
              <Icon name="x" size={12} />
            </button>
          </div>
        ))}
      </div>

      <button type="button" className={styles.addRowBtn} onClick={addRow}>
        <Icon name="chat" size={13} />
        Add mapping
      </button>

      {/* Update interval */}
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Update every</span>
        <div className={styles.intervalRow}>
          <Input
            aria-label="Update interval (seconds)"
            type="number"
            value={interval}
            onChange={e => setInterval(e.currentTarget.value)}
            placeholder="5"
          />
          <Text variant="meta" tone="subtle">seconds</Text>
        </div>
      </div>

      <div className={styles.actionRow}>
        <Button variant="solid" size="sm" onClick={() => void handleSave()} disabled={saving || !obsConnected}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>

      {saveError && (
        <Text variant="meta" tone="subtle" as="p" className={styles.errorNote}>
          {saveError}
        </Text>
      )}
    </div>
  );
}

// Source name input with datalist autocomplete.
function SourceInput({
  value,
  sources,
  onChange,
}: {
  value: string;
  sources: OBSSourceInfo[];
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
        onChange={e => onChange(e.currentTarget.value)}
        placeholder="OBS source name"
        aria-label="OBS text source name"
      />
      {sources.length > 0 && (
        <datalist id={listId}>
          {sources.filter(s => s.kind.includes('text') || s.kind.includes('Text')).map(s => (
            <option key={s.name} value={s.name} />
          ))}
        </datalist>
      )}
    </div>
  );
}

// ---- Overlays tab ----------------------------------------------------------

function OverlaysTab() {
  const [token, setToken] = useState('');
  const [baseUrl, setBaseUrl] = useState('');
  const [panelId, setPanelId] = useState('feed');
  const [width, setWidth] = useState('400');
  const [height, setHeight] = useState('600');
  const [x, setX] = useState('0');
  const [y, setY] = useState('0');
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

  const overlayUrl = useMemo(() => {
    if (!token || !baseUrl) return '';
    return buildOverlayUrl(baseUrl, {
      panelId,
      token,
      transparent: true,
      width: parseInt(width, 10) || 0,
      height: parseInt(height, 10) || 0,
    });
  }, [token, baseUrl, panelId, width, height]);

  const handleCopy = useCallback(() => {
    if (!overlayUrl) return;
    void copyText(overlayUrl).then(() => {
      setCopied(true);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(false), 2000);
    });
  }, [overlayUrl]);

  const handleOpen = useCallback(() => {
    if (!overlayUrl) return;
    window.open(overlayUrl, '_blank', `width=${width || 400},height=${height || 600},toolbar=no,menubar=no`);
  }, [overlayUrl, width, height]);

  const _x = x; const _y = y; // position inputs: informational, set in OBS

  return (
    <div className={styles.tabBody}>
      <Text variant="meta" tone="subtle" as="p" className={styles.descNote}>
        Add any panel as an OBS browser source. Configure size, then copy the URL.
      </Text>

      {/* Panel picker */}
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Panel</span>
        <select
          className={styles.nativeSelect}
          aria-label="Overlay panel"
          value={panelId}
          onChange={e => setPanelId(e.currentTarget.value)}
        >
          {OVERLAY_PANELS.map(p => (
            <option key={p.id} value={p.id}>{p.label}</option>
          ))}
        </select>
      </div>

      {/* Size */}
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Width</span>
        <Input
          aria-label="Overlay width"
          type="number"
          value={width}
          onChange={e => setWidth(e.currentTarget.value)}
          placeholder="400"
        />
      </div>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>Height</span>
        <Input
          aria-label="Overlay height"
          type="number"
          value={height}
          onChange={e => setHeight(e.currentTarget.value)}
          placeholder="600"
        />
      </div>

      {/* Position inputs: informational, user sets these in OBS */}
      <div className={styles.sectionDivider}>
        <Text variant="meta" tone="subtle">OBS position (set in OBS, not in URL)</Text>
      </div>
      <div className={styles.posRow}>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>X</span>
          <Input
            aria-label="X position"
            type="number"
            value={_x}
            onChange={e => setX(e.currentTarget.value)}
            placeholder="0"
          />
        </div>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>Y</span>
          <Input
            aria-label="Y position"
            type="number"
            value={_y}
            onChange={e => setY(e.currentTarget.value)}
            placeholder="0"
          />
        </div>
      </div>

      {/* URL output */}
      {overlayUrl ? (
        <>
          <div className={styles.urlBox}>
            <code className={styles.urlText}>{overlayUrl}</code>
          </div>
          <div className={styles.actionRow}>
            <Button variant="solid" size="sm" onClick={handleCopy}>
              <Icon name={copied ? 'check' : 'popout'} size={13} />
              {copied ? 'Copied' : 'Copy URL'}
            </Button>
            <Button variant="ghost" size="sm" onClick={handleOpen}>
              <Icon name="stream" size={13} />
              Preview
            </Button>
          </div>
        </>
      ) : (
        <div className={styles.noticeBox}>
          <Text variant="meta" tone="subtle">Daemon not reachable. Run the server first.</Text>
        </div>
      )}

      {/* OBS instructions */}
      <div className={styles.instructionBlock}>
        <Text variant="heading" as="h4" className={styles.instructionTitle}>How to add in OBS</Text>
        <ol className={styles.steps}>
          <li><strong>Add Source</strong> then <strong>Browser</strong></li>
          <li>Paste the URL above and set the width and height to match</li>
          <li>Check <strong>Allow transparency</strong> for a transparent background</li>
        </ol>
        <Text variant="meta" tone="subtle" as="p" className={styles.tokenNote}>
          The URL embeds a read token. Treat it like a password: anyone with it can read the chat feed.
        </Text>
      </div>
    </div>
  );
}

// ---- Root component --------------------------------------------------------

export default function OBSPanel() {
  const [tab, setTab] = useState<Tab>('connect');
  const [status, setStatus] = useState<OBSStatus>({ state: 'disconnected' });

  const refreshStatus = useCallback(() => {
    getOBSStatus().then(setStatus).catch(() => {});
  }, []);

  // Poll status while on the connect tab; light refresh elsewhere.
  useEffect(() => {
    refreshStatus();
    const id = setInterval(refreshStatus, 5000);
    return () => clearInterval(id);
  }, [refreshStatus]);

  const TABS: { id: Tab; label: string }[] = [
    { id: 'connect', label: 'Connect' },
    { id: 'data',    label: 'Data' },
    { id: 'overlays', label: 'Overlays' },
  ];

  return (
    <div className={styles.panel}>
      {/* Tab bar */}
      <div className={styles.tabBar} role="tablist" aria-label="OBS panel sections">
        {TABS.map(t => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            className={`${styles.tabBtn} ${tab === t.id ? styles.tabActive : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
            {t.id === 'connect' && (
              <span className={`${styles.tabDot} ${styles[`tabDot-${statusToDot(status.state)}`]}`} aria-hidden />
            )}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className={styles.tabContent} role="tabpanel">
        {tab === 'connect' && (
          <ConnectTab status={status} onStatusChange={refreshStatus} />
        )}
        {tab === 'data' && (
          <DataTab obsConnected={status.state === 'connected'} />
        )}
        {tab === 'overlays' && (
          <OverlaysTab />
        )}
      </div>
    </div>
  );
}

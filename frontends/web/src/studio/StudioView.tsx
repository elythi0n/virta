import { useCallback, useEffect, useRef, useState } from 'react';
import { Badge, Button, Dialog, Input, Popover, Switch, Tabs, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import ChatOverlay from './ChatOverlay';
import OverlayControls from './OverlayControls';
import Timeline from './Timeline';
import ClipsTab from './ClipsTab';
import { createLocalController, createTwitchController, type PlaybackController } from './playback';
import { VodChatBuffer, parseVodId } from './vodChat';
import { renderLocalClip, renderOverlayClip, downloadSpec } from './export';
import { addClip, deletePreset, savePreset, setActivePreset, setAutoMode, useStudioStore } from './store';
import { clock, duration } from './format';
import { DEFAULT_OVERLAY, type ChatLine, type ClipSpec, type OverlayConfig, type SourceKind } from './types';
import styles from './StudioView.module.css';

type Source = { kind: SourceKind; ref: string; url?: string };
type Export = { active: boolean; frac: number; label: string } | null;

// The Studio: a full-bleed tool view for reviewing a VOD and composing clips with a configurable
// chat overlay. Hero flow: load a VOD, scrub with synced chat, set an in/out range, dial in the
// overlay look (or apply a saved preset), then save/export the clip.
export default function StudioView() {
  const store = useStudioStore();
  const [tab, setTab] = useState<'review' | 'clips'>('review');
  const [source, setSource] = useState<Source | null>(null);
  const [vodInput, setVodInput] = useState('');
  const [error, setError] = useState('');

  const [overlay, setOverlay] = useState<OverlayConfig>({ ...DEFAULT_OVERLAY });
  const [now, setNow] = useState({ t: 0, dur: 0, paused: true });
  const [visible, setVisible] = useState<ChatLine[]>([]);
  const [inSec, setInSec] = useState(0);
  const [outSec, setOutSec] = useState(30);
  const [exp, setExp] = useState<Export>(null);
  const [editLayout, setEditLayout] = useState(false);
  const [chatStatus, setChatStatus] = useState<'idle' | 'loading' | 'ready' | 'empty' | 'error'>('idle');
  const [savePresetOpen, setSavePresetOpen] = useState(false);
  const [presetName, setPresetName] = useState('');
  const [clipTitle, setClipTitle] = useState('');

  const stageRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const controllerRef = useRef<PlaybackController | null>(null);
  const bufferRef = useRef<VodChatBuffer | null>(null);
  const rangeInit = useRef(false);
  const reseekTimer = useRef(0);

  // Apply the active preset's look when auto mode is on (and once on mount).
  useEffect(() => {
    if (!store.autoMode) return;
    const active = store.presets.find((p) => p.id === store.activePresetId);
    if (active) setOverlay({ ...active.overlay });
  }, [store.autoMode, store.activePresetId, store.presets]);

  // Source lifecycle: build the right controller + chat buffer, then poll the clock.
  useEffect(() => {
    if (!source) return;
    let alive = true;
    let timer = 0;
    rangeInit.current = false;

    const startClock = () => {
      timer = window.setInterval(() => {
        const ctrl = controllerRef.current;
        if (!ctrl) return;
        const t = ctrl.getTime();
        const dur = ctrl.getDuration();
        setNow({ t, dur, paused: ctrl.isPaused() });
        if (dur > 0 && !rangeInit.current) {
          rangeInit.current = true;
          setInSec(0);
          setOutSec(Math.min(30, dur));
        }
        const buf = bufferRef.current;
        if (buf) {
          void buf.pump(t);
          setVisible(buf.linesUpTo(t, 40));
        }
      }, 100);
    };

    if (source.kind === 'twitch') {
      const buf = new VodChatBuffer(source.ref);
      bufferRef.current = buf;
      setChatStatus('loading');
      buf
        .seek(0)
        .then(() => alive && setChatStatus(buf.all().length ? 'ready' : 'empty'))
        .catch(() => alive && setChatStatus('error'));
      createTwitchController(stageRef.current as HTMLElement, source.ref, () => {})
        .then((c) => {
          if (!alive) {
            c.destroy();
            return;
          }
          controllerRef.current = c;
          startClock();
        })
        .catch((e) => alive && setError(e instanceof Error ? e.message : 'failed to load the VOD'));
    } else {
      bufferRef.current = null;
      setVisible([]);
      setChatStatus('idle');
      const v = videoRef.current;
      if (v && source.url) {
        v.src = source.url;
        const onMeta = () => {
          if (!alive) return;
          controllerRef.current = createLocalController(v);
          startClock();
        };
        v.addEventListener('loadedmetadata', onMeta, { once: true });
      }
    }

    return () => {
      alive = false;
      if (timer) clearInterval(timer);
      controllerRef.current?.destroy();
      controllerRef.current = null;
    };
  }, [source]);

  // Pause playback when leaving the reviewer so the video doesn't keep playing under the Clips tab.
  useEffect(() => {
    if (tab !== 'review') controllerRef.current?.pause();
  }, [tab]);

  const loadVod = useCallback(() => {
    const id = parseVodId(vodInput);
    if (!id) {
      setError('Enter a Twitch VOD URL (twitch.tv/videos/…) or numeric ID.');
      return;
    }
    setError('');
    setSource({ kind: 'twitch', ref: id });
  }, [vodInput]);

  const loadLocal = useCallback((file: File) => {
    setError('');
    setSource({ kind: 'local', ref: file.name, url: URL.createObjectURL(file) });
  }, []);

  const seek = useCallback((sec: number) => {
    controllerRef.current?.seek(sec);
    // Re-fetching VOD chat is expensive; only do it after scrubbing settles.
    if (bufferRef.current) {
      if (reseekTimer.current) clearTimeout(reseekTimer.current);
      reseekTimer.current = window.setTimeout(() => void bufferRef.current?.seek(sec), 250);
    }
  }, []);

  const patchOverlay = useCallback((patch: Partial<OverlayConfig>) => setOverlay((o) => ({ ...o, ...patch })), []);

  const togglePlay = useCallback(() => {
    const c = controllerRef.current;
    if (!c) return;
    if (c.isPaused()) c.play();
    else c.pause();
  }, []);

  const activePreset = store.presets.find((p) => p.id === store.activePresetId);

  const buildClip = useCallback((): ClipSpec | null => {
    if (!source) return null;
    return {
      id: `clip-${Date.now().toString(36)}`,
      title: clipTitle.trim() || `${source.kind === 'twitch' ? 'VOD' : source.ref} ${clock(inSec)}`,
      source: source.kind,
      sourceRef: source.ref,
      channel: '',
      inSec,
      outSec,
      createdAtMs: Date.now(),
      overlay: { ...overlay },
      presetName: activePreset?.name,
    };
  }, [source, clipTitle, inSec, outSec, overlay, activePreset]);

  const saveClip = useCallback(() => {
    const clip = buildClip();
    if (clip) {
      addClip(clip);
      setClipTitle('');
    }
  }, [buildClip]);

  const runExport = useCallback(
    async (mode: 'overlay' | 'full') => {
      if (!source) return;
      const lines = bufferRef.current?.all() ?? [];
      const title = clipTitle.trim() || 'clip';
      try {
        if (mode === 'full' && source.kind === 'local' && videoRef.current) {
          setExp({ active: true, frac: 0, label: 'Rendering clip…' });
          await renderLocalClip({ video: videoRef.current, lines, overlay, inSec, outSec, title, onProgress: (f) => setExp({ active: true, frac: f, label: 'Rendering clip…' }) });
        } else {
          setExp({ active: true, frac: 0, label: 'Rendering chat overlay…' });
          await renderOverlayClip({ lines, overlay, inSec, outSec, title, onProgress: (f) => setExp({ active: true, frac: f, label: 'Rendering chat overlay…' }) });
        }
      } catch {
        /* surfaced below via the cleared state */
      } finally {
        setExp(null);
      }
    },
    [source, clipTitle, overlay, inSec, outSec],
  );

  const loadClip = useCallback((clip: ClipSpec) => {
    setTab('review');
    setOverlay({ ...clip.overlay });
    setInSec(clip.inSec);
    setOutSec(clip.outSec);
    if (clip.source === 'twitch') {
      setSource({ kind: 'twitch', ref: clip.sourceRef });
      setTimeout(() => seek(clip.inSec), 600);
    }
  }, [seek]);

  const clipLen = Math.max(0, outSec - inSec);

  return (
    <div className={styles.studio}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <Icon name="studio" size={18} />
          <Text variant="title">Studio</Text>
        </div>
        <Tabs
          variant="pill"
          ariaLabel="Studio section"
          value={tab}
          onValueChange={(v) => setTab(v as 'review' | 'clips')}
          items={[
            { value: 'review', label: 'Reviewer' },
            { value: 'clips', label: `Clips${store.clips.length ? ` (${store.clips.length})` : ''}` },
          ]}
        />
        <div className={styles.headerRight}>
          <label className={styles.autoMode}>
            <Text variant="meta" tone="subtle">Auto mode</Text>
            <Switch ariaLabel="Auto mode" checked={store.autoMode} onChange={(e) => setAutoMode(e.currentTarget.checked)} />
          </label>
          <Popover
            align="end"
            trigger={
              <button className={styles.presetTrigger} aria-label="Presets">
                <span>{activePreset?.name ?? 'Preset'}</span>
                <Icon name="chevron-down" size={14} />
              </button>
            }
          >
            <div className={styles.presetMenu}>
              <div className={styles.presetMenuHead}><Text variant="meta" tone="subtle">PRESETS</Text></div>
              {store.presets.map((p) => (
                <div key={p.id} className={`${styles.presetRow} ${p.id === store.activePresetId ? styles.presetActive : ''}`}>
                  <button className={styles.presetName} onClick={() => setActivePreset(p.id)}>
                    {p.id === store.activePresetId && <Icon name="check" size={14} />}
                    <span>{p.name}</span>
                  </button>
                  <button
                    className={styles.presetDel}
                    aria-label={`Delete ${p.name}`}
                    disabled={store.presets.length <= 1}
                    onClick={() => deletePreset(p.id)}
                  >
                    <Icon name="trash" size={13} />
                  </button>
                </div>
              ))}
            </div>
          </Popover>
        </div>
      </header>

      {/* The reviewer stays mounted whenever a source is loaded — hidden (not unmounted) on the
          Clips tab — so the video player, clock, and chat buffer survive switching tabs. */}
      {tab === 'review' && !source && (
        <Loader vodInput={vodInput} setVodInput={setVodInput} onLoadVod={loadVod} onLoadLocal={loadLocal} error={error} />
      )}
      {tab === 'clips' && <ClipsTab onLoad={loadClip} />}
      {source && (
        <div className={styles.workspace} hidden={tab !== 'review'}>
          <div className={styles.main}>
            <div className={`${styles.stage} ${editLayout ? styles.editing : ''}`}>
              {source.kind === 'twitch' ? (
                <div ref={stageRef} className={styles.video} style={{ pointerEvents: editLayout ? 'none' : 'auto' }} />
              ) : (
                <video ref={videoRef} className={styles.video} style={{ pointerEvents: editLayout ? 'none' : 'auto' }} controls={!editLayout} playsInline />
              )}
              {/* In edit mode the overlay layer captures input (drag/resize the chat box); in watch
                  mode it's click-through so the video player stays fully interactive (native
                  play/sound/controls — a programmatic play() in the cross-origin iframe is blocked). */}
              <div className={styles.overlayLayer} style={{ pointerEvents: editLayout ? 'auto' : 'none' }}>
                <ChatOverlay
                  lines={visible}
                  config={overlay}
                  staticFrame={now.paused}
                  editable={editLayout}
                  onRectChange={(rect) => patchOverlay({ rect })}
                />
              </div>
              {editLayout && <div className={styles.editBadge}>Editing chat layout — drag to move, corners to resize</div>}
              {source.kind === 'twitch' && chatStatus === 'loading' && <div className={styles.chatStatus}>Loading chat…</div>}
              {source.kind === 'twitch' && chatStatus === 'empty' && <div className={styles.chatStatus}>No saved chat for this VOD</div>}
              {source.kind === 'twitch' && chatStatus === 'error' && <div className={styles.chatStatus}>Couldn’t load chat for this VOD</div>}
            </div>

            <Timeline
              duration={now.dur}
              current={now.t}
              moments={[]}
              inSec={inSec}
              outSec={outSec}
              onSeek={seek}
              onSetIn={setInSec}
              onSetOut={setOutSec}
            />

            <div className={styles.transport}>
              <button className={styles.playBtn} onClick={togglePlay} aria-label={now.paused ? 'Play' : 'Pause'}>
                <Icon name={now.paused ? 'play' : 'pause'} size={18} />
              </button>
              <span className={styles.timecode}>{clock(now.t)} <span className={styles.dim}>/ {clock(now.dur)}</span></span>
              <div className={styles.spacer} />
              <Tooltip content={editLayout ? 'Done — back to watching' : 'Move & resize the chat overlay on the video'} side="top">
                <Button size="sm" variant={editLayout ? 'solid' : 'ghost'} onClick={() => setEditLayout((v) => !v)}>
                  <Icon name="studio" size={14} /> {editLayout ? 'Done' : 'Edit chat'}
                </Button>
              </Tooltip>
              <Tooltip content="Set clip start to playhead" side="top">
                <Button size="sm" variant="subtle" onClick={() => setInSec(now.t)}>Set in</Button>
              </Tooltip>
              <Badge tone="accent">{duration(clipLen)}</Badge>
              <Tooltip content="Set clip end to playhead" side="top">
                <Button size="sm" variant="subtle" onClick={() => setOutSec(now.t)}>Set out</Button>
              </Tooltip>
            </div>
          </div>

          <aside className={styles.rail}>
            <div className={styles.railScroll}>
              <OverlayControls config={overlay} onChange={patchOverlay} />
            </div>
            <div className={styles.railActions}>
              <Input placeholder="Clip title" value={clipTitle} onChange={(e) => setClipTitle(e.currentTarget.value)} />
              <div className={styles.actionRow}>
                <Button variant="ghost" size="sm" onClick={() => { setPresetName(activePreset?.name ? `${activePreset.name} copy` : ''); setSavePresetOpen(true); }}>Save preset</Button>
                <Button variant="subtle" size="sm" onClick={saveClip}>Save clip</Button>
              </div>
              <div className={styles.actionRow}>
                <Button variant="solid" size="md" onClick={() => void runExport('overlay')} disabled={exp?.active}>
                  <Icon name="download" size={14} /> Export chat overlay
                </Button>
              </div>
              {source.kind === 'local' && (
                <Button variant="subtle" size="sm" onClick={() => void runExport('full')} disabled={exp?.active}>
                  <Icon name="download" size={14} /> Export full clip (video + chat)
                </Button>
              )}
              <Button variant="ghost" size="sm" onClick={() => { const c = buildClip(); if (c) downloadSpec(c); }}>Export spec (.json)</Button>
              {exp?.active && (
                <div className={styles.progress} role="status">
                  <div className={styles.progressBar} style={{ width: `${Math.round(exp.frac * 100)}%` }} />
                  <Text variant="meta" tone="subtle">{exp.label} {Math.round(exp.frac * 100)}%</Text>
                </div>
              )}
            </div>
          </aside>
        </div>
      )}

      <Dialog open={savePresetOpen} onOpenChange={setSavePresetOpen} title="Save preset" description="Save the current overlay look so you can reuse it on future clips." footer={
        <>
          <Button variant="ghost" onClick={() => setSavePresetOpen(false)}>Cancel</Button>
          <Button variant="solid" onClick={() => { savePreset(presetName, overlay); setSavePresetOpen(false); }}>Save</Button>
        </>
      }>
        <Input autoFocus placeholder="Preset name" value={presetName} onChange={(e) => setPresetName(e.currentTarget.value)} />
      </Dialog>
    </div>
  );
}

// The empty state: load a Twitch VOD or a local recording.
function Loader({ vodInput, setVodInput, onLoadVod, onLoadLocal, error }: {
  vodInput: string;
  setVodInput: (s: string) => void;
  onLoadVod: () => void;
  onLoadLocal: (f: File) => void;
  error: string;
}) {
  const fileRef = useRef<HTMLInputElement>(null);
  return (
    <div className={styles.loader}>
      <div className={styles.loaderCard}>
        <Icon name="studio" size={32} />
        <Text variant="heading">Open a VOD to review and clip</Text>
        <Text variant="body" tone="subtle">Replay a Twitch VOD with its chat synced, or load your own recording.</Text>
        <div className={styles.loaderRow}>
          <Input placeholder="twitch.tv/videos/… or VOD ID" value={vodInput} onChange={(e) => setVodInput(e.currentTarget.value)} onKeyDown={(e) => e.key === 'Enter' && onLoadVod()} />
          <Button variant="solid" onClick={onLoadVod}>Load VOD</Button>
        </div>
        <div className={styles.or}><span>or</span></div>
        <Button variant="subtle" onClick={() => fileRef.current?.click()}>Load a local recording</Button>
        <input ref={fileRef} type="file" accept="video/*" hidden onChange={(e) => { const f = e.currentTarget.files?.[0]; if (f) onLoadLocal(f); }} />
        {error && <Text variant="meta" tone="default" className={styles.error}>{error}</Text>}
      </div>
    </div>
  );
}

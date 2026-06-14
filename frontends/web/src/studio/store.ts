import { useSyncExternalStore } from 'react';
import { DEFAULT_OVERLAY, type ClipSpec, type OverlayConfig, type StudioPreset } from './types';

// Studio persistence: presets (saved looks), defined clips, and settings (active preset + auto mode).
// Backed by localStorage and exposed through a tiny reactive store so every surface stays in sync.

const KEY = 'virta.studio.v1';

interface StudioState {
  presets: StudioPreset[];
  clips: ClipSpec[];
  activePresetId: string | null;
  autoMode: boolean; // when on, new clips inherit the active preset's overlay automatically
}

const STARTER_PRESETS: StudioPreset[] = [
  { id: 'preset-corner', name: 'Corner chat', overlay: { ...DEFAULT_OVERLAY } },
  {
    id: 'preset-sidebar',
    name: 'Side rail',
    overlay: { ...DEFAULT_OVERLAY, rect: { x: 0.72, y: 0.05, w: 0.26, h: 0.9 }, theme: 'solid', opacity: 0.7, maxLines: 14 },
  },
  {
    id: 'preset-minimal',
    name: 'Minimal',
    overlay: { ...DEFAULT_OVERLAY, rect: { x: 0.03, y: 0.5, w: 0.34, h: 0.46 }, theme: 'minimal', opacity: 0, shadow: true, nameStyle: 'bold', maxLines: 5 },
  },
];

// Merge any saved overlay onto current defaults so older saves (e.g. from before the free-rect
// model) gain the fields they're missing instead of crashing the renderer.
function normalizeOverlay(o: Partial<OverlayConfig> | undefined): OverlayConfig {
  return {
    ...DEFAULT_OVERLAY,
    ...(o ?? {}),
    rect: { ...DEFAULT_OVERLAY.rect, ...(o?.rect ?? {}) },
  };
}

function read(): StudioState {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<StudioState>;
      const presets = (parsed.presets?.length ? parsed.presets : STARTER_PRESETS).map((p) => ({
        ...p,
        overlay: normalizeOverlay(p.overlay),
      }));
      const clips = (parsed.clips ?? []).map((c) => ({ ...c, overlay: normalizeOverlay(c.overlay) }));
      return {
        presets,
        clips,
        activePresetId: parsed.activePresetId ?? presets[0]?.id ?? STARTER_PRESETS[0].id,
        autoMode: parsed.autoMode ?? true,
      };
    }
  } catch {
    /* fall through to defaults */
  }
  return { presets: STARTER_PRESETS, clips: [], activePresetId: STARTER_PRESETS[0].id, autoMode: true };
}

let state: StudioState = read();
const listeners = new Set<() => void>();

function commit(next: StudioState) {
  state = next;
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* private mode / quota — keep in-memory */
  }
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

// ── Mutations ────────────────────────────────────────────────────────────────
export function savePreset(name: string, overlay: OverlayConfig): StudioPreset {
  const id = `preset-${Date.now().toString(36)}-${state.presets.length}`;
  const preset: StudioPreset = { id, name: name.trim() || 'Untitled', overlay: { ...overlay } };
  commit({ ...state, presets: [...state.presets, preset], activePresetId: id });
  return preset;
}

export function updatePreset(id: string, patch: Partial<StudioPreset>) {
  commit({ ...state, presets: state.presets.map((p) => (p.id === id ? { ...p, ...patch } : p)) });
}

export function deletePreset(id: string) {
  const presets = state.presets.filter((p) => p.id !== id);
  const activePresetId = state.activePresetId === id ? (presets[0]?.id ?? null) : state.activePresetId;
  commit({ ...state, presets, activePresetId });
}

export function setActivePreset(id: string) {
  commit({ ...state, activePresetId: id });
}

export function setAutoMode(on: boolean) {
  commit({ ...state, autoMode: on });
}

export function addClip(clip: ClipSpec) {
  commit({ ...state, clips: [clip, ...state.clips] });
}

export function removeClip(id: string) {
  commit({ ...state, clips: state.clips.filter((c) => c.id !== id) });
}

// ── Reactive access ──────────────────────────────────────────────────────────
export function useStudioStore(): StudioState {
  return useSyncExternalStore(subscribe, () => state, () => state);
}

export function activeOverlay(): OverlayConfig {
  const active = state.presets.find((p) => p.id === state.activePresetId);
  return normalizeOverlay(active?.overlay);
}

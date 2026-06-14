// Playback controllers: a small common surface over the two video sources so the rest of the studio
// (chat sync, timeline, transport) reads one clock regardless of where the video comes from.
//   - TwitchController wraps the Twitch embedded player JS API (real VOD playback + getCurrentTime).
//   - LocalVideoController wraps a <video> element for a streamer's own recording.

export interface PlaybackController {
  getTime(): number; // seconds
  getDuration(): number; // seconds (0 if unknown)
  isPaused(): boolean;
  play(): void;
  pause(): void;
  seek(sec: number): void;
  setRate(rate: number): void;
  destroy(): void;
}

// ── Twitch embedded player ─────────────────────────────────────────────────────
interface TwitchPlayerInstance {
  getCurrentTime(): number;
  getDuration(): number;
  isPaused(): boolean;
  play(): void;
  pause(): void;
  seek(t: number): void;
  setQuality?(q: string): void;
  addEventListener(event: string, cb: () => void): void;
}
interface TwitchPlayerCtor {
  new (el: HTMLElement | string, opts: Record<string, unknown>): TwitchPlayerInstance;
  READY: string;
  PLAY: string;
  PAUSE: string;
  ENDED: string;
}
declare global {
  interface Window {
    Twitch?: { Player: TwitchPlayerCtor };
  }
}

const EMBED_SRC = 'https://player.twitch.tv/js/embed/v1.js';
let embedPromise: Promise<void> | null = null;

function loadTwitchEmbed(): Promise<void> {
  if (window.Twitch?.Player) return Promise.resolve();
  if (embedPromise) return embedPromise;
  embedPromise = new Promise<void>((resolve, reject) => {
    const s = document.createElement('script');
    s.src = EMBED_SRC;
    s.async = true;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error('failed to load the Twitch player'));
    document.head.appendChild(s);
  });
  return embedPromise;
}

export async function createTwitchController(
  mount: HTMLElement,
  vodId: string,
  onReady: () => void,
): Promise<PlaybackController> {
  await loadTwitchEmbed();
  const Ctor = window.Twitch!.Player;
  // The embed API resolves its mount by element id, so guarantee one.
  if (!mount.id) mount.id = `twitch-player-${Math.random().toString(36).slice(2)}`;
  const player = new Ctor(mount.id, {
    video: vodId,
    width: '100%',
    height: '100%',
    autoplay: false,
    muted: false,
    parent: [location.hostname],
  });
  player.addEventListener(Ctor.READY, onReady);
  return {
    getTime: () => player.getCurrentTime() || 0,
    getDuration: () => player.getDuration() || 0,
    isPaused: () => player.isPaused(),
    play: () => player.play(),
    pause: () => player.pause(),
    seek: (sec) => player.seek(sec),
    setRate: () => {}, // the Twitch embed has no public rate control
    destroy: () => {
      mount.innerHTML = '';
    },
  };
}

// ── Local <video> element ───────────────────────────────────────────────────────
export function createLocalController(video: HTMLVideoElement): PlaybackController {
  return {
    getTime: () => video.currentTime || 0,
    getDuration: () => (Number.isFinite(video.duration) ? video.duration : 0),
    isPaused: () => video.paused,
    play: () => void video.play(),
    pause: () => video.pause(),
    seek: (sec) => {
      video.currentTime = sec;
    },
    setRate: (rate) => {
      video.playbackRate = rate;
    },
    destroy: () => {
      video.removeAttribute('src');
      video.load();
    },
  };
}

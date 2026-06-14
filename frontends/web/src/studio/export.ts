import type { ChatLine, ClipSpec, OverlayConfig } from './types';

// Clip export. For a local recording we can composite the chat overlay onto the video frames with a
// 2D canvas and capture the result to a real video file (chat baked in). For a platform VOD the
// source video isn't ours to re-encode client-side, so we export a portable clip spec instead.

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 4000);
}

export function downloadSpec(clip: ClipSpec) {
  const blob = new Blob([JSON.stringify(clip, null, 2)], { type: 'application/json' });
  triggerDownload(blob, `${slug(clip.title)}.clip.json`);
}

function slug(s: string) {
  return (s || 'clip').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'clip';
}

const FONT_FAMILY: Record<OverlayConfig['font'], string> = {
  ui: 'system-ui, sans-serif',
  mono: 'ui-monospace, monospace',
  rounded: 'Nunito, system-ui, sans-serif',
  condensed: '"Roboto Condensed", system-ui, sans-serif',
};

// Re-implements the overlay layout in canvas 2D so the baked frame matches the live preview closely.
function drawOverlay(ctx: CanvasRenderingContext2D, W: number, H: number, lines: ChatLine[], cfg: OverlayConfig) {
  if (!cfg.enabled || lines.length === 0) return;
  const fs = Math.round(15 * cfg.scale * (W / 1280));
  const lh = Math.round(fs * 1.35);
  const pad = Math.round(12 * cfg.padding * (W / 1280));
  const shown = lines.slice(-cfg.maxLines);
  // The box is the free rect; lines pin to its bottom and clip to its height.
  const x = cfg.rect.x * W;
  const y = cfg.rect.y * H;
  const boxW = cfg.rect.w * W;
  const boxH = cfg.rect.h * H;

  // Background
  if (cfg.theme !== 'minimal' && cfg.theme !== 'outline' && cfg.opacity > 0) {
    ctx.fillStyle = `rgba(14,15,18,${cfg.opacity})`;
    roundRect(ctx, x, y, boxW, boxH, 8);
    ctx.fill();
  }

  ctx.textBaseline = 'bottom';
  ctx.font = `${fs}px ${FONT_FAMILY[cfg.font]}`;
  let cy = y + boxH - pad;
  for (let i = shown.length - 1; i >= 0 && cy > y + pad; i--) {
    const l = shown[i];
    const namePart = cfg.nameStyle === 'hidden' ? '' : `${l.author}: `;
    if (cfg.shadow || cfg.theme === 'outline') {
      ctx.shadowColor = 'rgba(0,0,0,0.9)';
      ctx.shadowBlur = 3;
    } else {
      ctx.shadowBlur = 0;
    }
    let tx = x + pad;
    if (namePart) {
      ctx.font = `600 ${fs}px ${FONT_FAMILY[cfg.font]}`;
      ctx.fillStyle = cfg.nameStyle === 'colored' ? l.color || cfg.accent : '#ffffff';
      ctx.fillText(namePart, tx, cy);
      tx += ctx.measureText(namePart).width;
    }
    ctx.font = `${fs}px ${FONT_FAMILY[cfg.font]}`;
    ctx.fillStyle = '#f2f4f8';
    ctx.fillText(l.text, tx, cy);
    cy -= lh;
  }
  ctx.shadowBlur = 0;
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

type RenderArgs = {
  video: HTMLVideoElement;
  lines: ChatLine[]; // full buffer for the source, sorted by offset
  overlay: OverlayConfig;
  inSec: number;
  outSec: number;
  title: string;
  onProgress?: (frac: number) => void;
};

// Renders [inSec, outSec] of a local video with the chat overlay baked in, capturing to a WebM via
// MediaRecorder in real time. Resolves when the file has downloaded.
export async function renderLocalClip(args: RenderArgs): Promise<void> {
  const { video, lines, overlay, inSec, outSec, title, onProgress } = args;
  const W = video.videoWidth || 1280;
  const H = video.videoHeight || 720;
  const canvas = document.createElement('canvas');
  canvas.width = W;
  canvas.height = H;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('canvas unavailable');

  const fps = 30;
  const stream = canvas.captureStream(fps);
  // Pull audio from the source so the clip keeps sound.
  const elWithCapture = video as HTMLVideoElement & { captureStream?: () => MediaStream };
  const srcStream = elWithCapture.captureStream?.();
  srcStream?.getAudioTracks().forEach((t) => stream.addTrack(t));

  const mime = MediaRecorder.isTypeSupported('video/webm;codecs=vp9') ? 'video/webm;codecs=vp9' : 'video/webm';
  const recorder = new MediaRecorder(stream, { mimeType: mime, videoBitsPerSecond: 8_000_000 });
  const chunks: BlobPart[] = [];
  recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);

  const done = new Promise<void>((resolve) => {
    recorder.onstop = () => {
      triggerDownload(new Blob(chunks, { type: 'video/webm' }), `${slug(title)}.webm`);
      resolve();
    };
  });

  await seekTo(video, inSec);
  recorder.start(200);
  await video.play();

  let raf = 0;
  const draw = () => {
    const t = video.currentTime;
    ctx.drawImage(video, 0, 0, W, H);
    const windowed = lines.filter((l) => l.offset <= t).slice(-overlay.maxLines);
    drawOverlay(ctx, W, H, windowed, overlay);
    onProgress?.(Math.min(1, (t - inSec) / Math.max(0.1, outSec - inSec)));
    if (t >= outSec) {
      cancelAnimationFrame(raf);
      video.pause();
      recorder.stop();
      return;
    }
    raf = requestAnimationFrame(draw);
  };
  raf = requestAnimationFrame(draw);
  await done;
}

type OverlayRenderArgs = {
  lines: ChatLine[];
  overlay: OverlayConfig;
  inSec: number;
  outSec: number;
  title: string;
  width?: number;
  height?: number;
  onProgress?: (frac: number) => void;
};

// Renders just the chat overlay over the [inSec, outSec] window to a TRANSPARENT video, captured in
// real time. The result is a chat-overlay track a clipper drops over their own footage — and it
// works for any source (Twitch VODs included) since no source video is touched.
export async function renderOverlayClip(args: OverlayRenderArgs): Promise<void> {
  const { lines, overlay, inSec, outSec, title, onProgress } = args;
  const W = args.width ?? 1280;
  const H = args.height ?? 720;
  const canvas = document.createElement('canvas');
  canvas.width = W;
  canvas.height = H;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('canvas unavailable');

  const fps = 30;
  const stream = canvas.captureStream(fps);
  const mime = MediaRecorder.isTypeSupported('video/webm;codecs=vp9') ? 'video/webm;codecs=vp9' : 'video/webm';
  const recorder = new MediaRecorder(stream, { mimeType: mime, videoBitsPerSecond: 6_000_000 });
  const chunks: BlobPart[] = [];
  recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);
  const done = new Promise<void>((resolve) => {
    recorder.onstop = () => {
      triggerDownload(new Blob(chunks, { type: 'video/webm' }), `${slug(title)}-chat.webm`);
      resolve();
    };
  });

  const span = Math.max(0.1, outSec - inSec);
  const startWall = performance.now();
  recorder.start(200);
  let raf = 0;
  const draw = () => {
    const elapsed = (performance.now() - startWall) / 1000;
    const t = inSec + elapsed;
    ctx.clearRect(0, 0, W, H); // transparent background
    drawOverlay(ctx, W, H, lines.filter((l) => l.offset <= t), overlay);
    onProgress?.(Math.min(1, elapsed / span));
    if (elapsed >= span) {
      cancelAnimationFrame(raf);
      recorder.stop();
      return;
    }
    raf = requestAnimationFrame(draw);
  };
  raf = requestAnimationFrame(draw);
  await done;
}

function seekTo(video: HTMLVideoElement, sec: number): Promise<void> {
  return new Promise((resolve) => {
    const onSeeked = () => {
      video.removeEventListener('seeked', onSeeked);
      resolve();
    };
    video.addEventListener('seeked', onSeeked);
    video.currentTime = sec;
  });
}

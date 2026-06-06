import { request } from './http';
import type { SendResult, SendTarget } from './wire.gen';

// Per-target reachability for the composer's chips — which channels a message can go to, and why
// not where it can't (no send, no platform write).
export function previewSend(channels: string[]): Promise<SendTarget[]> {
  return request<{ targets: SendTarget[] }>('/v1/send/preview', {
    method: 'POST',
    body: JSON.stringify({ channels }),
  }).then((r) => r.targets);
}

// Cross-post text to the targets; returns each target's disposition.
export function sendMessage(channels: string[], text: string): Promise<SendResult[]> {
  return request<{ results: SendResult[] }>('/v1/send', {
    method: 'POST',
    body: JSON.stringify({ channels, text }),
  }).then((r) => r.results);
}

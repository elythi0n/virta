import type { Platform } from './types';

// Brand glyphs for the per-row platform rail. The brand color comes from CSS (currentColor via a
// data-platform rule), keeping platform color to the two sanctioned accents: the rail and this
// glyph. Each mark keeps its own viewBox.
export default function PlatformGlyph({ platform, className }: { platform: Platform; className?: string }) {
  switch (platform) {
    case 'twitch':
      return (
        <svg className={className} data-platform="twitch" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
          <path d="M4.265 0 1.6 4.064v15.05h5.067V24l4.266-4.886h3.733L21.6 12V0H4.265Zm15.467 11.2-3.2 3.2h-3.733l-2.667 2.4v-2.4H6.667V1.6h13.065V11.2Z" />
          <path d="M17.6 4.8h-1.6v4.8h1.6V4.8ZM13.067 4.8h-1.6v4.8h1.6V4.8Z" />
        </svg>
      );
    case 'kick':
      return (
        <svg className={className} data-platform="kick" viewBox="0 0 32 32" fill="currentColor" aria-hidden>
          <path d="M3 3h7v7h3V6.5h3V3h7v7h-3.5v3.5h-3V17h3v3.5H23V24h-7v-3.5h-3V17h-3v8H3V3Z" />
        </svg>
      );
    case 'x':
      return (
        <svg className={className} data-platform="x" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
          <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24h-6.656l-5.214-6.817-5.966 6.817H1.683l7.73-8.835L1.254 2.25H8.08l4.713 6.231 5.45-6.231Zm-1.161 17.52h1.833L7.084 4.126H5.117L17.083 19.77Z" />
        </svg>
      );
  }
}

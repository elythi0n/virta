import { Text } from '@virta/ui-kit';
import { platformLabel } from '@virta/feed-core';
import Icon from '../Icon';
import { useIsDesktop } from '../shell/useIsDesktop';
import styles from './WatchPane.module.css';

function embedUrl(platform: string, slug: string): string | null {
  const parent = location.hostname || 'localhost';
  switch (platform) {
    case 'twitch':
      return `https://player.twitch.tv/?channel=${encodeURIComponent(slug)}&parent=${parent}&muted=true`;
    case 'kick':
      return `https://player.kick.com/${encodeURIComponent(slug)}?parent=${parent}`;
    default:
      // YouTube has no handle-addressable embed (embeds need a channel/video id), so it gets
      // the channel-page link below instead of an iframe player.
      return null;
  }
}

// The platform's own watch page, for opening outside the embed (desktop window or a new tab).
function pageUrl(platform: string, slug: string): string | null {
  switch (platform) {
    case 'twitch':
      return `https://www.twitch.tv/${encodeURIComponent(slug)}`;
    case 'kick':
      return `https://kick.com/${encodeURIComponent(slug)}`;
    case 'youtube':
      return `https://www.youtube.com/@${encodeURIComponent(slug.replace(/^@/, ''))}/live`;
    default:
      return null;
  }
}

// The Twitch/Kick players need a Chromium-grade webview. Windows' WebView2 is Chromium and embeds
// fine, but macOS/Linux Wails use WebKit(GTK), whose player can't decode the IVS stream. Detect
// Chromium by the "Chrome/" UA token (present in WebView2 and real browsers, absent in WebKit).
const isChromiumWebview = typeof navigator !== 'undefined' && /Chrome\//.test(navigator.userAgent);

export default function WatchPane({ channel }: { channel?: string }) {
  const isDesktop = useIsDesktop();

  if (!channel) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">Open a stream from the Streams panel.</Text>
      </div>
    );
  }

  const [platform, slug = ''] = channel.split(':');
  const embed = embedUrl(platform, slug);
  const page = pageUrl(platform, slug);

  if (!embed && !page) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">No player available for {platform}.</Text>
      </div>
    );
  }

  // Two cases land on the channel page instead of an embed: a WebKit-based desktop webview
  // (macOS/Linux Wails), where Twitch's IVS player can't decode, and platforms without an
  // embeddable player (YouTube). Windows' Chromium WebView2 keeps the embed. Both fallbacks show
  // a button that opens the page externally.
  if (page && ((isDesktop && !isChromiumWebview) || !embed)) {
    const plat = platformLabel(platform);
    return (
      <div className={styles.placeholder}>
        <button
          type="button"
          className={styles.openBtn}
          onClick={() => {
            if (window.wails?.Browser?.OpenURL) void window.wails.Browser.OpenURL(page);
            else window.open(page, '_blank', 'noopener');
          }}
        >
          <Icon name="popout" size={14} />
          Watch {slug} on {plat}
        </button>
        <Text variant="meta" tone="subtle" as="p" className={styles.desktopNote}>
          {isDesktop ? 'Opens in your default browser.' : 'Opens in a new tab.'}
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <iframe
        className={styles.player}
        src={embed ?? undefined}
        title={`${slug} stream`}
        allow="autoplay; fullscreen; picture-in-picture"
        allowFullScreen
      />
    </div>
  );
}

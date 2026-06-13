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

// Whether the current webview is Chromium (WebView2 on Windows, or a real browser): present as the
// "Chrome/" UA token, absent in WebKit(GTK) on macOS/Linux, whose player can't decode the streams.
const isChromiumWebview = typeof navigator !== 'undefined' && /Chrome\//.test(navigator.userAgent);

// canEmbed reports whether the platform's player iframe will actually render in this context.
//   - No embeddable player (YouTube) → false.
//   - In a browser → always true (https origin, no restrictions).
//   - In a WebKit desktop webview → false (can't decode the IVS/HLS player).
//   - Twitch in a desktop webview → false: Twitch sets frame-ancestors to https://<parent>, but
//     the Wails webview is served over http://wails.localhost, so the embed is CSP-blocked. Kick
//     has no such restriction and embeds fine in Chromium WebView2.
function canEmbed(platform: string, hasEmbed: boolean, isDesktop: boolean): boolean {
  if (!hasEmbed) return false;
  if (!isDesktop) return true;
  if (!isChromiumWebview) return false;
  return platform !== 'twitch';
}

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

  // When the embed can't render here (see canEmbed), show a button that opens the channel another
  // way — covers YouTube, WebKit desktop webviews, and Twitch on the desktop.
  if (page && !canEmbed(platform, !!embed, isDesktop)) {
    const plat = platformLabel(platform);
    // On a Chromium desktop webview (Windows WebView2) the shell can open a native in-app player
    // window via the bound App.OpenStreamWindow method — it loads the channel page top-level, so
    // Twitch's player works despite the in-panel iframe being CSP-blocked. WebKit desktop webviews
    // (macOS/Linux) can't decode the player even top-level, so those open the system browser.
    const useNativeWindow = isDesktop && isChromiumWebview && !!window.wails?.Call;
    return (
      <div className={styles.placeholder}>
        <button
          type="button"
          className={styles.openBtn}
          onClick={() => {
            if (useNativeWindow) {
              void window.wails!.Call!({ methodName: 'main.App.OpenStreamWindow', args: [platform, slug] });
            } else if (window.wails?.Browser?.OpenURL) {
              void window.wails.Browser.OpenURL(page);
            } else {
              window.open(page, '_blank', 'noopener');
            }
          }}
        >
          <Icon name="popout" size={14} />
          {useNativeWindow ? `Open ${slug} player` : `Watch ${slug} on ${plat}`}
        </button>
        <Text variant="meta" tone="subtle" as="p" className={styles.desktopNote}>
          {useNativeWindow ? 'Opens in an in-app player window.' : isDesktop ? 'Opens in your default browser.' : 'Opens in a new tab.'}
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

import { Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { useIsDesktop } from '../shell/useIsDesktop';
import styles from './WatchPane.module.css';

// Native page URL (not the embed URL) to open in an external browser.
function nativeUrl(platform: string, slug: string): string | null {
  switch (platform) {
    case 'twitch': return `https://twitch.tv/${encodeURIComponent(slug)}`;
    case 'kick':   return `https://kick.com/${encodeURIComponent(slug)}`;
    default:       return null;
  }
}

// Embed iframe URL. Only used when running in a real browser where
// location.hostname is a valid domain the player can validate.
function embedUrl(platform: string, slug: string): string | null {
  const parent = location.hostname || 'localhost';
  switch (platform) {
    case 'twitch':
      return `https://player.twitch.tv/?channel=${encodeURIComponent(slug)}&parent=${parent}&muted=true`;
    case 'kick':
      return `https://player.kick.com/${encodeURIComponent(slug)}?parent=${parent}`;
    default:
      return null;
  }
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

  // In the Wails desktop app the page runs at wails://wails/ so
  // location.hostname is "wails". Twitch and Kick both validate the parent=
  // parameter against the actual embedding hostname and reject "wails".
  // Opening in the system browser is the reliable path for the desktop app.
  if (isDesktop) {
    const url = nativeUrl(platform, slug);
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle" as="p" className={styles.desktopNote}>
          Stream embeds require a browser context. Open this stream in your browser:
        </Text>
        {url ? (
          <button
            type="button"
            className={styles.openBtn}
            onClick={() => window.open(url, '_blank', 'noopener')}
          >
            <Icon name="popout" size={14} />
            Watch {slug} on {platform.charAt(0).toUpperCase() + platform.slice(1)}
          </button>
        ) : (
          <Text variant="meta" tone="subtle">No player available for {platform}.</Text>
        )}
      </div>
    );
  }

  const url = embedUrl(platform, slug);
  if (!url) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">No embeddable player for {platform}.</Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <iframe
        className={styles.player}
        src={url}
        title={`${slug} stream`}
        allow="autoplay; fullscreen; picture-in-picture"
        allowFullScreen
      />
    </div>
  );
}

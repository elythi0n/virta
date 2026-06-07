import { Text } from '@virta/ui-kit';
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
  const embed = embedUrl(platform, slug);

  if (!embed) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">No player available for {platform}.</Text>
      </div>
    );
  }

  // In the desktop app (wails:// origin), Twitch's IVS player needs WebGPU
  // which is not available in WebKitGTK. Open in a dedicated Wails native
  // window pointing to the actual channel page (not the embed iframe).
  if (isDesktop) {
    const label = slug;
    const plat = platform.charAt(0).toUpperCase() + platform.slice(1);
    return (
      <div className={styles.placeholder}>
        <button
          type="button"
          className={styles.openBtn}
          onClick={() => void window.wails?.Browser?.OpenURL?.(
            platform === 'twitch'
              ? `https://www.twitch.tv/${encodeURIComponent(slug)}`
              : `https://kick.com/${encodeURIComponent(slug)}`
          )}
        >
          <Icon name="popout" size={14} />
          Watch {label} on {plat}
        </button>
        <Text variant="meta" tone="subtle" as="p" className={styles.desktopNote}>
          Opens in your default browser.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <iframe
        className={styles.player}
        src={embed}
        title={`${slug} stream`}
        allow="autoplay; fullscreen; picture-in-picture"
        allowFullScreen
      />
    </div>
  );
}

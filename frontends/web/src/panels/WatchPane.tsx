import { Text } from '@virta/ui-kit';
import styles from './WatchPane.module.css';

// In the Wails desktop app the page runs under wails://wails/ so
// location.hostname is "wails" — not a valid parent for embed players.
// Fall back to "localhost" for any non-HTTP-hostname.
function embedParent(): string {
  const h = location.hostname;
  return h && h !== 'wails' ? h : 'localhost';
}

function embedUrl(platform: string, slug: string): string | null {
  const parent = embedParent();
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
  if (!channel) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">Open a stream from the Streams panel.</Text>
      </div>
    );
  }
  const [platform, slug = ''] = channel.split(':');
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

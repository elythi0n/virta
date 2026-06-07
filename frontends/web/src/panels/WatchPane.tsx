import { Text } from '@virta/ui-kit';
import styles from './WatchPane.module.css';

// Compute the parent domain for embed players. In a normal browser this is the
// page hostname. In the Wails desktop app (wails://wails/) location.hostname is
// "wails" — Twitch rejects this; "localhost" is the canonical allow-listed value
// for local/desktop clients on both Twitch and Kick.
function embedParent(): string {
  const h = location.hostname;
  if (!h || h === 'wails') return 'localhost';
  return h;
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

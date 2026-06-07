import { Text } from '@virta/ui-kit';
import styles from './WatchPane.module.css';

// In Wails v3 the asset server uses wails://localhost so location.hostname is
// "localhost" and parent=localhost works for Twitch and Kick embed players.
// For any non-http(s) scheme that doesn't match "localhost" (shouldn't happen in
// practice), open the stream natively via App.OpenStreamWindow (v3 multi-window).
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

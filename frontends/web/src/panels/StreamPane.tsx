import { Text } from '@virta/ui-kit';
import styles from './StreamPane.module.css';

// Embeds a platform's web player for one channel. Twitch requires the embedding host in `parent`
// (works on localhost / the web build; a packaged webview host must be allowlisted — tracked).
// Kick's player takes the slug directly. X has no embeddable live player.
function embedUrl(platform: string, slug: string): string | null {
  switch (platform) {
    case 'twitch':
      return `https://player.twitch.tv/?channel=${encodeURIComponent(slug)}&parent=${location.hostname}&muted=true`;
    case 'kick':
      return `https://player.kick.com/${encodeURIComponent(slug)}`;
    default:
      return null;
  }
}

export default function StreamPane({ channel }: { channel?: string }) {
  if (!channel) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">
          Open a stream from the Streams panel.
        </Text>
      </div>
    );
  }
  const [platform, slug = ''] = channel.split(':');
  const url = embedUrl(platform, slug);
  if (!url) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">
          No embeddable player for {platform}.
        </Text>
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

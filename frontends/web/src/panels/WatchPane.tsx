import { useState } from 'react';
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

function nativeUrl(platform: string, slug: string): string | null {
  switch (platform) {
    case 'twitch': return `https://twitch.tv/${encodeURIComponent(slug)}`;
    case 'kick':   return `https://kick.com/${encodeURIComponent(slug)}`;
    default:       return null;
  }
}

export default function WatchPane({ channel }: { channel?: string }) {
  const isDesktop = useIsDesktop();
  const [embedFailed, setEmbedFailed] = useState(false);

  if (!channel) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">Open a stream from the Streams panel.</Text>
      </div>
    );
  }

  const [platform, slug = ''] = channel.split(':');
  const embed = embedUrl(platform, slug);
  const native = nativeUrl(platform, slug);
  const label = `${slug} on ${platform.charAt(0).toUpperCase() + platform.slice(1)}`;

  if (!embed) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle">No player available for {platform}.</Text>
      </div>
    );
  }

  // In the desktop app (WebKitGTK), Twitch's IVS player requires WebGPU / WebCodecs
  // which are not available. Show the embed anyway (works if the system ever gains
  // support) but always offer a reliable "open in browser" button.
  if (isDesktop && (embedFailed || !embed)) {
    return (
      <div className={styles.placeholder}>
        <Text variant="ui" tone="subtle" as="p" className={styles.desktopNote}>
          Video playback requires a browser with WebGPU support.
        </Text>
        {native && (
          <button
            type="button"
            className={styles.openBtn}
            onClick={() => void window.wails?.Browser?.OpenURL?.(native)}
          >
            <Icon name="popout" size={14} />
            Watch {label}
          </button>
        )}
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
        onError={() => setEmbedFailed(true)}
      />
      {/* In the desktop app, always show an escape hatch since WebKitGTK
          may not support the video codec the player needs. */}
      {isDesktop && native && (
        <div className={styles.desktopBar}>
          <button
            type="button"
            className={styles.desktopBarBtn}
            onClick={() => void window.wails?.Browser?.OpenURL?.(native)}
          >
            <Icon name="popout" size={12} />
            Open in browser
          </button>
        </div>
      )}
    </div>
  );
}

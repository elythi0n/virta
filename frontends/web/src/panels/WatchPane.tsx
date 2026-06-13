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

// canEmbed reports whether the platform's player iframe will render in this context. Both the
// browser build and the Electron desktop build run Chromium served from an http origin (the dev
// server / daemon in the browser, http://localhost in the desktop shell), and the desktop shell
// strips the players' frame-ancestors CSP, so Twitch and Kick embed inline everywhere. Only
// platforms without an embeddable player (YouTube) fall back to opening the page.
function canEmbed(hasEmbed: boolean): boolean {
  return hasEmbed;
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

  // When there's no inline player (YouTube), show a button that opens the channel another way.
  // On the desktop the shell can pop out a native in-app player window via the bound
  // OpenStreamWindow method (loads the page top-level); the browser opens a new tab.
  if (page && !canEmbed(!!embed)) {
    const plat = platformLabel(platform);
    const useNativeWindow = isDesktop && !!window.wails?.Call;
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

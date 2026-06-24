import { Badge, Checkbox, EmptyState, Field, Input, Section, Text } from '@virta/ui-kit';
import Icon from '../../Icon';
import type { StreamInfo } from '../../daemon/wire.gen';
import type { DeckChannelState, DeckStatus } from '../../daemon/deck';
import type { AccountInfo } from '../../daemon/wire.gen';
import DeckCategoryField from '../DeckCategoryField';
import { STATUS_PENDING, useDeck } from '../DeckContext';
import styles from './StreamInfoPane.module.css';

// Platform colors mirror the design tokens so the per-channel accent picks the same brand color
// as everywhere else in the app (and adapts when the tokens change).
const PLATFORM_COLOR: Record<string, string> = {
  twitch: 'var(--virta-plat-twitch)',
  kick: 'var(--virta-plat-kick)',
  youtube: 'var(--virta-plat-youtube)',
  x: 'var(--virta-plat-x)',
};

const TITLE_MAX = 140;

// StreamInfoPane: the editable broadcast metadata (title, category, tags) plus the channel
// selector and the OBS auto-start toggle. The big "Go live" CTA lives in the deck hero — pulled
// out of this pane so it's always visible even when the user rearranges or hides the form.
export default function StreamInfoPane() {
  const {
    title,
    setTitle,
    tagsInput,
    setTagsInput,
    tags,
    category,
    setCategory,
    prefilled,
    accounts,
    accountsError,
    currentStates,
    selected,
    toggleChannel,
    targets,
    broadcastablePlatforms,
    streams,
    isSyncing,
    scopeErrors,
    submit,
    obsConnected,
    obsStreaming,
    startObsAfterSync,
    setStartObsAfterSync,
  } = useDeck();

  const invalid = invalidTags(tags);

  return (
    <div className={styles.pane}>
      {scopeErrors.length > 0 && (
        <div className={styles.banner} role="alert">
          <Icon name="ban" size={16} />
          <div className={styles.bannerBody}>
            <Text variant="ui">Some platforms need to be re-authorized</Text>
            <Text variant="meta" tone="subtle">
              The {scopeErrors.map((e) => e.platform).join(' + ')} sign-in is missing the
              channel-update scope. Open Settings → Connections, disconnect, and sign in again.
            </Text>
          </div>
        </div>
      )}

      <Section
        title="Stream info"
        badge={1}
        meta={prefilled && currentStates ? 'Pre-filled from current' : undefined}
      >
        <Field
          label="Title"
          labelMeta={`${title.length}/${TITLE_MAX}`}
          htmlFor="deck-title"
        >
          <Input
            id="deck-title"
            placeholder="What's the stream about?"
            value={title}
            onChange={(e) => setTitle(e.currentTarget.value)}
            maxLength={TITLE_MAX}
            disabled={isSyncing}
          />
        </Field>

        <Field
          label="Category"
          hint="Resolves the same category on every platform. Leave blank to keep the current one."
        >
          <DeckCategoryField
            platforms={broadcastablePlatforms}
            selected={category}
            onChange={setCategory}
            disabled={isSyncing}
          />
        </Field>

        <Field
          label="Tags"
          htmlFor="deck-tags"
          hint="Comma-separated. Twitch accepts up to 10 (alphanumeric, 1–25 chars); Kick manages tags from its dashboard so they're skipped there."
          error={invalid.length > 0 ? `Twitch will reject: ${invalid.join(', ')}` : undefined}
        >
          <Input
            id="deck-tags"
            placeholder="english, speedrun, chill"
            value={tagsInput}
            onChange={(e) => setTagsInput(e.currentTarget.value)}
            disabled={isSyncing}
          />
        </Field>
      </Section>

      <Section
        title="Channels"
        badge={2}
        meta={`${targets.length} of ${accounts?.length ?? 0} selected`}
      >
        {renderChannels({
          accounts,
          accountsError,
          selected,
          onToggle: toggleChannel,
          currentStates,
          streams,
          disabled: isSyncing,
        })}
      </Section>

      <Section title="OBS auto-start" badge={3}>
        <label
          className={styles.obsToggle}
          data-disabled={!obsConnected || obsStreaming ? 'true' : 'false'}
        >
          <Checkbox
            checked={startObsAfterSync}
            onChange={(e) => setStartObsAfterSync(e.currentTarget.checked)}
            disabled={!obsConnected || obsStreaming}
          />
          <span className={styles.obsToggleBody}>
            <span className={styles.obsToggleTitle}>Start OBS stream after sync</span>
            <span className={styles.obsToggleHint}>
              {obsStreaming
                ? 'Already streaming.'
                : obsConnected
                ? 'OBS will start broadcasting once every channel sync settles.'
                : 'Connect OBS from Settings → OBS to enable this.'}
            </span>
          </span>
        </label>

        {(submit.kind === 'running' || submit.kind === 'done') && (
          <div className={styles.results}>
            <div className={styles.resultsHead}>
              {submit.kind === 'running' ? 'Syncing…' : 'Last sync'}
            </div>
            <ul className={styles.resultList}>
              {submit.results.map((r) => (
                <li key={r.channel} className={styles.resultRow}>
                  <span className={styles.resultChannel}>{r.channel}</span>
                  {r.status === STATUS_PENDING ? (
                    <span className={styles.resultPending} aria-label="syncing">
                      <span className={styles.spinner} />
                    </span>
                  ) : (
                    <Badge tone={badgeTone(r.status)}>{statusLabel(r.status)}</Badge>
                  )}
                  {r.reason && (
                    <Text variant="meta" tone="subtle" className={styles.resultReason}>
                      {r.reason}
                    </Text>
                  )}
                </li>
              ))}
            </ul>
          </div>
        )}
      </Section>
    </div>
  );
}

function renderChannels({
  accounts,
  accountsError,
  selected,
  onToggle,
  currentStates,
  streams,
  disabled,
}: {
  accounts: AccountInfo[] | null;
  accountsError: string | null;
  selected: Set<string>;
  onToggle: (key: string) => void;
  currentStates: DeckChannelState[] | null;
  streams: Record<string, StreamInfo>;
  disabled: boolean;
}) {
  if (accountsError) {
    return (
      <EmptyState
        variant="plain"
        title="Couldn't load accounts"
        hint={accountsError}
      />
    );
  }
  if (accounts === null) {
    return <EmptyState variant="plain" title="Loading accounts…" />;
  }
  if (accounts.length === 0) {
    return (
      <EmptyState
        variant="plain"
        title="No broadcastable channels"
        hint="Sign in to Twitch or Kick from Settings → Connections to broadcast from here."
      />
    );
  }
  const stateByKey = new Map((currentStates ?? []).map((s) => [s.channel, s] as const));
  return (
    <ul className={styles.channelGrid}>
      {accounts.map((a) => {
        const key = `${a.platform}:${a.login}`;
        const cs = stateByKey.get(key);
        const stream = streams[key];
        const subline = cs?.status === 'ok' && cs.title ? cs.title : cs?.reason;
        const isSelected = selected.has(key);
        const color = PLATFORM_COLOR[a.platform];
        return (
          <li
            key={a.id}
            className={`${styles.channelCard} ${isSelected ? styles.channelCardActive : ''}`}
            style={color ? ({ ['--platform-color' as string]: color } as React.CSSProperties) : undefined}
            onClick={() => !disabled && onToggle(key)}
          >
            <Checkbox
              className={styles.channelCheck}
              checked={isSelected}
              onChange={() => onToggle(key)}
              disabled={disabled}
              onClick={(e) => e.stopPropagation()}
            />
            <div className={styles.channelBody}>
              <div className={styles.channelTop}>
                <span className={styles.channelLogin}>{a.display_name || a.login}</span>
                <LiveDot stream={stream} />
              </div>
              <span className={styles.channelPlatform}>{a.platform}</span>
              {subline && <span className={styles.channelHint}>{subline}</span>}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function LiveDot({ stream }: { stream: StreamInfo | undefined }) {
  if (!stream) return null;
  if (!stream.live) return <span className={styles.liveOff} aria-label="offline" />;
  return (
    <span className={styles.livePill} aria-label={`live with ${stream.viewer_count} viewers`}>
      <span className={styles.liveDot} />
      <span>{formatViewers(stream.viewer_count)}</span>
    </span>
  );
}

function formatViewers(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
  return Math.round(n / 1000) + 'k';
}

function badgeTone(status: string): 'ok' | 'warn' | 'neutral' | 'accent' {
  switch (status as DeckStatus) {
    case 'ok':
      return 'ok';
    case 'error':
      return 'warn';
    case 'excluded':
    case 'unsupported':
      return 'neutral';
    default:
      return 'accent';
  }
}

function statusLabel(status: string): string {
  switch (status as DeckStatus) {
    case 'ok':
      return 'Synced';
    case 'error':
      return 'Failed';
    case 'excluded':
      return 'Skipped';
    case 'unsupported':
      return 'Not supported';
    default:
      return status;
  }
}

// Twitch tag rules: alphanumeric, 1–25 chars, no spaces. Filtered out before submit but also
// surfaced inline so the user sees which ones won't make it.
function invalidTags(tags: string[]): string[] {
  return tags.filter((t) => !/^[A-Za-z0-9]{1,25}$/.test(t));
}

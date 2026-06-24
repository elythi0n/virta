import { Badge, Button, Dialog } from '@virta/ui-kit';
import Icon from '../Icon';
import type { OBSStatus } from '../daemon/obsws';
import type { DeckChannelState } from '../daemon/deck';
import { useDeck } from './DeckContext';
import styles from './DeckHero.module.css';

// Hero chrome for the Deck workspace: the always-visible header that brands the workspace, shows
// live state at a glance (channels selected, channels currently live, OBS state), and carries the
// big "Go live" CTA. The CTA lives here — not inside the dockview — so the streamer can rearrange
// the panes underneath without ever losing the action button.

export default function DeckHero() {
  const {
    targets,
    liveCount,
    obsStatus,
    obsStreaming,
    canSync,
    isSyncing,
    onSyncClick,
    obsConnected,
    startObsAfterSync,
    confirmOpen,
    setConfirmOpen,
    runSync,
    title,
    category,
    tags,
    currentStates,
  } = useDeck();

  const armed = startObsAfterSync && obsConnected && !obsStreaming;
  // The broadcast badge flips from the standby red to a healthy green the moment any selected
  // channel goes live or OBS reports it's pushing — a quiet "you're on air, all good" signal
  // distinct from the CTA's red (which is the call-to-action, not a status).
  const isLive = liveCount > 0 || obsStreaming;
  // Bake the count into the label so the CTA reads as one self-explanatory action — no separate
  // count chip needed. When disabled, the label still describes the action so the user knows what
  // it would do once they fill the form.
  const ctaLabel = isSyncing
    ? 'Syncing…'
    : armed
    ? `Go live${targets.length > 0 ? ` · ${targets.length}` : ''}`
    : targets.length > 0
    ? `Push to ${targets.length} ${targets.length === 1 ? 'channel' : 'channels'}`
    : 'Push to channels';

  return (
    <header className={styles.hero}>
      <div className={styles.heroLeft}>
        <span className={styles.heroBadge} data-live={isLive ? 'true' : 'false'} aria-hidden="true">
          <Icon name="deck" size={24} />
        </span>
        <div className={styles.heroText}>
          <span className={styles.heroTitle}>Broadcast</span>
          <span className={styles.heroSub}>
            One workspace for the title push, the chat, and everything that screams while you stream.
          </span>
        </div>
      </div>

      <div className={styles.heroRight}>
        <div className={styles.statBar} role="group" aria-label="Broadcast status">
          <div className={styles.cell} title="Channels selected">
            <Icon name="stream" size={14} className={styles.cellIcon} />
            <span className={styles.cellValue}>{targets.length}</span>
            <span className={styles.cellLabel}>
              {targets.length === 1 ? 'channel' : 'channels'}
            </span>
          </div>
          {liveCount > 0 && (
            <div className={`${styles.cell} ${styles.cellLive}`} title="Channels currently live">
              <span className={styles.liveDot} aria-hidden="true" />
              <span className={styles.cellValue}>{liveCount}</span>
              <span className={styles.cellLabel}>live</span>
            </div>
          )}
          <div className={styles.cell} title={obsLabel(obsStatus, obsStreaming)}>
            <ObsDot status={obsStatus} streaming={obsStreaming} />
            <span className={styles.cellLabel}>{obsLabel(obsStatus, obsStreaming)}</span>
          </div>
        </div>
        <button
          type="button"
          className={styles.cta}
          onClick={onSyncClick}
          disabled={!canSync}
          aria-label={ctaLabel}
          data-armed={armed ? 'true' : 'false'}
        >
          <Icon name="deck" size={15} />
          <span>{ctaLabel}</span>
        </button>
      </div>

      <Dialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Confirm sync"
        description={`Pushing to ${targets.length} channel${targets.length === 1 ? '' : 's'}. Review the changes before they go live.`}
        size="md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmOpen(false)}>Cancel</Button>
            <Button variant="solid" onClick={() => void runSync()}>Sync now</Button>
          </>
        }
      >
        <ul className={styles.diffList}>
          {targets.map((channel) => (
            <DiffRow
              key={channel}
              channel={channel}
              title={title}
              category={category?.name ?? ''}
              tags={tags}
              currentStates={currentStates}
            />
          ))}
        </ul>
      </Dialog>
    </header>
  );
}

function DiffRow({
  channel,
  title,
  category,
  tags,
  currentStates,
}: {
  channel: string;
  title: string;
  category: string;
  tags: string[];
  currentStates: DeckChannelState[] | null;
}) {
  const cur = (currentStates ?? []).find((s) => s.channel === channel);
  const platform = channel.split(':')[0];
  const newTitle = title.trim();
  const newCat = category.trim();
  const fromTags = cur?.tags ?? [];
  const titleChanged = !!newTitle && (!cur || cur.title !== newTitle);
  const catChanged = !!newCat && (!cur || cur.category !== newCat);
  const tagsChanged = platform !== 'kick' && tags.length > 0 && !sameSet(fromTags, tags);
  const noChange = !titleChanged && !catChanged && !tagsChanged;

  return (
    <li className={styles.diffRow}>
      <div className={styles.diffHead}>
        <span className={styles.diffChannel}>{channel}</span>
        {noChange && <Badge tone="neutral">No change</Badge>}
      </div>
      {!noChange && (
        <dl className={styles.diffFields}>
          {titleChanged && (
            <DiffField label="Title" from={cur?.title ?? ''} to={newTitle} />
          )}
          {catChanged && (
            <DiffField label="Category" from={cur?.category ?? ''} to={newCat} />
          )}
          {tagsChanged && (
            <DiffField
              label="Tags"
              from={fromTags.join(', ') || '—'}
              to={tags.join(', ') || '—'}
            />
          )}
        </dl>
      )}
    </li>
  );
}

function DiffField({ label, from, to }: { label: string; from: string; to: string }) {
  return (
    <>
      <dt className={styles.diffLabel}>{label}</dt>
      <dd className={styles.diffValue}>
        <span className={styles.diffFrom}>{from || <em>empty</em>}</span>
        <Icon name="arrow-up" size={12} className={styles.diffArrow} />
        <span className={styles.diffTo}>{to}</span>
      </dd>
    </>
  );
}

function sameSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const sa = new Set(a);
  for (const v of b) if (!sa.has(v)) return false;
  return true;
}

// OBS state expressed as a single status dot + a single label, so it slots into the segmented
// stat bar alongside the channels/live cells without a second border treatment. The dot color
// (and animation) is the only thing that changes between states; the label always reads "OBS …".
function ObsDot({ status, streaming }: { status: OBSStatus | null; streaming: boolean }) {
  let cls = styles.obsDotOff;
  if (status?.state === 'connecting') cls = styles.obsDotConnecting;
  else if (status?.state === 'connected') cls = streaming ? styles.obsDotLive : styles.obsDotOk;
  return <span className={cls} aria-hidden="true" />;
}

function obsLabel(status: OBSStatus | null, streaming: boolean): string {
  if (!status || status.state === 'disconnected' || status.state === 'error') return 'OBS offline';
  if (status.state === 'connecting') return 'OBS connecting';
  return streaming ? 'OBS live' : 'OBS ready';
}

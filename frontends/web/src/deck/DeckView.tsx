import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Button, Checkbox, Dialog, Input, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { listAccounts } from '../daemon/accounts';
import { useStreams } from '../daemon/streams';
import type { AccountInfo, StreamInfo } from '../daemon/wire.gen';
import {
  getDeckChannelInfo,
  updateDeckChannelInfo,
  type DeckChannelState,
  type DeckResult,
  type DeckStatus,
} from '../daemon/deck';
import FeedPanel from '../panels/FeedPanel';
import { getOBSStatus, getOBSStreamStatus, startOBSStream, type OBSStatus, type OBSStreamStatus } from '../daemon/obsws';
import DeckCategoryField, { selectedFromDraft, type SelectedCategory } from './DeckCategoryField';
import { clearDeckDraft, isEmptyDraft, loadDeckDraft, saveDeckDraft } from './deckDraft';
import styles from './DeckView.module.css';

// Deck: the live broadcast control plane. Pushes title/category/tags across every authenticated
// channel in one shot, with the user's current values pre-filled from each platform so editing
// feels like editing — not like re-entering from scratch.

// Platforms with a channel-info update endpoint on their public API. Other accounts appear
// elsewhere in the app but Deck excludes them so the UI never offers an action that can't run.
const SUPPORTED_PLATFORMS = new Set(['twitch', 'kick']);

// Sync state machine. While running, results grows as each per-channel call returns; rows that
// haven't resolved yet sit with status 'pending' so the UI can render a spinner badge instead of
// blank space. Done means every channel has settled.
type SubmitState =
  | { kind: 'idle' }
  | { kind: 'running'; results: DeckResult[] }
  | { kind: 'done'; results: DeckResult[] }
  | { kind: 'error'; message: string };

const STATUS_PENDING = 'pending';

// A per-channel diff between the form's intended values and the platform's current state, shown
// in the confirmation dialog before any request fires.
interface ChannelDiff {
  channel: string;
  current?: DeckChannelState;
  changes: {
    title?: { from: string; to: string };
    category?: { from: string; to: string };
    tags?: { from: string[]; to: string[] };
  };
  noChange: boolean;
}

export default function DeckView() {
  const [accounts, setAccounts] = useState<AccountInfo[] | null>(null);
  const [accountsError, setAccountsError] = useState<string | null>(null);
  const [currentStates, setCurrentStates] = useState<DeckChannelState[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [prefilled, setPrefilled] = useState(false);

  const [title, setTitle] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [category, setCategory] = useState<SelectedCategory | null>(null);

  const [submit, setSubmit] = useState<SubmitState>({ kind: 'idle' });
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [obsStatus, setObsStatus] = useState<OBSStatus | null>(null);
  const [obsStream, setObsStream] = useState<OBSStreamStatus | null>(null);
  const [startObsAfterSync, setStartObsAfterSync] = useState(false);
  const streams = useStreams();

  // Poll OBS state every 5s while Deck is mounted. Cheap (local-host call), and lets the
  // connection pill + start-stream checkbox track reality without a refresh.
  useEffect(() => {
    let alive = true;
    const poll = async () => {
      try {
        const s = await getOBSStatus();
        if (alive) setObsStatus(s);
        if (s.state === 'connected') {
          try {
            const ss = await getOBSStreamStatus();
            if (alive) setObsStream(ss);
          } catch {
            if (alive) setObsStream(null);
          }
        } else if (alive) {
          setObsStream(null);
        }
      } catch {
        if (alive) setObsStatus(null);
      }
    };
    void poll();
    const id = window.setInterval(poll, 5000);
    return () => {
      alive = false;
      window.clearInterval(id);
    };
  }, []);

  const obsConnected = obsStatus?.state === 'connected';
  const obsStreaming = obsStream?.active === true;

  // Restore a draft synchronously on first render so the user's in-progress work isn't replaced
  // by the pre-fill fetch finishing later. A draft we restore here suppresses prefill below.
  const draftLoadedRef = useRef(false);
  if (!draftLoadedRef.current) {
    draftLoadedRef.current = true;
    const d = loadDeckDraft();
    if (d && !isEmptyDraft(d)) {
      setTitle(d.title);
      setTagsInput(d.tagsInput);
      setSelected(new Set(d.selectedChannels));
      const cat = selectedFromDraft(d.selectedCategoryName, d.categoryIds);
      if (cat) setCategory(cat);
    }
  }

  // Persist drafts on every meaningful change so navigating away (or hot-reloading) doesn't lose
  // edits. Debounced so we don't thrash localStorage on every keystroke.
  useEffect(() => {
    const t = window.setTimeout(() => {
      const ids: Record<string, string> = {};
      if (category) {
        for (const [p, c] of Object.entries(category.perPlatform)) ids[p] = c.id;
      }
      saveDeckDraft({
        title,
        tagsInput,
        selectedChannels: Array.from(selected),
        selectedCategoryName: category?.name ?? '',
        categoryIds: ids,
      });
    }, 300);
    return () => window.clearTimeout(t);
  }, [title, tagsInput, selected, category]);

  // Load accounts; default selection picks every broadcastable channel — the common case is
  // "push to all." A draft restore above already populated `selected` if one exists, so we don't
  // overwrite it here unless selected is empty (first-ever open).
  useEffect(() => {
    let alive = true;
    listAccounts()
      .then((list) => {
        if (!alive) return;
        const broadcastable = list.filter((a) => SUPPORTED_PLATFORMS.has(a.platform));
        setAccounts(broadcastable);
        setSelected((cur) => {
          if (cur.size > 0) return cur; // honour a restored draft's selection
          return new Set(broadcastable.map((a) => `${a.platform}:${a.login}`));
        });
      })
      .catch((err: unknown) => {
        if (!alive) return;
        setAccountsError(err instanceof Error ? err.message : 'failed to load accounts');
      });
    return () => {
      alive = false;
    };
  }, []);

  // Once accounts arrive, fetch the current per-channel state so the form pre-fills with what
  // the streamer already has live. Only prefill when the form is still empty (no draft restored)
  // so we never overwrite the user's in-progress work.
  useEffect(() => {
    if (!accounts || accounts.length === 0) return;
    let alive = true;
    const keys = accounts.map((a) => `${a.platform}:${a.login}`);
    getDeckChannelInfo(keys)
      .then((states) => {
        if (!alive) return;
        setCurrentStates(states);
        const formIsEmpty = !title.trim() && !tagsInput.trim() && !category;
        if (formIsEmpty) {
          const first = states.find((s) => s.status === 'ok' && (s.title || s.category));
          if (first) {
            setTitle(first.title ?? '');
            setTagsInput((first.tags ?? []).join(', '));
            if (first.category) {
              const platform = first.channel.split(':')[0];
              setCategory({
                name: first.category,
                perPlatform: { [platform]: { id: first.category_id ?? '', name: first.category } },
              });
            }
            setPrefilled(true);
          }
        }
      })
      .catch(() => {
        // Pre-fill is best-effort; a fetch failure just leaves the form empty.
      });
    return () => {
      alive = false;
    };
    // We deliberately only react to accounts arriving, not to every title/tags change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accounts]);

  const tags = useMemo(
    () =>
      tagsInput
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean),
    [tagsInput],
  );

  const targets = useMemo(() => Array.from(selected).sort(), [selected]);
  const isSyncing = submit.kind === 'running';
  const canSync = title.trim().length > 0 && targets.length > 0 && !isSyncing;

  const broadcastablePlatforms = useMemo(
    () => Array.from(new Set((accounts ?? []).map((a) => a.platform))),
    [accounts],
  );

  // Scope the embedded chat to every broadcastable channel the user is signed into. When nothing
  // is signed in, the FeedPanel itself shows its own empty state, so we still mount it.
  const chatChannels = useMemo(
    () => (accounts ?? []).map((a) => `${a.platform}:${a.login}`),
    [accounts],
  );

  const toggleChannel = useCallback((key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  // Mark the form as "user-edited" so we stop labelling fields as pre-filled.
  const onTitleChange = useCallback((v: string) => {
    setTitle(v);
    setPrefilled(false);
  }, []);
  const onTagsChange = useCallback((v: string) => {
    setTagsInput(v);
    setPrefilled(false);
  }, []);
  const onCategoryChange = useCallback((c: SelectedCategory | null) => {
    setCategory(c);
    setPrefilled(false);
  }, []);

  // Per-target diff between the form's intended values and the platform's current state. Powers
  // the confirmation dialog: the user sees exactly which fields will change on each channel
  // before any request fires. Channels with no current state (excluded / fetch failed) just show
  // the new values so we don't pretend to know what's there now.
  const diffs = useMemo<ChannelDiff[]>(() => {
    const stateByKey = new Map((currentStates ?? []).map((s) => [s.channel, s] as const));
    const newTitle = title.trim();
    const newCategory = category?.name ?? '';
    const newTags = tags;
    return targets.map((channel) => {
      const cur = stateByKey.get(channel);
      const changes: ChannelDiff['changes'] = {};
      if (newTitle && (!cur || cur.title !== newTitle)) {
        changes.title = { from: cur?.title ?? '', to: newTitle };
      }
      if (newCategory && (!cur || cur.category !== newCategory)) {
        changes.category = { from: cur?.category ?? '', to: newCategory };
      }
      // Kick has no tag list, so a Kick-only channel never reports a tags diff (the patch skips
      // tags for Kick targets anyway). Compare set-wise, not order-wise.
      const platform = channel.split(':')[0];
      if (newTags.length && platform !== 'kick') {
        const fromTags = cur?.tags ?? [];
        if (!sameSet(fromTags, newTags)) {
          changes.tags = { from: fromTags, to: newTags };
        }
      }
      return {
        channel,
        current: cur,
        changes,
        noChange: Object.keys(changes).length === 0,
      };
    });
  }, [targets, title, category, tags, currentStates]);

  const onSyncClick = useCallback(() => {
    if (!canSync) return;
    setConfirmOpen(true);
  }, [canSync]);

  const runSync = useCallback(async () => {
    if (!canSync) return;
    setConfirmOpen(false);
    // Seed one pending row per target so the user sees the work in flight; each gets replaced
    // as its individual call settles.
    const initial: DeckResult[] = targets.map((channel) => ({ channel, status: STATUS_PENDING }));
    setSubmit({ kind: 'running', results: initial });

    const categoryIds: Record<string, string> = {};
    if (category) {
      for (const [p, c] of Object.entries(category.perPlatform)) {
        if (c.id) categoryIds[p] = c.id;
      }
    }
    const info = {
      title: title.trim(),
      category: category?.name || undefined,
      category_ids: Object.keys(categoryIds).length ? categoryIds : undefined,
      tags: tags.length ? tags : undefined,
    };

    // Fire one request per channel in parallel. Each resolves its own row optimistically so the
    // user sees Twitch (~200ms) settle before Kick (~800ms) — instead of waiting on the slowest.
    const settled = await Promise.all(
      targets.map(async (channel): Promise<DeckResult> => {
        try {
          const res = await updateDeckChannelInfo([channel], info);
          // The backend returns one result per requested target; defensively fall back to a
          // synthetic "error" row if the server returns an unexpected shape.
          const row = res[0] ?? { channel, status: 'error', reason: 'no result returned' };
          setSubmit((cur) => (cur.kind === 'running' ? { ...cur, results: replaceRow(cur.results, row) } : cur));
          return row;
        } catch (err) {
          const row: DeckResult = {
            channel,
            status: 'error',
            reason: err instanceof Error ? err.message : 'sync failed',
          };
          setSubmit((cur) => (cur.kind === 'running' ? { ...cur, results: replaceRow(cur.results, row) } : cur));
          return row;
        }
      }),
    );

    setSubmit({ kind: 'done', results: settled });

    const cleanSync = settled.every((r) => r.status === 'ok' || r.status === 'excluded' || r.status === 'unsupported');

    // A clean sync resets the local draft — the next open should reflect server state.
    if (cleanSync) {
      clearDeckDraft();
    }

    // Close the loop: if the user opted in and metadata landed everywhere, start the OBS
    // broadcast. Failure here is reported alongside the per-channel results — it's a follow-up
    // step, not the sync itself, so we don't pretend the sync failed.
    if (cleanSync && startObsAfterSync && obsConnected && !obsStreaming) {
      try {
        await startOBSStream();
        const ss = await getOBSStreamStatus();
        setObsStream(ss);
      } catch (err) {
        setSubmit({
          kind: 'done',
          results: [
            ...settled,
            {
              channel: 'obs',
              status: 'error',
              reason: err instanceof Error ? `OBS: ${err.message}` : 'OBS: stream-start failed',
            },
          ],
        });
      }
    }
    // Re-fetch current state so the channel-row hints match what we just pushed.
    if (accounts) {
      const keys = accounts.map((a) => `${a.platform}:${a.login}`);
      getDeckChannelInfo(keys)
        .then((states) => setCurrentStates(states))
        .catch(() => {});
    }
  }, [canSync, targets, title, category, tags, accounts, startObsAfterSync, obsConnected, obsStreaming]);

  // Per-platform scope-error banner: surfaces results whose error reason looks like a missing
  // OAuth scope (the common case after upgrading: an existing token doesn't have the new
  // channel:manage:broadcast / channel:write scope yet).
  const scopeErrors = useMemo(() => {
    if (submit.kind !== 'done' && submit.kind !== 'running') return [] as { channel: string; platform: string }[];
    return submit.results
      .filter((r) => r.status === 'error' && isScopeError(r.reason))
      .map((r) => ({ channel: r.channel, platform: r.channel.split(':')[0] }));
  }, [submit]);

  return (
    <div className={styles.deck}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <Icon name="deck" size={18} />
          <Text variant="title">Deck</Text>
        </div>
        <div className={styles.headerRight}>
          <ObsPill status={obsStatus} streaming={obsStreaming} />
          <Text variant="meta" tone="subtle">Go live</Text>
        </div>
      </header>

      <div className={styles.workspace}>
        <section className={styles.formColumn}>
        <div className={styles.form}>
          <div className={styles.formHead}>
            <Text variant="heading">Set stream info</Text>
            <Text variant="body" tone="subtle">
              Push title, category, and tags to every authenticated channel in one shot.
            </Text>
            {prefilled && currentStates && (
              <Text variant="meta" tone="subtle">
                Pre-filled from your current stream info.
              </Text>
            )}
          </div>

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

          <div className={styles.field}>
            <label className={styles.label} htmlFor="deck-title">Title</label>
            <Input
              id="deck-title"
              placeholder="What's the stream about?"
              value={title}
              onChange={(e) => onTitleChange(e.currentTarget.value)}
              maxLength={140}
              disabled={isSyncing}
            />
          </div>

          <div className={styles.field}>
            <label className={styles.label}>Category</label>
            <DeckCategoryField
              platforms={broadcastablePlatforms}
              selected={category}
              onChange={onCategoryChange}
              disabled={isSyncing}
            />
            <Text variant="meta" tone="subtle">
              Resolves the same category id on each platform that has one. Leave blank to keep the
              current category.
            </Text>
          </div>

          <div className={styles.field}>
            <label className={styles.label} htmlFor="deck-tags">Tags</label>
            <Input
              id="deck-tags"
              placeholder="english, speedrun, chill"
              value={tagsInput}
              onChange={(e) => onTagsChange(e.currentTarget.value)}
              disabled={isSyncing}
            />
            <Text variant="meta" tone="subtle">
              Comma-separated. Twitch accepts up to 10 (alphanumeric, 1–25 chars); Kick manages
              tags from the dashboard so they're skipped there.
            </Text>
            {invalidTags(tags).length > 0 && (
              <Text variant="meta" tone="subtle">
                <span className={styles.invalidTag}>
                  Twitch will reject: {invalidTags(tags).join(', ')}
                </span>
              </Text>
            )}
          </div>

          <div className={styles.channelList}>
            <Text variant="meta" tone="subtle">Channels</Text>
            {renderChannels({
              accounts,
              accountsError,
              selected,
              onToggle: toggleChannel,
              currentStates,
              streams,
            })}
          </div>

          <div className={styles.actions}>
            <Button variant="solid" size="md" disabled={!canSync} onClick={onSyncClick}>
              <Icon name="arrow-up" size={14} />
              {isSyncing ? 'Syncing…' : 'Sync to all platforms'}
            </Button>
            {submit.kind === 'error' && (
              <Text variant="meta" tone="subtle">{submit.message}</Text>
            )}
          </div>

          {/* OBS closes the loop: after a successful metadata sync, optionally trigger the OBS
              broadcast so "Go live" is one action instead of two. Disabled when OBS isn't
              reachable or is already streaming. */}
          <label className={styles.obsToggle}>
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
              <Text variant="meta" tone="subtle">
                {submit.kind === 'running' ? 'Syncing…' : 'Last sync'}
              </Text>
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
        </div>
        </section>

        <aside className={styles.chatRail}>
          <div className={styles.chatHead}>
            <Icon name="chat" size={14} />
            <Text variant="meta" tone="subtle">Chat</Text>
          </div>
          <div className={styles.chatBody}>
            <FeedPanel channels={chatChannels.length ? chatChannels : undefined} panelId="deck-chat" />
          </div>
        </aside>
      </div>

      <Dialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Confirm sync"
        description={`Pushing to ${diffs.length} channel${diffs.length === 1 ? '' : 's'}. Review the changes before they go live.`}
        size="md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmOpen(false)}>Cancel</Button>
            <Button variant="solid" onClick={() => void runSync()}>Sync now</Button>
          </>
        }
      >
        <ul className={styles.diffList}>
          {diffs.map((d) => (
            <DiffRow key={d.channel} diff={d} />
          ))}
        </ul>
      </Dialog>
    </div>
  );
}

// One row in the confirm-diff dialog: the channel name plus the fields that will change. Rows
// with no changes still render — collapsed to a "No change" line — so the user sees every
// channel they're sync'ing to and isn't surprised by a silent skip.
function DiffRow({ diff }: { diff: ChannelDiff }) {
  const { channel, changes, noChange } = diff;
  return (
    <li className={styles.diffRow}>
      <div className={styles.diffHead}>
        <span className={styles.diffChannel}>{channel}</span>
        {noChange && <Badge tone="neutral">No change</Badge>}
      </div>
      {!noChange && (
        <dl className={styles.diffFields}>
          {changes.title && (
            <DiffField label="Title" from={changes.title.from} to={changes.title.to} />
          )}
          {changes.category && (
            <DiffField label="Category" from={changes.category.from} to={changes.category.to} />
          )}
          {changes.tags && (
            <DiffField
              label="Tags"
              from={changes.tags.from.join(', ') || '—'}
              to={changes.tags.to.join(', ') || '—'}
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

function renderChannels({
  accounts,
  accountsError,
  selected,
  onToggle,
  currentStates,
  streams,
}: {
  accounts: AccountInfo[] | null;
  accountsError: string | null;
  selected: Set<string>;
  onToggle: (key: string) => void;
  currentStates: DeckChannelState[] | null;
  streams: Record<string, StreamInfo>;
}) {
  if (accountsError) {
    return (
      <div className={styles.channelEmpty}>
        <Text variant="body" tone="subtle">Couldn't load accounts: {accountsError}</Text>
      </div>
    );
  }
  if (accounts === null) {
    return (
      <div className={styles.channelEmpty}>
        <Text variant="body" tone="subtle">Loading accounts…</Text>
      </div>
    );
  }
  if (accounts.length === 0) {
    return (
      <div className={styles.channelEmpty}>
        <Text variant="body" tone="subtle">
          Sign in to Twitch or Kick from Settings → Connections to broadcast from Deck.
        </Text>
      </div>
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
        return (
          <li key={a.id} className={styles.channelRow}>
            <Checkbox
              checked={selected.has(key)}
              onChange={() => onToggle(key)}
              label={
                <span className={styles.channelLabel}>
                  <span className={styles.channelPlatform}>{a.platform}</span>
                  <span className={styles.channelLogin}>{a.display_name || a.login}</span>
                  <LiveDot stream={stream} />
                </span>
              }
              hint={subline ? <span className={styles.channelHint}>{subline}</span> : undefined}
            />
          </li>
        );
      })}
    </ul>
  );
}

// Replace the row for one channel inside the running-results array. Pure so the state update
// is easy to reason about and the optimistic UI stays in sync with each settled promise.
function replaceRow(rows: DeckResult[], row: DeckResult): DeckResult[] {
  return rows.map((r) => (r.channel === row.channel ? row : r));
}

// OBS connection + stream-state pill in the header. Three modes: not connected (subtle grey),
// connected (green dot), streaming (red dot with duration). Status null = poll hasn't returned
// yet; we render an unobtrusive placeholder so the header doesn't shift when it arrives.
function ObsPill({ status, streaming }: { status: OBSStatus | null; streaming: boolean }) {
  if (!status || status.state === 'disconnected' || status.state === 'error') {
    return (
      <span className={styles.obsPill} title={status?.error || 'OBS not connected'}>
        <span className={styles.obsDotOff} />
        <span>OBS</span>
      </span>
    );
  }
  if (status.state === 'connecting') {
    return (
      <span className={styles.obsPill} title="connecting to OBS">
        <span className={styles.obsDotConnecting} />
        <span>OBS</span>
      </span>
    );
  }
  if (streaming) {
    return (
      <span className={`${styles.obsPill} ${styles.obsPillLive}`} title="OBS broadcast is live">
        <span className={styles.liveDot} />
        <span>OBS live</span>
      </span>
    );
  }
  return (
    <span className={styles.obsPill} title={`OBS ${status.obs_version ?? 'connected'}`}>
      <span className={styles.obsDotOk} />
      <span>OBS ready</span>
    </span>
  );
}

// Live-or-offline pill next to each channel row. Surfaces the stream's live state so the user
// knows which channels are currently broadcasting before they push metadata. Renders nothing when
// stream info isn't available (e.g. the engine hasn't polled yet) so the row stays clean.
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
// surfaced inline so the user sees which ones won't make it instead of being silently rejected.
function invalidTags(tags: string[]): string[] {
  return tags.filter((t) => !/^[A-Za-z0-9]{1,25}$/.test(t));
}

// Order-insensitive equality for tag lists, so reordering tags isn't reported as a diff.
function sameSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const sa = new Set(a);
  for (const v of b) if (!sa.has(v)) return false;
  return true;
}

// Recognises platform error messages that mean "your OAuth token doesn't carry the scope this
// action needs" — a string-pattern match rather than a structured field, because the
// platforms report it differently and the engine surfaces the raw message.
function isScopeError(reason: string | undefined): boolean {
  if (!reason) return false;
  const r = reason.toLowerCase();
  return (
    r.includes('status 401') ||
    r.includes('unauthorized') ||
    r.includes('missing required scope') ||
    r.includes('channel:manage:broadcast') ||
    r.includes('channel:write') ||
    r.includes('insufficient scope')
  );
}

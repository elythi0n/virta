import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { listAccounts } from '../daemon/accounts';
import { useStreams } from '../daemon/streams';
import type { AccountInfo, StreamInfo } from '../daemon/wire.gen';
import {
  getDeckChannelInfo,
  updateDeckChannelInfo,
  type DeckChannelState,
  type DeckResult,
} from '../daemon/deck';
import {
  getOBSStatus,
  getOBSStreamStatus,
  startOBSStream,
  type OBSStatus,
  type OBSStreamStatus,
} from '../daemon/obsws';
import { selectedFromDraft, type SelectedCategory } from './DeckCategoryField';
import { clearDeckDraft, isEmptyDraft, loadDeckDraft, saveDeckDraft } from './deckDraft';

// Platforms with a channel-info update endpoint on their public API. Deck excludes everything
// else so the UI never offers an action that can't run.
export const SUPPORTED_PLATFORMS = new Set(['twitch', 'kick']);

export const STATUS_PENDING = 'pending';

export type SubmitState =
  | { kind: 'idle' }
  | { kind: 'running'; results: DeckResult[] }
  | { kind: 'done'; results: DeckResult[] }
  | { kind: 'error'; message: string };

export interface DeckState {
  // form
  title: string;
  setTitle: (v: string) => void;
  tagsInput: string;
  setTagsInput: (v: string) => void;
  tags: string[];
  category: SelectedCategory | null;
  setCategory: (c: SelectedCategory | null) => void;
  prefilled: boolean;

  // channels
  accounts: AccountInfo[] | null;
  accountsError: string | null;
  currentStates: DeckChannelState[] | null;
  selected: Set<string>;
  toggleChannel: (key: string) => void;
  targets: string[];
  broadcastablePlatforms: string[];
  chatChannels: string[];
  streams: Record<string, StreamInfo>;
  liveCount: number;

  // sync flow
  submit: SubmitState;
  canSync: boolean;
  isSyncing: boolean;
  scopeErrors: { channel: string; platform: string }[];
  onSyncClick: () => void;
  runSync: () => Promise<void>;
  confirmOpen: boolean;
  setConfirmOpen: (v: boolean) => void;

  // OBS auto-start
  obsStatus: OBSStatus | null;
  obsStream: OBSStreamStatus | null;
  obsConnected: boolean;
  obsStreaming: boolean;
  startObsAfterSync: boolean;
  setStartObsAfterSync: (v: boolean) => void;
}

const DeckCtx = createContext<DeckState | null>(null);

export function useDeck(): DeckState {
  const v = useContext(DeckCtx);
  if (!v) throw new Error('useDeck must be used inside <DeckProvider>');
  return v;
}

// Provider for the Deck workspace. Owns title/tags/category/channel-selection plus the sync flow
// state, so the hero CTA and the StreamInfoPane (rendered inside the dock) read from one source.
export function DeckProvider({ children }: { children: ReactNode }) {
  const [accounts, setAccounts] = useState<AccountInfo[] | null>(null);
  const [accountsError, setAccountsError] = useState<string | null>(null);
  const [currentStates, setCurrentStates] = useState<DeckChannelState[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [prefilled, setPrefilled] = useState(false);

  const [title, setTitleState] = useState('');
  const [tagsInput, setTagsInputState] = useState('');
  const [category, setCategoryState] = useState<SelectedCategory | null>(null);

  const [submit, setSubmit] = useState<SubmitState>({ kind: 'idle' });
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [obsStatus, setObsStatus] = useState<OBSStatus | null>(null);
  const [obsStream, setObsStream] = useState<OBSStreamStatus | null>(null);
  const [startObsAfterSync, setStartObsAfterSync] = useState(false);
  const streams = useStreams();

  // Poll OBS state every 5s while Deck is mounted. Cheap (local-host call), and lets the OBS pill
  // + auto-start checkbox track reality without a refresh.
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
  // by the pre-fill fetch finishing later.
  const draftLoadedRef = useRef(false);
  if (!draftLoadedRef.current) {
    draftLoadedRef.current = true;
    const d = loadDeckDraft();
    if (d && !isEmptyDraft(d)) {
      setTitleState(d.title);
      setTagsInputState(d.tagsInput);
      setSelected(new Set(d.selectedChannels));
      const cat = selectedFromDraft(d.selectedCategoryName, d.categoryIds);
      if (cat) setCategoryState(cat);
    }
  }

  // Persist drafts on every meaningful change.
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

  // Load accounts; default selection picks every broadcastable channel — the common case.
  useEffect(() => {
    let alive = true;
    listAccounts()
      .then((list) => {
        if (!alive) return;
        const broadcastable = list.filter((a) => SUPPORTED_PLATFORMS.has(a.platform));
        setAccounts(broadcastable);
        setSelected((cur) => {
          if (cur.size > 0) return cur;
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
  // the streamer already has live. Only when the form is still empty.
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
            setTitleState(first.title ?? '');
            setTagsInputState((first.tags ?? []).join(', '));
            if (first.category) {
              const platform = first.channel.split(':')[0];
              setCategoryState({
                name: first.category,
                perPlatform: { [platform]: { id: first.category_id ?? '', name: first.category } },
              });
            }
            setPrefilled(true);
          }
        }
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
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

  // Embedded chat scopes to every broadcastable channel the user is signed into.
  const chatChannels = useMemo(
    () => (accounts ?? []).map((a) => `${a.platform}:${a.login}`),
    [accounts],
  );

  const liveCount = useMemo(() => {
    let n = 0;
    for (const key of selected) {
      if (streams[key]?.live) n += 1;
    }
    return n;
  }, [selected, streams]);

  const toggleChannel = useCallback((key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  // Setters that drop the "pre-filled" marker so the form stops claiming to mirror server state.
  const setTitle = useCallback((v: string) => {
    setTitleState(v);
    setPrefilled(false);
  }, []);
  const setTagsInput = useCallback((v: string) => {
    setTagsInputState(v);
    setPrefilled(false);
  }, []);
  const setCategory = useCallback((c: SelectedCategory | null) => {
    setCategoryState(c);
    setPrefilled(false);
  }, []);

  const onSyncClick = useCallback(() => {
    if (!canSync) return;
    setConfirmOpen(true);
  }, [canSync]);

  const runSync = useCallback(async () => {
    if (!canSync) return;
    setConfirmOpen(false);
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

    const settled = await Promise.all(
      targets.map(async (channel): Promise<DeckResult> => {
        try {
          const res = await updateDeckChannelInfo([channel], info);
          const row = res[0] ?? { channel, status: 'error', reason: 'no result returned' };
          setSubmit((cur) =>
            cur.kind === 'running' ? { ...cur, results: replaceRow(cur.results, row) } : cur,
          );
          return row;
        } catch (err) {
          const row: DeckResult = {
            channel,
            status: 'error',
            reason: err instanceof Error ? err.message : 'sync failed',
          };
          setSubmit((cur) =>
            cur.kind === 'running' ? { ...cur, results: replaceRow(cur.results, row) } : cur,
          );
          return row;
        }
      }),
    );

    setSubmit({ kind: 'done', results: settled });

    const cleanSync = settled.every(
      (r) => r.status === 'ok' || r.status === 'excluded' || r.status === 'unsupported',
    );
    if (cleanSync) clearDeckDraft();

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
    if (accounts) {
      const keys = accounts.map((a) => `${a.platform}:${a.login}`);
      getDeckChannelInfo(keys)
        .then((states) => setCurrentStates(states))
        .catch(() => {});
    }
  }, [canSync, targets, title, category, tags, accounts, startObsAfterSync, obsConnected, obsStreaming]);

  // Per-platform scope-error banner: results whose error reason matches a missing-scope pattern.
  const scopeErrors = useMemo(() => {
    if (submit.kind !== 'done' && submit.kind !== 'running')
      return [] as { channel: string; platform: string }[];
    return submit.results
      .filter((r) => r.status === 'error' && isScopeError(r.reason))
      .map((r) => ({ channel: r.channel, platform: r.channel.split(':')[0] }));
  }, [submit]);

  const value: DeckState = {
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
    chatChannels,
    streams,
    liveCount,
    submit,
    canSync,
    isSyncing,
    scopeErrors,
    onSyncClick,
    runSync,
    confirmOpen,
    setConfirmOpen,
    obsStatus,
    obsStream,
    obsConnected,
    obsStreaming,
    startObsAfterSync,
    setStartObsAfterSync,
  };

  return <DeckCtx.Provider value={value}>{children}</DeckCtx.Provider>;
}

function replaceRow(rows: DeckResult[], row: DeckResult): DeckResult[] {
  return rows.map((r) => (r.channel === row.channel ? row : r));
}

// Recognises platform error messages that mean "your OAuth token doesn't carry the scope this
// action needs" — a string-pattern match because platforms report it differently and the engine
// surfaces the raw message.
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

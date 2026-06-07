import { useCallback, useEffect, useRef, useState } from 'react';
import { Popover, Text } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import { askStream, getIntelConfig, listModels } from '../daemon';
import { listConversations, saveConversation, deleteConversation, generateTitleStream } from '../daemon/conversations';
import type { ConversationSummary } from '../daemon/conversations';
import type { AskEvent, IntelConfig, ModelGroup } from '../daemon/wire.gen';
import ProviderIcon from '../ProviderIcon';
import Markdown from './Markdown';
import styles from './AskPanel.module.css';

// ── Types ──────────────────────────────────────────────────────────────────
type TurnRole = 'user' | 'assistant';
type TurnItem =
  | { kind: 'text'; role: TurnRole; text: string }
  | { kind: 'tool_use'; name: string; args: string }
  | { kind: 'tool_result'; name: string; json: string }
  | { kind: 'error'; text: string };

// Paired tool turn: a tool_use + its matching tool_result combined for rendering.
type ToolPair = { kind: 'tool'; name: string; args: string; result: string | null };
// Group of consecutive tool calls (shown as a collapsed stack by default).
type ToolGroup = { kind: 'tool_group'; tools: ToolPair[] };
type RenderItem = Exclude<TurnItem, { kind: 'tool_use' } | { kind: 'tool_result' }> | ToolPair | ToolGroup;

// ── Helpers ────────────────────────────────────────────────────────────────
function autoResize(el: HTMLTextAreaElement) {
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 180) + 'px';
}

function flatModels(groups: ModelGroup[]) {
  return groups.flatMap(g => g.models.map(m => ({ ...m, providerId: g.provider_id, providerName: g.display_name })));
}

function newConvId() {
  return `conv_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`;
}

function relativeDate(iso: string): string {
  const d = new Date(iso);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

/** Pair + group tool turns: consecutive tool pairs become a single ToolGroup. */
function pairTools(turns: TurnItem[]): RenderItem[] {
  // Step 1: pair tool_use + tool_result
  const paired: RenderItem[] = [];
  let i = 0;
  while (i < turns.length) {
    const t = turns[i];
    if (t.kind === 'tool_use') {
      const next = turns[i + 1];
      if (next?.kind === 'tool_result' && next.name === t.name) {
        paired.push({ kind: 'tool', name: t.name, args: t.args, result: next.json });
        i += 2; continue;
      }
      paired.push({ kind: 'tool', name: t.name, args: t.args, result: null });
      i++; continue;
    }
    if (t.kind === 'tool_result') { i++; continue; } // orphaned, skip
    paired.push(t as RenderItem);
    i++;
  }

  // Step 2: group consecutive tool pairs into ToolGroup
  const out: RenderItem[] = [];
  let j = 0;
  while (j < paired.length) {
    const item = paired[j];
    if (item.kind === 'tool') {
      // Collect the run of consecutive tools
      const group: ToolPair[] = [item as ToolPair];
      while (j + group.length < paired.length && paired[j + group.length]?.kind === 'tool') {
        group.push(paired[j + group.length] as ToolPair);
      }
      out.push(group.length === 1 ? group[0] : { kind: 'tool_group', tools: group });
      j += group.length;
      continue;
    }
    out.push(item);
    j++;
  }
  return out;
}

// ── Tool card metadata ─────────────────────────────────────────────────────
const TOOL_ICON: Record<string, IconName> = {
  search_messages: 'search',
  get_user_history: 'clock',
  top_chatters: 'mentions',
  channel_stats: 'stats',
  get_messages_range: 'list',
  list_channels: 'stream',
};

const TOOL_LABEL: Record<string, string> = {
  search_messages: 'Search messages',
  get_user_history: 'User history',
  top_chatters: 'Top chatters',
  channel_stats: 'Channel stats',
  get_messages_range: 'Message range',
  list_channels: 'List channels',
};

function toolIcon(name: string): IconName  { return TOOL_ICON[name] ?? 'search'; }
function toolLabel(name: string): string   { return TOOL_LABEL[name] ?? name.replace(/_/g, ' '); }

function tryPretty(s: string): string {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

function isToolError(json: string): boolean {
  try { const p = JSON.parse(json); return typeof p === 'object' && p !== null && 'error' in p; } catch { return false; }
}

function toolArgSummary(argsJson: string): string {
  try {
    const obj = JSON.parse(argsJson);
    const entries = Object.entries(obj).filter(([, v]) => v !== undefined && v !== '');
    if (entries.length === 0) return '';
    const [k, v] = entries[0];
    const val = String(v);
    return `${k}: ${val.length > 40 ? val.slice(0, 40) + '…' : val}`;
  } catch { return ''; }
}

// ── Components ────────────────────────────────────────────────────────────
function ToolCard({ name, args, result }: { name: string; args: string; result: string | null }) {
  const [open, setOpen] = useState(false);
  const pending = result === null;
  const hasError = !pending && isToolError(result);
  const summary = toolArgSummary(args);

  let argEntries: [string, unknown][] = [];
  try { argEntries = Object.entries(JSON.parse(args)); } catch { /* ignore */ }

  return (
    <div className={`${styles.toolCard} ${hasError ? styles.toolCardError : ''}`}>
      <button
        type="button"
        className={styles.toolHeader}
        onClick={() => !pending && setOpen(o => !o)}
        disabled={pending}
        aria-expanded={open}
      >
        <span className={styles.toolIconWrap}>
          <Icon name={toolIcon(name)} size={13} />
        </span>
        <span className={styles.toolName}>{toolLabel(name)}</span>
        {summary && !open && <span className={styles.toolSummaryChip}>{summary}</span>}
        <span className={`${styles.toolStatus} ${pending ? styles.toolStatusPending : hasError ? styles.toolStatusError : styles.toolStatusDone}`}>
          {pending ? (
            <><span className={styles.spinner} aria-hidden />Running</>
          ) : hasError ? (
            <>✕ Error</>
          ) : (
            <>✓ Done</>
          )}
        </span>
        {!pending && (
          <Icon name="chevron-down" size={12} className={open ? styles.chevronOpen : styles.chevron} />
        )}
      </button>

      {open && result !== null && (
        <div className={styles.toolBody}>
          {argEntries.length > 0 && (
            <div className={styles.toolSection}>
              <div className={styles.toolSectionLabel}>Input</div>
              <dl className={styles.toolArgList}>
                {argEntries.map(([k, v]) => (
                  <div key={k} className={styles.toolArgRow}>
                    <dt className={styles.toolArgKey}>{k}</dt>
                    <dd className={styles.toolArgVal}>{String(v ?? '—')}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
          <div className={styles.toolSection}>
            <div className={styles.toolSectionLabel}>{hasError ? 'Error' : 'Output'}</div>
            <pre className={`${styles.toolResult} ${hasError ? styles.toolResultError : ''}`}>
              {tryPretty(result)}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}

// Groups ≥2 consecutive tool calls into a single collapsible stack.
function ToolGroupCard({ tools }: { tools: ToolPair[] }) {
  const [open, setOpen] = useState(false);
  const doneCount  = tools.filter(t => t.result !== null && !isToolError(t.result)).length;
  const errorCount = tools.filter(t => t.result !== null && isToolError(t.result)).length;
  const pendingCount = tools.filter(t => t.result === null).length;

  return (
    <div className={styles.toolGroup}>
      <button type="button" className={styles.toolGroupHeader} onClick={() => setOpen(o => !o)}>
        <div className={styles.toolGroupIcons}>
          {tools.map((t, i) => (
            <span key={i} className={`${styles.toolGroupIcon} ${isToolError(t.result ?? '') ? styles.toolGroupIconError : t.result === null ? styles.toolGroupIconPending : styles.toolGroupIconDone}`}>
              <Icon name={toolIcon(t.name)} size={12} />
            </span>
          ))}
        </div>
        <span className={styles.toolGroupLabel}>
          {tools.length} tool{tools.length !== 1 ? 's' : ''}
        </span>
        <div className={styles.toolGroupBadges}>
          {doneCount > 0    && <span className={styles.toolGroupDone}>{doneCount} ✓</span>}
          {errorCount > 0   && <span className={styles.toolGroupError}>{errorCount} ✕</span>}
          {pendingCount > 0 && <span className={styles.toolGroupPending}>{pendingCount} …</span>}
        </div>
        <Icon name="chevron-down" size={12} className={open ? styles.chevronOpen : styles.chevron} />
      </button>

      {open && (
        <div className={styles.toolGroupBody}>
          {tools.map((t, i) => (
            <ToolCard key={i} name={t.name} args={t.args} result={t.result} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── Main panel ────────────────────────────────────────────────────────────
export default function AskPanel() {
  const [config, setConfig] = useState<IntelConfig | null>(null);
  const [groups, setGroups] = useState<ModelGroup[]>([]);
  const [modelsReady, setModelsReady] = useState(false);
  const [model, setModel] = useState('');
  const [modelOpen, setModelOpen] = useState(false);
  const [question, setQuestion] = useState('');
  const [turns, setTurns] = useState<TurnItem[]>([]);
  const [running, setRunning] = useState(false);
  const [tokens, setTokens] = useState({ in: 0, out: 0 });

  const [convId, setConvId] = useState(() => newConvId());
  const [convTitle, setConvTitle] = useState('New conversation');
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [convOpen, setConvOpen] = useState(false);
  const titleStreamingRef = useRef(false);

  const abortRef = useRef<(() => void) | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    getIntelConfig()
      .then(cfg => { setConfig(cfg); if (cfg.selected_model) setModel(cfg.selected_model); })
      .catch(() => {});

    listModels().then(g => {
      setGroups(g);
      setModelsReady(true);
      const all = flatModels(g);
      if (all.length === 0) return;
      setModel(prev => {
        if (prev && all.some(m => m.id === prev)) return prev;
        return all.find(m => m.supports_tools)?.id ?? all[0]?.id ?? prev;
      });
    }).catch(() => setModelsReady(true));

    listConversations().then(setConversations).catch(() => {});
  }, []);

  const scrollBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  const saveTurns = useCallback((nextTurns: TurnItem[], currentModel: string, title: string) => {
    void saveConversation({ id: convId, title, model: currentModel, messages: nextTurns }).catch(() => {});
  }, [convId]);

  const generateTitle = useCallback((firstUserMessage: string, currentModel: string) => {
    if (titleStreamingRef.current) return;
    titleStreamingRef.current = true;
    let built = '';
    generateTitleStream(firstUserMessage, currentModel,
      chunk => { built += chunk; setConvTitle(built); },
      () => {
        titleStreamingRef.current = false;
        setConvTitle(built.trim() || 'New conversation');
        listConversations().then(setConversations).catch(() => {});
      },
    );
  }, []);

  const handleAsk = useCallback(() => {
    const q = question.trim();
    const m = model;
    if (!q || !m || running) return;
    setQuestion('');
    if (textareaRef.current) textareaRef.current.style.height = 'auto';
    setRunning(true);

    const isFirst = turns.length === 0;
    setTurns(prev => [...prev, { kind: 'text', role: 'user', text: q }]);

    let assistantBuf = '';
    let finalTurns: TurnItem[] = [];

    const appendAssistant = (text: string) => {
      assistantBuf += text;
      setTurns(prev => {
        const last = prev[prev.length - 1];
        const next = last?.kind === 'text' && last.role === 'assistant'
          ? [...prev.slice(0, -1), { ...last, text: assistantBuf }]
          : [...prev, { kind: 'text' as const, role: 'assistant' as const, text: assistantBuf }];
        finalTurns = next;
        return next;
      });
      scrollBottom();
    };

    abortRef.current = askStream(q, m, (ev: AskEvent) => {
      switch (ev.kind) {
        case 'text':        appendAssistant(ev.text ?? ''); break;
        case 'tool_use':    setTurns(p => { const n = [...p, { kind: 'tool_use' as const, name: ev.tool_name ?? '', args: ev.tool_args ?? '' }]; finalTurns = n; return n; }); scrollBottom(); break;
        case 'tool_result': setTurns(p => { const n = [...p, { kind: 'tool_result' as const, name: ev.tool_name ?? '', json: ev.tool_result ?? '' }]; finalTurns = n; return n; }); scrollBottom(); break;
        case 'done':        if ((ev.input_tokens ?? 0) > 0) setTokens(t => ({ in: t.in + (ev.input_tokens ?? 0), out: t.out + (ev.output_tokens ?? 0) })); break;
        case 'error':       setTurns(p => { const n = [...p, { kind: 'error' as const, text: ev.error ?? 'Unknown error' }]; finalTurns = n; return n; }); scrollBottom(); break;
      }
    }, () => {
      setRunning(false);
      saveTurns(finalTurns, m, convTitle);
      if (isFirst) generateTitle(q, m);
    }, msg => {
      setTurns(p => [...p, { kind: 'error', text: msg }]);
      setRunning(false);
    });
  }, [question, model, running, scrollBottom, turns.length, convTitle, saveTurns, generateTitle]);

  const handleStop = useCallback(() => { abortRef.current?.(); setRunning(false); }, []);

  const startNew = useCallback(() => {
    setConvOpen(false);
    setConvId(newConvId()); setConvTitle('New conversation');
    setTurns([]); setTokens({ in: 0, out: 0 });
    titleStreamingRef.current = false;
  }, []);

  const loadConversation = useCallback((_id: string, title: string, convModel: string) => {
    setConvOpen(false);
    setConvId(_id); setConvTitle(title);
    setTurns([]); setTokens({ in: 0, out: 0 });
    if (convModel) setModel(convModel);
    titleStreamingRef.current = false;
  }, []);

  const handleDeleteConv = useCallback((e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    void deleteConversation(id).catch(() => {});
    setConversations(prev => prev.filter(c => c.id !== id));
    if (id === convId) startNew();
  }, [convId, startNew]);

  const selectedModel = flatModels(groups).find(m => m.id === model);
  const canSend = question.trim().length > 0 && model !== '' && !running;
  const modelBtnLabel = !modelsReady ? 'Loading…' : groups.length === 0 ? 'No models' : (selectedModel?.display_name ?? 'Select model');
  const renderItems = pairTools(turns);

  if (config && !config.enabled) {
    return (
      <div className={styles.gate}>
        <div className={styles.gateIcon}><Icon name="chat" size={28} /></div>
        <Text variant="title" as="h3" className={styles.gateTitle}>Ask AI is off</Text>
        <Text variant="body" tone="subtle" className={styles.gateBody}>
          Enable it in <b>Settings → Intelligence</b>, then add a provider.
          Ollama runs locally with no API key — set{' '}
          <code className={styles.gateCode}>http://ollama:11434</code> as the base URL.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      {/* ── Header ── */}
      <header className={styles.chatHeader}>
        {/* Left: conversation history dropdown */}
        <Popover open={convOpen} onOpenChange={setConvOpen} side="bottom" align="start" trigger={
          <button type="button" className={styles.historyBtn} aria-label="Conversation history">
            <Icon name="list" size={14} />
            <span>Chats</span>
            <Icon name="chevron-down" size={11} className={styles.modelChevron} />
          </button>
        }>
          <div className={styles.convMenu}>
            <button type="button" className={styles.convNewBtn} onClick={startNew}>
              <Icon name="plus" size={14} />New conversation
            </button>
            {conversations.length > 0 && (
              <>
                <div className={styles.convDivider} />
                <div className={styles.convList}>
                  {conversations.map(c => (
                    <div key={c.id} className={`${styles.convItem} ${c.id === convId ? styles.convItemActive : ''}`}
                      role="button" tabIndex={0}
                      onClick={() => loadConversation(c.id, c.title, c.model)}
                      onKeyDown={e => e.key === 'Enter' && loadConversation(c.id, c.title, c.model)}>
                      <div className={styles.convItemTitle}>{c.title}</div>
                      <div className={styles.convItemMeta}>
                        <span>{relativeDate(c.updated_at)}</span>
                        <button type="button" className={styles.convDeleteBtn}
                          aria-label={`Delete ${c.title}`}
                          onClick={e => handleDeleteConv(e, c.id)}>
                          <Icon name="trash" size={12} />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </>
            )}
            {conversations.length === 0 && (
              <div className={styles.convEmpty}>No past conversations yet.</div>
            )}
          </div>
        </Popover>

        {/* Center: current conversation title */}
        <div className={styles.chatTitle} title={convTitle}>{convTitle}</div>

        {/* Right: new conversation */}
        <button type="button" className={styles.newChatBtn} onClick={startNew} aria-label="New conversation" title="New conversation">
          <Icon name="plus" size={15} />
        </button>
      </header>

      {/* ── Feed ── */}
      <div className={styles.feed}>
        {renderItems.length === 0 && (
          <div className={styles.empty}>
            <div className={styles.emptyIcon}><Icon name="chat" size={32} /></div>
            <Text variant="title" as="p" className={styles.emptyTitle}>Ask about your chat</Text>
            <div className={styles.suggestions}>
              {['Who is our top fan this month?', 'What did people say about the last game?', "Summarise today's chat"].map(s => (
                <button key={s} type="button" className={styles.suggestion} onClick={() => setQuestion(s)}>{s}</button>
              ))}
            </div>
          </div>
        )}

        {renderItems.map((t, i) => {
          if (t.kind === 'text' && t.role === 'user') {
            return (
              <div key={i} className={styles.userTurn}>
                <div className={styles.userBubble}>{t.text}</div>
              </div>
            );
          }
          if (t.kind === 'text' && t.role === 'assistant') {
            return (
              <div key={i} className={styles.assistantTurn}>
                <div className={styles.assistantAvatar} aria-hidden>
                  <Icon name="chat" size={12} />
                </div>
                <div className={styles.assistantBody}>
                  <Markdown>{t.text}</Markdown>
                </div>
              </div>
            );
          }
          if (t.kind === 'tool') {
            return (
              <div key={i} className={styles.toolTurn}>
                <ToolCard name={t.name} args={t.args} result={t.result} />
              </div>
            );
          }
          if (t.kind === 'tool_group') {
            return (
              <div key={i} className={styles.toolTurn}>
                <ToolGroupCard tools={t.tools} />
              </div>
            );
          }
          if (t.kind === 'error') {
            return (
              <div key={i} className={styles.errorTurn}>
                <Icon name="ban" size={13} />
                <span>{t.text}</span>
              </div>
            );
          }
          return null;
        })}

        {running && (
          <div className={styles.assistantTurn}>
            <div className={styles.assistantAvatar} aria-hidden>
              <Icon name="chat" size={12} />
            </div>
            <div className={styles.thinking}>
              <span /><span /><span />
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* ── Floating input ── */}
      <div className={styles.inputWrap}>
        {(tokens.in > 0 || tokens.out > 0) && (
          <div className={styles.tokenRow}>
            <Text variant="meta" tone="subtle">{fmt(tokens.in)} in · {fmt(tokens.out)} out</Text>
          </div>
        )}
        <div className={styles.inputCard}>
          <textarea
            ref={textareaRef}
            className={styles.textarea}
            placeholder="Ask anything about your chat…"
            value={question}
            disabled={running}
            rows={1}
            onChange={e => { setQuestion(e.currentTarget.value); autoResize(e.currentTarget); }}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleAsk(); } }}
          />
          <div className={styles.inputFooter}>
            <div className={styles.footerLeft}>
              {/* Model picker */}
              <Popover open={modelOpen} onOpenChange={setModelOpen} side="top" align="start" trigger={
                <button type="button" className={styles.modelBtn}>
                  <span className={styles.modelProviderIcon} data-provider={selectedModel?.providerId} aria-hidden>
                    <ProviderIcon provider={selectedModel?.providerId ?? ''} size={14} />
                  </span>
                  <span className={styles.modelName}>{modelBtnLabel}</span>
                  <Icon name="chevron-down" size={11} className={styles.modelChevron} />
                </button>
              }>
                <div className={styles.modelMenu}>
                  {groups.map(g => (
                    <div key={g.provider_id}>
                      <div className={styles.modelGroup}>
                        <span className={styles.modelGroupIcon} data-provider={g.provider_id} aria-hidden>
                          <ProviderIcon provider={g.provider_id} size={12} />
                        </span>
                        {g.display_name}
                      </div>
                      {g.models.map(m => (
                        <button key={m.id} type="button"
                          className={`${styles.modelItem} ${m.id === model ? styles.modelItemOn : ''}`}
                          onClick={() => { setModel(m.id); setModelOpen(false); }}>
                          <span className={styles.modelItemName}>{m.display_name}</span>
                          {!m.supports_tools && <span className={styles.noTools} title="May not support tool use">⚠</span>}
                          {m.price_in_per_mtok != null && m.price_in_per_mtok > 0 && (
                            <span className={styles.modelPrice}>${m.price_in_per_mtok}/${m.price_out_per_mtok}</span>
                          )}
                        </button>
                      ))}
                    </div>
                  ))}
                  {groups.length === 0 && (
                    <div className={styles.modelEmpty}>No providers configured — add one in Settings → Intelligence.</div>
                  )}
                </div>
              </Popover>
            </div>

            {running ? (
              <button type="button" className={styles.stopBtn} onClick={handleStop} aria-label="Stop">
                <span className={styles.stopSquare} aria-hidden />
              </button>
            ) : (
              <button type="button" className={styles.sendBtn} disabled={!canSend} onClick={handleAsk} aria-label="Send">
                <Icon name="arrow-up" size={16} />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function fmt(n: number) { return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n); }

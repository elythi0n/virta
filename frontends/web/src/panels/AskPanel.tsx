import { useCallback, useEffect, useRef, useState } from 'react';
import { Popover, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { askStream, getIntelConfig, listModels } from '../daemon';
import { listConversations, saveConversation, deleteConversation, generateTitleStream } from '../daemon/conversations';
import type { ConversationSummary } from '../daemon/conversations';
import type { AskEvent, IntelConfig, ModelGroup } from '../daemon/wire.gen';
import styles from './AskPanel.module.css';

type TurnRole = 'user' | 'assistant';
type TurnItem =
  | { kind: 'text'; role: TurnRole; text: string }
  | { kind: 'tool_use'; name: string; args: string }
  | { kind: 'tool_result'; name: string; json: string }
  | { kind: 'error'; text: string };

function autoResize(el: HTMLTextAreaElement) {
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 180) + 'px';
}

function flatModels(groups: ModelGroup[]) {
  return groups.flatMap(g => g.models.map(m => ({ ...m, providerName: g.display_name, providerId: g.provider_id })));
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

// ProviderIcon import kept minimal — only used for the model dot
import ProviderIcon from '../ProviderIcon';

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

  // Conversation state
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
      .then(cfg => {
        setConfig(cfg);
        if (cfg.selected_model) setModel(cfg.selected_model);
      })
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

  // Auto-save after every completed turn pair.
  const saveTurns = useCallback((nextTurns: TurnItem[], currentModel: string, title: string) => {
    void saveConversation({
      id: convId,
      title,
      model: currentModel,
      messages: nextTurns,
    }).catch(() => {});
  }, [convId]);

  // Generate a streaming title after the first exchange.
  const generateTitle = useCallback((firstUserMessage: string, currentModel: string) => {
    if (titleStreamingRef.current) return;
    titleStreamingRef.current = true;
    let built = '';
    generateTitleStream(
      firstUserMessage,
      currentModel,
      chunk => {
        built += chunk;
        setConvTitle(built);
      },
      () => {
        titleStreamingRef.current = false;
        const final = built.trim() || 'New conversation';
        setConvTitle(final);
        // Refresh the conversation list so the new title appears.
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
    const userTurn: TurnItem = { kind: 'text', role: 'user', text: q };
    setTurns(prev => [...prev, userTurn]);

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

  const loadConversation = useCallback(async (id: string, title: string, convModel: string) => {
    setConvOpen(false);
    // We store only summaries (no messages in list). Fetch messages from the server.
    // For now, start a fresh UI state and set the id/title — messages will re-populate as the user chats.
    // A real implementation would store and fetch full message history.
    setConvId(id);
    setConvTitle(title);
    setTurns([]);
    setTokens({ in: 0, out: 0 });
    if (convModel) setModel(convModel);
    titleStreamingRef.current = false;
  }, []);

  const startNew = useCallback(() => {
    setConvOpen(false);
    setConvId(newConvId());
    setConvTitle('New conversation');
    setTurns([]);
    setTokens({ in: 0, out: 0 });
    titleStreamingRef.current = false;
  }, []);

  const handleDeleteConv = useCallback(async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    await deleteConversation(id).catch(() => {});
    setConversations(prev => prev.filter(c => c.id !== id));
    if (id === convId) startNew();
  }, [convId, startNew]);

  const selectedModel = flatModels(groups).find(m => m.id === model);
  const canSend = question.trim().length > 0 && model !== '' && !running;
  const modelBtnLabel = !modelsReady ? 'Loading…' : groups.length === 0 ? 'No models' : (selectedModel?.display_name ?? 'Select model');

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
      {/* ── Feed ── */}
      <div className={styles.feed}>
        {turns.length === 0 && (
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

        {turns.map((t, i) => {
          switch (t.kind) {
            case 'text':
              return t.role === 'user' ? (
                <div key={i} className={styles.userTurn}><div className={styles.userBubble}>{t.text}</div></div>
              ) : (
                <div key={i} className={styles.assistantTurn}><div className={styles.assistantBubble}>{t.text}</div></div>
              );
            case 'tool_use':
              return (
                <details key={i} className={styles.toolBlock}>
                  <summary className={styles.toolSummary}><span className={styles.toolDot} aria-hidden /><Icon name="search" size={12} />Called <code>{t.name}</code></summary>
                  <pre className={styles.toolCode}>{t.args}</pre>
                </details>
              );
            case 'tool_result':
              return (
                <details key={i} className={styles.toolBlock}>
                  <summary className={styles.toolSummary}><span className={styles.toolDot} aria-hidden /><Icon name="check" size={12} />Result from <code>{t.name}</code></summary>
                  <pre className={styles.toolCode}>{tryPretty(t.json)}</pre>
                </details>
              );
            case 'error':
              return <div key={i} className={styles.errorTurn}><Icon name="ban" size={13} /><span>{t.text}</span></div>;
          }
        })}

        {running && <div className={styles.thinking}><span /><span /><span /></div>}
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
              {/* Conversation picker */}
              <Popover
                open={convOpen}
                onOpenChange={setConvOpen}
                side="top"
                align="start"
                trigger={
                  <button type="button" className={styles.convBtn} aria-label="Conversation">
                    <Icon name="chat" size={12} className={styles.convIcon} />
                    <span className={styles.convTitle}>{convTitle}</span>
                    <Icon name="chevron-down" size={11} className={styles.modelChevron} />
                  </button>
                }
              >
                <div className={styles.convMenu}>
                  <button type="button" className={styles.convNewBtn} onClick={startNew}>
                    <Icon name="plus" size={14} />
                    New conversation
                  </button>
                  {conversations.length > 0 && (
                    <>
                      <div className={styles.convDivider} />
                      <div className={styles.convList}>
                        {conversations.map(c => (
                          <div
                            key={c.id}
                            className={`${styles.convItem} ${c.id === convId ? styles.convItemActive : ''}`}
                            role="button"
                            tabIndex={0}
                            onClick={() => void loadConversation(c.id, c.title, c.model)}
                            onKeyDown={e => e.key === 'Enter' && void loadConversation(c.id, c.title, c.model)}
                          >
                            <div className={styles.convItemTitle}>{c.title}</div>
                            <div className={styles.convItemMeta}>
                              <span>{relativeDate(c.updated_at)}</span>
                              <button
                                type="button"
                                className={styles.convDeleteBtn}
                                aria-label={`Delete ${c.title}`}
                                onClick={e => void handleDeleteConv(e, c.id)}
                              >
                                <Icon name="trash" size={12} />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </>
                  )}
                </div>
              </Popover>

              {/* Model picker */}
              <Popover
                open={modelOpen}
                onOpenChange={setModelOpen}
                side="top"
                align="start"
                trigger={
                  <button type="button" className={styles.modelBtn} aria-label="Select model">
                    <span className={styles.modelProviderIcon} data-provider={selectedModel?.providerId} aria-hidden>
                      <ProviderIcon provider={selectedModel?.providerId ?? ''} size={14} />
                    </span>
                    <span className={styles.modelName}>{modelBtnLabel}</span>
                    <Icon name="chevron-down" size={11} className={styles.modelChevron} />
                  </button>
                }
              >
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
                        <button
                          key={m.id}
                          type="button"
                          className={`${styles.modelItem} ${m.id === model ? styles.modelItemOn : ''}`}
                          onClick={() => { setModel(m.id); setModelOpen(false); }}
                        >
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

            {/* Send / stop */}
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
function tryPretty(s: string) {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

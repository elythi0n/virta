import { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Input, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { askStream, getIntelConfig, listModels } from '../daemon';
import type { AskEvent, IntelConfig, ModelGroup } from '../daemon/wire.gen';
import styles from './AskPanel.module.css';

type TurnRole = 'user' | 'assistant';
type TurnItem =
  | { kind: 'text'; role: TurnRole; text: string }
  | { kind: 'tool_use'; name: string; args: string }
  | { kind: 'tool_result'; name: string; json: string }
  | { kind: 'error'; text: string };

// The Ask pane: a conversational interface over logged chat history using the LLM tool belt.
// Answers cite tool calls; cost footer shows tokens used. Gated by logging.
export default function AskPanel() {
  const [config, setConfig] = useState<IntelConfig | null>(null);
  const [groups, setGroups] = useState<ModelGroup[]>([]);
  const [model, setModel] = useState('');
  const [question, setQuestion] = useState('');
  const [turns, setTurns] = useState<TurnItem[]>([]);
  const [running, setRunning] = useState(false);
  const abortRef = useRef<(() => void) | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [tokens, setTokens] = useState({ in: 0, out: 0 });

  useEffect(() => {
    getIntelConfig().then(setConfig).catch(() => {});
    listModels().then(g => {
      setGroups(g);
      if (!model && g.length > 0 && g[0].models.length > 0) {
        setModel(g[0].models.find(m => m.supports_tools)?.id ?? g[0].models[0].id);
      }
    }).catch(() => {});
  }, []);

  const scrollBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  const runningRef = useRef(running);
  runningRef.current = running;

  const handleAsk = useCallback(() => {
    if (!question.trim() || runningRef.current) return;
    const q = question.trim();
    setQuestion('');
    setRunning(true);
    setTurns(prev => [...prev, { kind: 'text', role: 'user', text: q }]);

    let assistantBuf = '';
    const appendAssistant = (text: string) => {
      assistantBuf += text;
      setTurns(prev => {
        const last = prev[prev.length - 1];
        if (last?.kind === 'text' && last.role === 'assistant') {
          return [...prev.slice(0, -1), { ...last, text: assistantBuf }];
        }
        return [...prev, { kind: 'text', role: 'assistant', text: assistantBuf }];
      });
      scrollBottom();
    };

    const onEvent = (ev: AskEvent) => {
      switch (ev.kind) {
        case 'text':
          appendAssistant(ev.text ?? '');
          break;
        case 'tool_use':
          setTurns(prev => [...prev, { kind: 'tool_use', name: ev.tool_name ?? '', args: ev.tool_args ?? '' }]);
          scrollBottom();
          break;
        case 'tool_result':
          setTurns(prev => [...prev, { kind: 'tool_result', name: ev.tool_name ?? '', json: ev.tool_result ?? '' }]);
          scrollBottom();
          break;
        case 'done':
          if ((ev.input_tokens ?? 0) > 0) setTokens(t => ({ in: t.in + (ev.input_tokens ?? 0), out: t.out + (ev.output_tokens ?? 0) }));
          break;
        case 'error':
          setTurns(prev => [...prev, { kind: 'error', text: ev.error ?? 'Unknown error' }]);
          scrollBottom();
          break;
      }
    };

    abortRef.current = askStream(q, model, onEvent, () => setRunning(false), (msg) => {
      setTurns(prev => [...prev, { kind: 'error', text: msg }]);
      setRunning(false);
    });
  }, [question, model, scrollBottom]);

  if (config && !config.enabled) {
    return (
      <div className={styles.gate}>
        <Icon name="chat" size={24} />
        <Text variant="ui" as="h3" className={styles.gateTitle}>Ask pane is disabled</Text>
        <Text variant="meta" tone="subtle">
          To use Ask, you need two things: message logging enabled (so there's history to query) and
          an AI provider configured. Head to{' '}
          <b>Settings → Intelligence</b> to connect a provider — Ollama works offline with no API key.
          Nothing leaves your machine until you explicitly enable and configure a provider.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <div className={styles.feed}>
        {turns.length === 0 && (
          <div className={styles.empty}>
            <Text variant="meta" tone="subtle">
              Ask anything about your logged chat. Examples: "Who is our top fan this month?" · "What did Alice say about the game?"
            </Text>
          </div>
        )}
        {turns.map((t, i) => {
          switch (t.kind) {
            case 'text':
              return (
                <div key={i} className={`${styles.turn} ${t.role === 'user' ? styles.user : styles.assistant}`}>
                  {t.role === 'user' ? (
                    <Text variant="ui" as="p" className={styles.userText}>{t.text}</Text>
                  ) : (
                    <div className={styles.assistantText}>{t.text}</div>
                  )}
                </div>
              );
            case 'tool_use':
              return (
                <details key={i} className={styles.toolCall}>
                  <summary className={styles.toolSummary}>
                    <span className={styles.toolIcon}><Icon name="search" size={13} /></span>
                    Called <code>{t.name}</code>
                  </summary>
                  <pre className={styles.toolCode}>{t.args}</pre>
                </details>
              );
            case 'tool_result':
              return (
                <details key={i} className={styles.toolResult}>
                  <summary className={styles.toolSummary}>
                    <span className={styles.toolIcon}><Icon name="check" size={13} /></span>
                    Result from <code>{t.name}</code>
                  </summary>
                  <pre className={styles.toolCode}>{tryPretty(t.json)}</pre>
                </details>
              );
            case 'error':
              return (
                <div key={i} className={styles.errorTurn}>
                  <Icon name="ban" size={14} />
                  <Text variant="meta" tone="subtle">{t.text}</Text>
                </div>
              );
          }
        })}
        {running && (
          <div className={styles.thinking}>
            <span className={styles.dot1} /><span className={styles.dot2} /><span className={styles.dot3} />
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {(tokens.in > 0 || tokens.out > 0) && (
        <div className={styles.costBar}>
          <Text variant="meta" tone="subtle">
            {fmt(tokens.in)} in · {fmt(tokens.out)} out tokens
          </Text>
        </div>
      )}

      <div className={styles.inputBar}>
        {groups.length > 0 && (
          <select className={styles.modelSelect} value={model} onChange={e => setModel(e.target.value)}>
            {groups.map(g => (
              <optgroup key={g.provider_id} label={g.display_name}>
                {g.models.map(m => (
                  <option key={m.id} value={m.id} disabled={!m.supports_tools}>
                    {m.display_name}{m.price_in_per_mtok ? ` ·$${m.price_in_per_mtok}/$${m.price_out_per_mtok}` : ''}
                    {!m.supports_tools ? ' ⚠ no tools' : ''}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
        )}
        <Input
          aria-label="Ask a question"
          placeholder="Who is our top fan this month?"
          value={question}
          disabled={running}
          onChange={e => setQuestion(e.currentTarget.value)}
          onKeyDown={e => e.key === 'Enter' && !e.shiftKey && void handleAsk()}
        />
        <Button variant="solid" size="md" disabled={!question.trim() || running} onClick={() => void handleAsk()}>
          {running ? '…' : 'Ask'}
        </Button>
        {running && abortRef.current && (
          <Button variant="ghost" size="md" onClick={() => { abortRef.current?.(); setRunning(false); }}>Stop</Button>
        )}
      </div>
    </div>
  );
}

function fmt(n: number) { return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n); }

function tryPretty(s: string) {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

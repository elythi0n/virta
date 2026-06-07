import { useEffect, useState } from 'react';
import { Button, Input, Segmented, Text } from '@virta/ui-kit';
import { listFilters, saveFilters } from '../daemon';
import type { FilterRule } from '../daemon/wire.gen';
import styles from './FiltersPanel.module.css';

type Action = 'hide' | 'highlight' | 'mask';
const PLATFORMS = ['twitch', 'kick', 'x'] as const;

// Comma-separated matcher fields are edited as raw strings, split on save.
type EditRule = {
  id: string;
  action: Action;
  platforms: string[];
  keywords: string;
  authors: string;
  channels: string;
  regexes: string;
};

const csv = (xs?: string[]) => (xs ?? []).join(', ');
const parse = (s: string) =>
  s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean);

const uid = () => `rule-${crypto.randomUUID?.() ?? Math.random().toString(36).slice(2)}`;

function toEdit(r: FilterRule): EditRule {
  return {
    id: r.id || uid(),
    action: (r.action as Action) || 'hide',
    platforms: r.platforms ?? [],
    keywords: csv(r.keywords),
    authors: csv(r.authors),
    channels: csv(r.channels),
    regexes: csv(r.regexes),
  };
}

function toRule(e: EditRule): FilterRule {
  return {
    id: e.id,
    action: e.action,
    platforms: e.platforms.length ? e.platforms : undefined,
    keywords: parse(e.keywords).length ? parse(e.keywords) : undefined,
    authors: parse(e.authors).length ? parse(e.authors) : undefined,
    channels: parse(e.channels).length ? parse(e.channels) : undefined,
    regexes: parse(e.regexes).length ? parse(e.regexes) : undefined,
  };
}

const blank = (): EditRule => ({ id: uid(), action: 'hide', platforms: [], keywords: '', authors: '', channels: '', regexes: '' });

// Rule builder for the active profile's filter ruleset: ordered rules that hide, highlight, or
// mask matching messages. Edits stay local until saved; the daemon validates and hot-swaps.
export default function FiltersPanel() {
  const [rules, setRules] = useState<EditRule[]>([]);
  const [status, setStatus] = useState<'loading' | 'ready' | 'offline'>('loading');
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    listFilters()
      .then((rs) => {
        if (cancelled) return;
        setRules(rs.map(toEdit));
        setStatus('ready');
      })
      .catch(() => !cancelled && setStatus('offline'));
    return () => {
      cancelled = true;
    };
  }, []);

  const update = (id: string, patch: Partial<EditRule>) => setRules((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  const remove = (id: string) => setRules((rs) => rs.filter((r) => r.id !== id));
  const togglePlatform = (id: string, p: string) =>
    setRules((rs) => rs.map((r) => (r.id === id ? { ...r, platforms: r.platforms.includes(p) ? r.platforms.filter((x) => x !== p) : [...r.platforms, p] } : r)));

  const save = async () => {
    setSaving(true);
    setMessage(null);
    try {
      const stored = await saveFilters(rules.map(toRule));
      setRules(stored.map(toEdit));
      setMessage('Saved.');
    } catch {
      setMessage("Couldn't save; check your regexes and try again.");
    } finally {
      setSaving(false);
    }
  };

  if (status === 'loading') {
    return (
      <div className={styles.empty}>
        <Text variant="meta" tone="subtle">Loading filters…</Text>
      </div>
    );
  }

  if (status === 'offline') {
    return (
      <div className={styles.empty}>
        <Text variant="ui" tone="subtle">
          Not connected to a daemon. Filters apply to the active profile.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <Text variant="body" tone="subtle">
          Rules run in order on every message.
        </Text>
        <div className={styles.actions}>
          {message && (
            <Text variant="meta" tone="subtle">
              {message}
            </Text>
          )}
          <Button variant="subtle" size="sm" onClick={() => setRules((rs) => [...rs, blank()])}>
            + Add rule
          </Button>
          <Button variant="solid" size="sm" disabled={saving} onClick={() => void save()}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </div>

      <div className={styles.list}>
        {rules.length === 0 && (
          <Text variant="ui" tone="subtle" as="p" className={styles.hint}>
            No rules yet. Add one to hide, highlight, or mask messages.
          </Text>
        )}
        {rules.map((r) => (
          <div key={r.id} className={styles.rule} data-action={r.action}>
            <div className={styles.ruleHead}>
              <Segmented
                ariaLabel="Action"
                value={r.action}
                onValueChange={(v) => update(r.id, { action: v as Action })}
                options={[
                  { value: 'hide', label: 'Hide' },
                  { value: 'highlight', label: 'Highlight' },
                  { value: 'mask', label: 'Mask' },
                ]}
              />
              <button type="button" className={styles.remove} aria-label="Remove rule" onClick={() => remove(r.id)}>
                ✕
              </button>
            </div>

            <div className={styles.fields}>
              <label className={styles.field}>
                <span className={styles.label}>Keywords</span>
                <Input placeholder="whole or partial words, comma-separated" value={r.keywords} onChange={(e) => update(r.id, { keywords: e.currentTarget.value })} />
              </label>
              <label className={styles.field}>
                <span className={styles.label}>Authors</span>
                <Input placeholder="logins, comma-separated" value={r.authors} onChange={(e) => update(r.id, { authors: e.currentTarget.value })} />
              </label>
              <label className={styles.field}>
                <span className={styles.label}>Channels</span>
                <Input placeholder="slugs, comma-separated" value={r.channels} onChange={(e) => update(r.id, { channels: e.currentTarget.value })} />
              </label>
              <label className={styles.field}>
                <span className={styles.label}>Regex</span>
                <Input placeholder="patterns, comma-separated" value={r.regexes} onChange={(e) => update(r.id, { regexes: e.currentTarget.value })} />
              </label>
            </div>

            <div className={styles.platforms} role="group" aria-label="Platforms">
              {PLATFORMS.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={`${styles.chip} ${r.platforms.includes(p) ? styles.chipOn : ''}`}
                  aria-pressed={r.platforms.includes(p)}
                  onClick={() => togglePlatform(r.id, p)}
                >
                  {p}
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

import { useCallback, useEffect, useState } from 'react';
import { Badge, Button, Text } from '@virta/ui-kit';
import { deleteTheme, exportTheme, importTheme, listThemes } from '../daemon';
import type { ThemeInfo } from '../daemon/wire.gen';
import styles from './ThemeEditor.module.css';


interface Props {
  /** Currently applied theme id (from app-level theme state). */
  currentTheme: string;
  onApply: (id: string) => void;
}

// A theme row in the list — shows the name, base if custom, appearance badge, and actions.
function ThemeRow({ theme, current, onApply, onExport, onDelete }: {
  theme: ThemeInfo;
  current: boolean;
  onApply: () => void;
  onExport: () => void;
  onDelete?: () => void;
}) {
  return (
    <div className={`${styles.row} ${current ? styles.rowActive : ''}`}>
      <div className={styles.rowMeta}>
        <span className={styles.rowName}>{theme.name}</span>
        {theme.base && <span className={styles.rowBase}>based on {theme.base}</span>}
        <Badge tone={theme.appearance === 'dark' ? 'neutral' : 'accent'}>
          {theme.appearance === 'dark' ? 'dark' : 'light'}
        </Badge>
        {theme.warnings && theme.warnings.length > 0 && (
          <Badge tone="warn">{theme.warnings.length} warning{theme.warnings.length !== 1 ? 's' : ''}</Badge>
        )}
      </div>
      <div className={styles.rowActions}>
        {!current && <Button variant="ghost" size="sm" onClick={onApply}>Apply</Button>}
        {current && <span className={styles.activePill}>Active</span>}
        <Button variant="ghost" size="sm" onClick={onExport}>Export</Button>
        {onDelete && <Button variant="ghost" size="sm" onClick={onDelete}>Delete</Button>}
      </div>
    </div>
  );
}

export default function ThemeEditor({ currentTheme, onApply }: Props) {
  const [themes, setThemes] = useState<ThemeInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [pasteInput, setPasteInput] = useState('');
  const [pasteError, setPasteError] = useState('');
  const [dragging, setDragging] = useState(false);

  const reload = useCallback(() => {
    setLoading(true);
    listThemes().then(t => { setThemes(t); setLoading(false); }).catch(() => setLoading(false));
  }, []);
  useEffect(() => reload(), [reload]);

  const handleImport = async (data: object) => {
    setPasteError('');
    try {
      await importTheme(data);
      reload();
    } catch (e: unknown) {
      setPasteError(e instanceof Error ? e.message : String(e));
    }
  };

  const handlePaste = async () => {
    const s = pasteInput.trim();
    if (!s) return;
    let data: object;
    if (s.startsWith('vtheme1:')) {
      // Decode paste string: vtheme1:<base64> → JSON.
      try {
        const b64 = s.slice(8).replace(/-/g, '+').replace(/_/g, '/');
        const json = atob(b64.padEnd(b64.length + (4 - b64.length % 4) % 4, '='));
        data = JSON.parse(json);
      } catch {
        setPasteError('Invalid vtheme1: paste string');
        return;
      }
    } else {
      try {
        data = JSON.parse(s);
      } catch {
        setPasteError('Expected JSON or a vtheme1: paste string');
        return;
      }
    }
    await handleImport(data);
    setPasteInput('');
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files[0];
    if (!file) return;
    try {
      const text = await file.text();
      await handleImport(JSON.parse(text));
    } catch {
      setPasteError('Could not read the dropped file as a .vtheme JSON');
    }
  };

  const handleExport = async (id: string, name: string) => {
    try {
      const blob = await exportTheme(id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name.toLowerCase().replace(/\s+/g, '-') + '.vtheme';
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // ignore
    }
  };

  const handleDelete = async (id: string) => {
    await deleteTheme(id).catch(() => {});
    if (currentTheme === id) onApply('graphite-dark');
    reload();
  };

  const builtIns = themes.filter(t => !t.base);
  const custom = themes.filter(t => !!t.base);

  return (
    <div
      className={`${styles.editor} ${dragging ? styles.dropping : ''}`}
      onDragOver={e => { e.preventDefault(); setDragging(true); }}
      onDragLeave={() => setDragging(false)}
      onDrop={handleDrop}
    >
      {dragging && (
        <div className={styles.dropOverlay}>
          <span>Drop .vtheme file here</span>
        </div>
      )}

      {loading ? (
        <Text variant="meta" tone="subtle">Loading themes…</Text>
      ) : (
        <>
          <div className={styles.group}>
            <Text variant="meta" tone="subtle" as="h4" className={styles.groupLabel}>Built-in</Text>
            {builtIns.map(t => (
              <ThemeRow key={t.id} theme={t} current={t.id === currentTheme}
                onApply={() => onApply(t.id)}
                onExport={() => void handleExport(t.id, t.name)} />
            ))}
          </div>
          {custom.length > 0 && (
            <div className={styles.group}>
              <Text variant="meta" tone="subtle" as="h4" className={styles.groupLabel}>Custom</Text>
              {custom.map(t => (
                <ThemeRow key={t.id} theme={t} current={t.id === currentTheme}
                  onApply={() => onApply(t.id)}
                  onExport={() => void handleExport(t.id, t.name)}
                  onDelete={() => void handleDelete(t.id)} />
              ))}
            </div>
          )}
        </>
      )}

      <div className={styles.importGroup}>
        <Text variant="meta" tone="subtle" as="h4" className={styles.groupLabel}>
          Import theme
        </Text>
        <Text variant="meta" tone="subtle" as="p">
          Drag a <code>.vtheme</code> file here, or paste a <code>vtheme1:</code> share string.
        </Text>
        <div className={styles.pasteRow}>
          <input
            className={styles.pasteInput}
            aria-label="vtheme paste string or JSON"
            placeholder="vtheme1:… or paste JSON"
            value={pasteInput}
            onChange={e => setPasteInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && void handlePaste()}
          />
          <Button variant="solid" size="sm" disabled={!pasteInput.trim()} onClick={() => void handlePaste()}>
            Import
          </Button>
        </div>
        {pasteError && <Text variant="meta" tone="subtle" as="p" className={styles.error}>{pasteError}</Text>}
      </div>
    </div>
  );
}

import { Input, Segmented, Select } from '@virta/ui-kit';
import { parseSchema, type JsonSchema } from './schemaFields';
import styles from './SchemaForm.module.css';

type Props = {
  schema: JsonSchema;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
};

// Renders a config form from a plugin's JSON Schema using the same primitives as the rest of
// Settings — a plugin never writes its own form. Each change merges into the value object and is
// handed back; persistence (per-profile, under the plugin id) is the caller's job.
export default function SchemaForm({ schema, value, onChange }: Props) {
  const fields = parseSchema(schema);
  const set = (key: string, v: unknown) => onChange({ ...value, [key]: v });

  return (
    <div className={styles.form}>
      {fields.map((f) => (
        <label key={f.key} className={styles.field}>
          <span className={styles.label}>
            {f.label}
            {f.required && <span className={styles.req} aria-hidden> *</span>}
          </span>
          {f.hint && <span className={styles.hint}>{f.hint}</span>}
          {f.kind === 'toggle' ? (
            <Segmented
              ariaLabel={f.label}
              value={value[f.key] ? 'on' : 'off'}
              onValueChange={(v) => set(f.key, v === 'on')}
              options={[
                { value: 'on', label: 'On' },
                { value: 'off', label: 'Off' },
              ]}
            />
          ) : f.kind === 'select' ? (
            <Select
              ariaLabel={f.label}
              value={String(value[f.key] ?? f.options?.[0] ?? '')}
              onValueChange={(v) => set(f.key, v)}
              options={(f.options ?? []).map((o) => ({ value: o, label: o }))}
            />
          ) : f.kind === 'list' ? (
            <textarea
              className={styles.textarea}
              aria-label={f.label}
              rows={3}
              value={Array.isArray(value[f.key]) ? (value[f.key] as string[]).join('\n') : ''}
              onChange={(e) => set(f.key, e.currentTarget.value.split('\n').map((s) => s.trim()).filter(Boolean))}
            />
          ) : (
            <Input
              aria-label={f.label}
              type={f.kind === 'number' ? 'number' : 'text'}
              value={value[f.key] === undefined || value[f.key] === null ? '' : String(value[f.key])}
              onChange={(e) => set(f.key, f.kind === 'number' ? Number(e.currentTarget.value) : e.currentTarget.value)}
            />
          )}
        </label>
      ))}
    </div>
  );
}

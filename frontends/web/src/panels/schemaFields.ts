// A tiny JSON-Schema → form-field mapping, the seam for plugin configuration: a plugin
// ships a config schema and Settings renders the form from it — no plugin writes its own settings
// UI. Pure (no React) so the schema→fields logic is unit-tested. Supports the property shapes a
// config realistically needs: string, number/integer, boolean, enum (select), and string arrays.

export interface PropSchema {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'array';
  title?: string;
  description?: string;
  enum?: string[];
  default?: unknown;
  items?: { type?: string };
}

export interface JsonSchema {
  properties?: Record<string, PropSchema>;
  required?: string[];
}

export type FieldKind = 'text' | 'number' | 'toggle' | 'select' | 'list';

export interface SchemaField {
  key: string;
  label: string;
  hint?: string;
  kind: FieldKind;
  options?: string[]; // for select
  required?: boolean;
}

// parseSchema flattens a schema's properties into ordered, render-ready fields. An enum becomes a
// select regardless of its base type; arrays render as a line-per-item list; everything else maps
// by JSON type, defaulting to a text field for anything unrecognised (never a blank).
export function parseSchema(schema: JsonSchema): SchemaField[] {
  const required = new Set(schema.required ?? []);
  return Object.entries(schema.properties ?? {}).map(([key, p]) => {
    const base = { key, label: p.title ?? key, hint: p.description, required: required.has(key) };
    if (p.enum && p.enum.length > 0) return { ...base, kind: 'select' as const, options: p.enum };
    switch (p.type) {
      case 'boolean':
        return { ...base, kind: 'toggle' as const };
      case 'number':
      case 'integer':
        return { ...base, kind: 'number' as const };
      case 'array':
        return { ...base, kind: 'list' as const };
      default:
        return { ...base, kind: 'text' as const };
    }
  });
}

// defaultsFor builds the initial value object from each property's `default`, so a freshly enabled
// plugin starts from its declared defaults rather than empty.
export function defaultsFor(schema: JsonSchema): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, p] of Object.entries(schema.properties ?? {})) {
    if (p.default !== undefined) out[key] = p.default;
  }
  return out;
}

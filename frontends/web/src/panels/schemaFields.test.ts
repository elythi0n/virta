import { describe, expect, it } from 'vitest';
import { defaultsFor, parseSchema, type JsonSchema } from './schemaFields';

const SCHEMA: JsonSchema = {
  properties: {
    assetClass: { type: 'string', title: 'Asset class', enum: ['crypto', 'stocks'], default: 'crypto' },
    watchlist: { type: 'array', title: 'Watchlist', items: { type: 'string' } },
    refreshSeconds: { type: 'integer', title: 'Refresh (s)', default: 10 },
    sparkline: { type: 'boolean', description: 'Show sparklines', default: true },
    note: { type: 'string' },
  },
  required: ['assetClass'],
};

describe('parseSchema', () => {
  it('maps each property to a field of the right kind', () => {
    const fields = parseSchema(SCHEMA);
    const byKey = Object.fromEntries(fields.map((f) => [f.key, f]));
    expect(byKey.assetClass.kind).toBe('select');
    expect(byKey.assetClass.options).toEqual(['crypto', 'stocks']);
    expect(byKey.watchlist.kind).toBe('list');
    expect(byKey.refreshSeconds.kind).toBe('number');
    expect(byKey.sparkline.kind).toBe('toggle');
    expect(byKey.note.kind).toBe('text'); // untitled string → text, label falls back to key
    expect(byKey.note.label).toBe('note');
  });

  it('carries title, description, and required through', () => {
    const f = parseSchema(SCHEMA);
    const asset = f.find((x) => x.key === 'assetClass')!;
    expect(asset.label).toBe('Asset class');
    expect(asset.required).toBe(true);
    expect(f.find((x) => x.key === 'sparkline')!.hint).toBe('Show sparklines');
  });

  it('preserves property order', () => {
    expect(parseSchema(SCHEMA).map((f) => f.key)).toEqual(['assetClass', 'watchlist', 'refreshSeconds', 'sparkline', 'note']);
  });

  it('handles an empty schema', () => {
    expect(parseSchema({})).toEqual([]);
  });
});

describe('defaultsFor', () => {
  it('collects declared defaults only', () => {
    expect(defaultsFor(SCHEMA)).toEqual({ assetClass: 'crypto', refreshSeconds: 10, sparkline: true });
  });
});

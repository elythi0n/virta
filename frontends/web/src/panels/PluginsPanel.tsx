import { useState } from 'react';
import { Text } from '@virta/ui-kit';
import Icon from '../Icon';
import SchemaForm from './SchemaForm';
import { defaultsFor, type JsonSchema } from './schemaForm';
import styles from './PluginsPanel.module.css';

// The Markets plugin's config schema (docs/15 §3.4) — shown here as a live preview of how any
// plugin's settings render from its schema. It's illustrative, not an installed plugin: there's no
// plugin host yet (remote install is a later release), only the seams.
const MARKETS_SCHEMA: JsonSchema = {
  properties: {
    assetClass: {
      type: 'string',
      title: 'Asset class',
      enum: ['crypto', 'stocks', 'fx'],
      default: 'crypto',
      description: 'Crypto has free real-time feeds; stocks and FX are delayed on free tiers.',
    },
    watchlist: { type: 'array', title: 'Watchlist', items: { type: 'string' }, description: 'Symbols to track, one per line.' },
    quoteCurrency: { type: 'string', title: 'Quote currency', default: 'usd' },
    refreshSeconds: { type: 'integer', title: 'Refresh interval (seconds)', default: 10 },
    sparkline: { type: 'boolean', title: 'Show sparklines', default: true },
  },
};

// The Plugins surface. There's no plugin host yet (the seams are: a contribution registry for
// panels, the plugin.* WS namespace + DataSource, and schema-driven config) — so this explains the
// model and previews the schema-driven settings, rather than listing installed plugins.
export default function PluginsPanel() {
  const [preview, setPreview] = useState<Record<string, unknown>>(() => defaultsFor(MARKETS_SCHEMA));

  return (
    <div className={styles.panel}>
      <header className={styles.head}>
        <Icon name="plugins" size={20} />
        <div>
          <Text variant="ui" as="h2" className={styles.title}>
            Plugins
          </Text>
          <Text variant="meta" tone="subtle" as="p" className={styles.lead}>
            Plugins extend Virta with new panels, chat commands, and live data — through the same
            contribution registry the built-in panels already use.
          </Text>
        </div>
      </header>

      <section className={styles.section}>
        <Text variant="meta" tone="subtle" as="h3" className={styles.sectionTitle}>
          Installed
        </Text>
        <div className={styles.empty}>
          <Text variant="ui" tone="subtle">
            No plugins installed yet.
          </Text>
          <Text variant="meta" tone="subtle">
            Installing plugins from a URL arrives in a later release; the building blocks ship now.
          </Text>
        </div>
      </section>

      <section className={styles.section}>
        <Text variant="meta" tone="subtle" as="h3" className={styles.sectionTitle}>
          Settings preview
        </Text>
        <Text variant="meta" tone="subtle" as="p" className={styles.note}>
          A plugin declares a configuration schema and Virta renders the form — no plugin writes its
          own settings UI. Here's how the upcoming Markets plugin's config will look:
        </Text>
        <div className={styles.previewCard}>
          <SchemaForm schema={MARKETS_SCHEMA} value={preview} onChange={setPreview} />
        </div>
      </section>
    </div>
  );
}

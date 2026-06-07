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
          <Text variant="title" as="h2" className={styles.title}>
            Plugins
          </Text>
          <Text variant="body" tone="subtle" as="p" className={styles.lead}>
            Plugins extend Virta with new panels, chat commands, and live data — through the same
            contribution registry the built-in panels already use.
          </Text>
        </div>
      </header>

      <section className={styles.section}>
        <Text variant="heading" tone="subtle" as="h3" className={styles.sectionTitle}>
          Installed
        </Text>
        <div className={styles.empty}>
          <Text variant="body" tone="subtle">
            No plugins installed yet.
          </Text>
          <Text variant="ui" tone="subtle">
            Installing plugins from a URL is planned for a future release. The plugin infrastructure
            (contribution registry, event bus, schema-driven settings) ships in this version.
          </Text>
        </div>
      </section>

      <section className={styles.section}>
        <Text variant="heading" tone="subtle" as="h3" className={styles.sectionTitle}>
          Settings preview <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>— demo only</span>
        </Text>
        <Text variant="body" tone="subtle" as="p" className={styles.note}>
          A plugin declares a JSON Schema; Virta renders the settings form automatically — no plugin
          writes its own UI. The example below shows how the upcoming Markets plugin&rsquo;s config
          would render. This is a demo — the Markets plugin is not yet installed.
        </Text>
        <div className={styles.previewCard}>
          <SchemaForm schema={MARKETS_SCHEMA} value={preview} onChange={setPreview} />
        </div>
      </section>
    </div>
  );
}

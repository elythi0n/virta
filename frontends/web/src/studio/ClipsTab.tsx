import { Badge, Button, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { clock, duration } from './format';
import { downloadSpec } from './export';
import { removeClip, useStudioStore } from './store';
import type { ClipSpec } from './types';
import styles from './ClipsTab.module.css';

type Props = {
  onLoad: (clip: ClipSpec) => void;
};

// The clip library: every clip the user has defined, with its range, source, and look. Load reopens
// it in the reviewer; spec export saves a portable definition.
export default function ClipsTab({ onLoad }: Props) {
  const { clips } = useStudioStore();

  if (clips.length === 0) {
    return (
      <div className={styles.empty}>
        <Icon name="scissors" size={28} />
        <Text variant="title">No clips yet</Text>
        <Text variant="body" tone="subtle">
          Set an in/out range in the reviewer and hit <strong>Save clip</strong> — they collect here.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.grid}>
      {clips.map((c) => (
        <article key={c.id} className={styles.card}>
          <div className={styles.thumb} data-source={c.source}>
            <span className={styles.len}>{duration(c.outSec - c.inSec)}</span>
            <Badge tone={c.source === 'twitch' ? 'accent' : 'neutral'}>{c.source === 'twitch' ? 'Twitch VOD' : 'Local'}</Badge>
          </div>
          <div className={styles.body}>
            <Text variant="ui" className={styles.title}>{c.title}</Text>
            <Text variant="meta" tone="subtle">
              {clock(c.inSec)} – {clock(c.outSec)}{c.presetName ? ` · ${c.presetName}` : ''}
            </Text>
            <div className={styles.actions}>
              <Button size="sm" variant="subtle" onClick={() => onLoad(c)}>Load</Button>
              <Button size="sm" variant="ghost" onClick={() => downloadSpec(c)}>
                <Icon name="download" size={14} /> Spec
              </Button>
              <button className={styles.del} aria-label="Delete clip" onClick={() => removeClip(c.id)}>
                <Icon name="trash" size={14} />
              </button>
            </div>
          </div>
        </article>
      ))}
    </div>
  );
}

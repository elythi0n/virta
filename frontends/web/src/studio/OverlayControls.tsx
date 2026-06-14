import { memo } from 'react';
import { Button, Checkbox, Select, Slider, Switch, Text } from '@virta/ui-kit';
import { DEFAULT_OVERLAY, type NameStyle, type OverlayConfig, type OverlayFont, type OverlayTheme } from './types';
import styles from './OverlayControls.module.css';

type Props = {
  config: OverlayConfig;
  onChange: (patch: Partial<OverlayConfig>) => void;
};

const THEMES: { value: OverlayTheme; label: string }[] = [
  { value: 'glass', label: 'Glass' },
  { value: 'solid', label: 'Solid' },
  { value: 'shadow', label: 'Gradient' },
  { value: 'outline', label: 'Outline' },
  { value: 'minimal', label: 'Minimal' },
];
const FONTS: { value: OverlayFont; label: string }[] = [
  { value: 'ui', label: 'Sans' },
  { value: 'mono', label: 'Mono' },
  { value: 'rounded', label: 'Rounded' },
  { value: 'condensed', label: 'Condensed' },
];
const NAME_STYLES: { value: NameStyle; label: string }[] = [
  { value: 'colored', label: 'Platform colors' },
  { value: 'bold', label: 'Bold white' },
  { value: 'mono', label: 'Mono' },
  { value: 'hidden', label: 'Hide names' },
];

// A labelled control row: label + optional value readout on the left, control on the right.
function Row({ label, value, children }: { label: string; value?: string; children: React.ReactNode }) {
  return (
    <label className={styles.row}>
      <span className={styles.rowHead}>
        <Text variant="ui">{label}</Text>
        {value != null && <span className={styles.value}>{value}</span>}
      </span>
      <span className={styles.control}>{children}</span>
    </label>
  );
}

// The full chat-overlay configuration surface — every knob a clipper might want, grouped. Memoized
// so the reviewer's playback clock (which ticks several times a second) doesn't re-render it.
function OverlayControls({ config, onChange }: Props) {
  return (
    <div className={styles.controls}>
      <section className={styles.section}>
        <div className={styles.sectionHead}>
          <Text variant="meta" tone="subtle">PLACEMENT</Text>
          <Switch ariaLabel="Show chat overlay" checked={config.enabled} onChange={(e) => onChange({ enabled: e.currentTarget.checked })} />
        </div>
        <Text variant="meta" tone="subtle" as="p" className={styles.hint}>
          Drag the chat box on the video to move it, and pull a corner to resize.
        </Text>
        <Button variant="ghost" size="sm" onClick={() => onChange({ rect: { ...DEFAULT_OVERLAY.rect } })}>Reset position</Button>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHead}><Text variant="meta" tone="subtle">APPEARANCE</Text></div>
        <Row label="Theme">
          <Select ariaLabel="Theme" value={config.theme} onValueChange={(v) => onChange({ theme: v as OverlayTheme })} options={THEMES} />
        </Row>
        <Row label="Font">
          <Select ariaLabel="Font" value={config.font} onValueChange={(v) => onChange({ font: v as OverlayFont })} options={FONTS} />
        </Row>
        <Row label="Names">
          <Select ariaLabel="Name style" value={config.nameStyle} onValueChange={(v) => onChange({ nameStyle: v as NameStyle })} options={NAME_STYLES} />
        </Row>
        <Row label="Accent">
          <input type="color" className={styles.color} value={config.accent} onChange={(e) => onChange({ accent: e.currentTarget.value })} aria-label="Accent color" />
        </Row>
        <Row label="Text size" value={`${config.scale.toFixed(2)}×`}>
          <Slider min={0.6} max={2} step={0.05} value={config.scale} onChange={(e) => onChange({ scale: Number(e.currentTarget.value) })} />
        </Row>
        <Row label="Background" value={`${Math.round(config.opacity * 100)}%`}>
          <Slider min={0} max={1} step={0.05} value={config.opacity} onChange={(e) => onChange({ opacity: Number(e.currentTarget.value) })} />
        </Row>
        <Row label="Padding" value={`${config.padding.toFixed(1)}×`}>
          <Slider min={0.5} max={2.5} step={0.1} value={config.padding} onChange={(e) => onChange({ padding: Number(e.currentTarget.value) })} />
        </Row>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHead}><Text variant="meta" tone="subtle">CHAT</Text></div>
        <Row label="Max lines" value={String(config.maxLines)}>
          <Slider min={1} max={25} value={config.maxLines} onChange={(e) => onChange({ maxLines: Number(e.currentTarget.value) })} />
        </Row>
        <Row label="Fade-in" value={`${config.fadeMs}ms`}>
          <Slider min={0} max={600} step={20} value={config.fadeMs} onChange={(e) => onChange({ fadeMs: Number(e.currentTarget.value) })} />
        </Row>
        <div className={styles.checks}>
          <Checkbox label="Drop shadow" checked={config.shadow} onChange={(e) => onChange({ shadow: e.currentTarget.checked })} />
          <Checkbox label="Show timestamps" checked={config.showTimestamps} onChange={(e) => onChange({ showTimestamps: e.currentTarget.checked })} />
        </div>
      </section>
    </div>
  );
}

export default memo(OverlayControls);

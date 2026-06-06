import styles from './Segmented.module.css';

export type SegmentedOption = { value: string; label: string };

type SegmentedProps = {
  options: SegmentedOption[];
  value: string;
  onValueChange: (value: string) => void;
  ariaLabel?: string;
};

// A compact mutually-exclusive choice control, for small option sets where a dropdown would be
// heavier than the choice deserves (theme, density).
export default function Segmented({ options, value, onValueChange, ariaLabel }: SegmentedProps) {
  return (
    <div className={styles.segmented} role="group" aria-label={ariaLabel}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          className={`${styles.seg} ${value === o.value ? styles.on : ''}`}
          aria-pressed={value === o.value}
          onClick={() => onValueChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

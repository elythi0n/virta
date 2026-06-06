import styles from './StatusDot.module.css';

export type DotStatus = 'live' | 'idle' | 'offline';

type StatusDotProps = {
  status: DotStatus;
  /** Accessible label; defaults to the status word. */
  label?: string;
};

// A small connection/live indicator. Color carries the meaning, the label carries it for AT.
export default function StatusDot({ status, label }: StatusDotProps) {
  return (
    <span className={`${styles.dot} ${styles[status]}`} role="img" aria-label={label ?? status} />
  );
}

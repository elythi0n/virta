import type { ReactNode } from 'react';
import styles from './EmptyState.module.css';

type EmptyStateProps = {
  /** One-line headline — what isn't here, in plain language. */
  title: ReactNode;
  /** Supporting sentence — what to do about it, or why the surface is empty. */
  hint?: ReactNode;
  /** Optional glyph or illustration above the title (e.g. an Icon). */
  icon?: ReactNode;
  /** Optional action (typically a Button) below the hint. */
  action?: ReactNode;
  /** Visual variant: "subtle" (dashed inner card) or "plain" (no card chrome). */
  variant?: 'subtle' | 'plain';
  className?: string;
};

// A unified empty/no-data treatment so every "not connected yet", "nothing here yet", "sign in to
// see this" surface reads the same. The subtle variant draws a dashed card; the plain variant
// just centers the text so it works inside compact rows.
export default function EmptyState({
  title,
  hint,
  icon,
  action,
  variant = 'subtle',
  className,
}: EmptyStateProps) {
  return (
    <div
      className={[styles.empty, styles[`v-${variant}`], className].filter(Boolean).join(' ')}
      role="status"
    >
      {icon && <div className={styles.icon}>{icon}</div>}
      <div className={styles.title}>{title}</div>
      {hint && <div className={styles.hint}>{hint}</div>}
      {action && <div className={styles.action}>{action}</div>}
    </div>
  );
}

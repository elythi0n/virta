import type { ReactNode } from 'react';
import styles from './Section.module.css';

type SectionProps = {
  /** Section header label. */
  title: ReactNode;
  /** A small leading badge — either a number (rendered in a 24px square) or an icon node. */
  badge?: ReactNode;
  /** Right-aligned meta line in the header (e.g. "Pre-filled from current"). */
  meta?: ReactNode;
  /** Optional sub-title rendered under the title, before the body. */
  description?: ReactNode;
  className?: string;
  children?: ReactNode;
};

// A card that groups related fields under a consistent header — used everywhere the app shows
// "step N of a form" or "category X of settings". One geometry, one type rhythm, so any settings
// pane composed of Sections reads as the same instrument.
export default function Section({ title, badge, meta, description, className, children }: SectionProps) {
  return (
    <div className={[styles.section, className].filter(Boolean).join(' ')}>
      <div className={styles.head}>
        {badge && <span className={styles.badge}>{badge}</span>}
        <span className={styles.title}>{title}</span>
        {meta && <span className={styles.meta}>{meta}</span>}
      </div>
      {description && <div className={styles.description}>{description}</div>}
      <div className={styles.body}>{children}</div>
    </div>
  );
}

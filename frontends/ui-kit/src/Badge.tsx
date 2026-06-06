import type { ComponentPropsWithoutRef } from 'react';
import styles from './Badge.module.css';

export type BadgeTone = 'neutral' | 'accent' | 'ok' | 'warn' | 'danger';

type BadgeProps = {
  tone?: BadgeTone;
} & ComponentPropsWithoutRef<'span'>;

// Small, quiet status/label pill. Tones map to semantic color tokens.
export default function Badge({ tone = 'neutral', className, ...rest }: BadgeProps) {
  const cls = [styles.badge, styles[`tone-${tone}`], className].filter(Boolean).join(' ');
  return <span className={cls} {...rest} />;
}

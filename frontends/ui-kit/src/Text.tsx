import type { ElementType, ComponentPropsWithoutRef, ReactNode } from 'react';
import styles from './Text.module.css';

export type TextVariant = 'ui' | 'meta' | 'title' | 'mono' | 'chat-compact' | 'chat-cozy' | 'chat-comfortable';
export type TextTone = 'default' | 'muted' | 'subtle' | 'inherit';

type TextProps = {
  as?: ElementType;
  variant?: TextVariant;
  tone?: TextTone;
  className?: string;
  children?: ReactNode;
} & Omit<ComponentPropsWithoutRef<'span'>, 'className' | 'children'>;

// Typography primitive: every text style maps to a type-role token, so sizes, line heights, and
// weights stay on the one scale. `as` picks the element; visual style is independent of the tag.
export default function Text({ as, variant = 'ui', tone = 'default', className, children, ...rest }: TextProps) {
  const Tag = as ?? 'span';
  const cls = [styles.text, styles[`v-${variant}`], styles[`tone-${tone}`], className].filter(Boolean).join(' ');
  return (
    <Tag className={cls} {...rest}>
      {children}
    </Tag>
  );
}

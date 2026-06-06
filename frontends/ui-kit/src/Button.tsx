import { Slot } from '@radix-ui/react-slot';
import type { ComponentPropsWithoutRef } from 'react';
import styles from './Button.module.css';

export type ButtonVariant = 'solid' | 'ghost' | 'subtle';
export type ButtonSize = 'sm' | 'md';

type ButtonProps = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Render the single child as the button (Radix Slot), forwarding props and styling. */
  asChild?: boolean;
} & ComponentPropsWithoutRef<'button'>;

export default function Button({ variant = 'solid', size = 'md', asChild = false, className, ...rest }: ButtonProps) {
  const cls = [styles.btn, styles[`v-${variant}`], styles[`s-${size}`], className].filter(Boolean).join(' ');
  if (asChild) {
    return <Slot className={cls} {...rest} />;
  }
  return <button className={cls} type={rest.type ?? 'button'} {...rest} />;
}

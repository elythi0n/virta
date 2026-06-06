import * as RD from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import styles from './Dialog.module.css';

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
  /** Action row pinned to the bottom (right-aligned). Put the primary button last. */
  footer?: ReactNode;
  /** Widen for content-heavy dialogs (shortcut help, connections). Default is the standard width. */
  size?: 'sm' | 'md' | 'lg';
};

// Modal dialog on Radix (focus trap, escape/overlay dismiss, ARIA), token-styled with a clear
// header / body / footer structure: the title and description sit in the header, content fills a
// scrollable body, and actions live in a right-aligned footer. Title is required for accessibility.
export default function Dialog({ open, onOpenChange, title, description, children, footer, size = 'md' }: DialogProps) {
  return (
    <RD.Root open={open} onOpenChange={onOpenChange}>
      <RD.Portal>
        <RD.Overlay className={styles.overlay} />
        <RD.Content className={`${styles.content} ${styles[size]}`}>
          <header className={styles.header}>
            <RD.Title className={styles.title}>{title}</RD.Title>
            {description ? (
              <RD.Description className={styles.description}>{description}</RD.Description>
            ) : (
              <RD.Description className={styles.srOnly}>{title}</RD.Description>
            )}
          </header>
          <div className={styles.body}>{children}</div>
          {footer && <footer className={styles.footer}>{footer}</footer>}
          <RD.Close className={styles.close} aria-label="Close">
            ×
          </RD.Close>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  );
}

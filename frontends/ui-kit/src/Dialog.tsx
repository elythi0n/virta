import * as RD from '@radix-ui/react-dialog';
import type { ReactNode } from 'react';
import styles from './Dialog.module.css';

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
};

// Modal dialog on Radix (focus trap, escape/overlay dismiss, ARIA), token-styled. Title is
// required for accessibility; description is optional.
export default function Dialog({ open, onOpenChange, title, description, children }: DialogProps) {
  return (
    <RD.Root open={open} onOpenChange={onOpenChange}>
      <RD.Portal>
        <RD.Overlay className={styles.overlay} />
        <RD.Content className={styles.content}>
          <RD.Title className={styles.title}>{title}</RD.Title>
          {description ? (
            <RD.Description className={styles.description}>{description}</RD.Description>
          ) : (
            <RD.Description className={styles.srOnly}>{title}</RD.Description>
          )}
          <div className={styles.body}>{children}</div>
          <RD.Close className={styles.close} aria-label="Close">
            ×
          </RD.Close>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  );
}

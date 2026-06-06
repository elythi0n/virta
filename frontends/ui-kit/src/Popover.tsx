import * as RP from '@radix-ui/react-popover';
import type { ReactNode } from 'react';
import styles from './Popover.module.css';

type PopoverProps = {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  align?: 'start' | 'center' | 'end';
  side?: 'top' | 'right' | 'bottom' | 'left';
};

// Token-styled popover on Radix (focus, dismissal, collision handling, ARIA). The trigger is
// rendered as-is (asChild), so pass a button or any focusable element.
export default function Popover({ trigger, children, open, onOpenChange, align = 'start', side = 'bottom' }: PopoverProps) {
  return (
    <RP.Root open={open} onOpenChange={onOpenChange}>
      <RP.Trigger asChild>{trigger}</RP.Trigger>
      <RP.Portal>
        <RP.Content className={styles.content} align={align} side={side} sideOffset={6}>
          {children}
        </RP.Content>
      </RP.Portal>
    </RP.Root>
  );
}

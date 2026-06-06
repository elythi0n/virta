import * as RT from '@radix-ui/react-tooltip';
import type { ReactNode } from 'react';
import styles from './Tooltip.module.css';

type TooltipProviderProps = {
  children: ReactNode;
  delayDuration?: number;
};

// Wrap the app once. The shared provider gives the group behavior where, after the first tooltip
// shows, moving to a neighbor shows its tooltip instantly (skipDelayDuration).
export function TooltipProvider({ children, delayDuration = 320 }: TooltipProviderProps) {
  return (
    <RT.Provider delayDuration={delayDuration} skipDelayDuration={220}>
      {children}
    </RT.Provider>
  );
}

type TooltipProps = {
  content: ReactNode;
  side?: 'top' | 'right' | 'bottom' | 'left';
  sideOffset?: number;
  /** The trigger element; rendered via Radix Slot so the tooltip attaches to it directly. */
  children: ReactNode;
};

// Accessible tooltip on Radix, themed with our tokens. Hover/focus, dismissal, collision
// handling, and ARIA come from Radix; the chip and arrow are ours.
export default function Tooltip({ content, side = 'top', sideOffset = 6, children }: TooltipProps) {
  return (
    <RT.Root>
      <RT.Trigger asChild>{children}</RT.Trigger>
      <RT.Portal>
        <RT.Content className={styles.content} side={side} sideOffset={sideOffset}>
          {content}
          <RT.Arrow className={styles.arrow} />
        </RT.Content>
      </RT.Portal>
    </RT.Root>
  );
}

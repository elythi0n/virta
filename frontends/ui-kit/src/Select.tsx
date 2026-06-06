import * as RS from '@radix-ui/react-select';
import styles from './Select.module.css';

export type SelectOption = { value: string; label: string; disabled?: boolean };
export type SelectGroup = { label: string; options: SelectOption[] };

type SelectProps = {
  value?: string;
  onValueChange: (value: string) => void;
  /** Flat options, or use `groups` for labelled sections. */
  options?: SelectOption[];
  groups?: SelectGroup[];
  placeholder?: string;
  ariaLabel?: string;
  disabled?: boolean;
};

const Chevron = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="m6 9 6 6 6-6" />
  </svg>
);

const Check = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M20 6 9 17l-5-5" />
  </svg>
);

function Item({ option }: { option: SelectOption }) {
  return (
    <RS.Item className={styles.item} value={option.value} disabled={option.disabled}>
      <RS.ItemText>{option.label}</RS.ItemText>
      <RS.ItemIndicator className={styles.indicator}>
        <Check />
      </RS.ItemIndicator>
    </RS.Item>
  );
}

// Accessible select built on Radix, themed with our tokens. Renders flat options or grouped
// sections; the popover, keyboard nav, typeahead, and focus management come from Radix.
export default function Select({ value, onValueChange, options, groups, placeholder, ariaLabel, disabled }: SelectProps) {
  return (
    <RS.Root value={value} onValueChange={onValueChange} disabled={disabled}>
      <RS.Trigger className={styles.trigger} aria-label={ariaLabel}>
        <RS.Value placeholder={placeholder} />
        <RS.Icon className={styles.triggerIcon}>
          <Chevron />
        </RS.Icon>
      </RS.Trigger>
      <RS.Portal>
        <RS.Content className={styles.content} position="popper" sideOffset={4}>
          <RS.Viewport className={styles.viewport}>
            {options?.map((o) => <Item key={o.value} option={o} />)}
            {groups?.map((g, gi) => (
              <RS.Group key={g.label}>
                {gi > 0 && <RS.Separator className={styles.separator} />}
                <RS.Label className={styles.groupLabel}>{g.label}</RS.Label>
                {g.options.map((o) => <Item key={o.value} option={o} />)}
              </RS.Group>
            ))}
          </RS.Viewport>
        </RS.Content>
      </RS.Portal>
    </RS.Root>
  );
}

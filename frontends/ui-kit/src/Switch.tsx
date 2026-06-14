import { forwardRef, type InputHTMLAttributes } from 'react';
import styles from './Switch.module.css';

type SwitchProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'size'> & {
  ariaLabel?: string;
};

// A token-styled on/off toggle backed by a real checkbox input (role=switch for AT).
const Switch = forwardRef<HTMLInputElement, SwitchProps>(function Switch(
  { className, disabled, ariaLabel, ...rest },
  ref,
) {
  return (
    <span className={`${styles.root} ${className ?? ''}`} data-disabled={disabled ? '' : undefined}>
      <input
        ref={ref}
        type="checkbox"
        role="switch"
        aria-label={ariaLabel}
        className={styles.input}
        disabled={disabled}
        {...rest}
      />
      <span className={styles.track} aria-hidden="true">
        <span className={styles.thumb} />
      </span>
    </span>
  );
});

export default Switch;

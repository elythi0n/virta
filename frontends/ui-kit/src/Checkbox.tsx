import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react';
import styles from './Checkbox.module.css';

type CheckboxProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'size'> & {
  /** Optional inline label rendered to the right of the box. */
  label?: ReactNode;
  /** Optional secondary line under the label. */
  hint?: ReactNode;
};

// An accessible checkbox: a real <input type=checkbox> (keyboard, form semantics) with a
// token-styled box and an SVG tick that draws in on check.
const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
  { label, hint, className, disabled, ...rest },
  ref,
) {
  const box = (
    <span className={styles.box} aria-hidden="true">
      <svg viewBox="0 0 16 16" className={styles.tick} fill="none">
        <path d="M3.5 8.5l3 3 6-6.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </span>
  );

  if (!label && !hint) {
    return (
      <span className={`${styles.root} ${className ?? ''}`} data-disabled={disabled ? '' : undefined}>
        <input ref={ref} type="checkbox" className={styles.input} disabled={disabled} {...rest} />
        {box}
      </span>
    );
  }

  return (
    <label className={`${styles.root} ${styles.withLabel} ${className ?? ''}`} data-disabled={disabled ? '' : undefined}>
      <input ref={ref} type="checkbox" className={styles.input} disabled={disabled} {...rest} />
      {box}
      <span className={styles.labelText}>
        <span className={styles.label}>{label}</span>
        {hint && <span className={styles.hint}>{hint}</span>}
      </span>
    </label>
  );
});

export default Checkbox;

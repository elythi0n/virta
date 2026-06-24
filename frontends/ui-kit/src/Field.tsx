import { useId, type ReactNode } from 'react';
import styles from './Field.module.css';

type FieldProps = {
  /** Field label shown above the control. */
  label?: ReactNode;
  /** Right-aligned meta in the label row (e.g. character counter). */
  labelMeta?: ReactNode;
  /** Subtle line under the control explaining usage or constraints. */
  hint?: ReactNode;
  /** Validation message under the hint; overrides hint styling when present. */
  error?: ReactNode;
  /** When provided, the label binds to this id via htmlFor. Otherwise a generated id is used. */
  htmlFor?: string;
  className?: string;
  children?: ReactNode;
};

// A form row: label + control + hint/error, with one canonical spacing rhythm. Wrap any control
// (Input, Textarea, Select, custom widget) instead of repeating the label/hint markup. When
// htmlFor isn't supplied, callers can use Field.id to bind their own control to the label.
export default function Field({ label, labelMeta, hint, error, htmlFor, className, children }: FieldProps) {
  const generatedId = useId();
  const labelId = htmlFor ?? generatedId;
  return (
    <div className={[styles.field, className].filter(Boolean).join(' ')}>
      {label && (
        <div className={styles.labelRow}>
          <label className={styles.label} htmlFor={labelId}>
            {label}
          </label>
          {labelMeta && <span className={styles.labelMeta}>{labelMeta}</span>}
        </div>
      )}
      {children}
      {error ? (
        <span className={styles.error}>{error}</span>
      ) : hint ? (
        <span className={styles.hint}>{hint}</span>
      ) : null}
    </div>
  );
}

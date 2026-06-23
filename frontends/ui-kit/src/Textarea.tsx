import { forwardRef, type ComponentPropsWithoutRef } from 'react';
import styles from './Textarea.module.css';

type TextareaProps = { className?: string } & ComponentPropsWithoutRef<'textarea'>;

// Multi-line input styled to match Input: same border, focus ring, padding rhythm. Vertical
// resize only — callers set rows or min-height where they need more room — so the surrounding
// form layout never has to compensate for a user dragging the box wider.
const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { className, ...rest },
  ref,
) {
  return (
    <textarea
      ref={ref}
      className={[styles.textarea, className].filter(Boolean).join(' ')}
      {...rest}
    />
  );
});

export default Textarea;

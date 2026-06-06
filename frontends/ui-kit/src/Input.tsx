import { forwardRef, type ComponentPropsWithoutRef } from 'react';
import styles from './Input.module.css';

type InputProps = { className?: string } & ComponentPropsWithoutRef<'input'>;

// Text input styled to match Select's trigger, so form controls read as one family.
const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ className, ...rest }, ref) {
  return <input ref={ref} className={[styles.input, className].filter(Boolean).join(' ')} {...rest} />;
});

export default Input;

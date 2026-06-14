import { forwardRef, type InputHTMLAttributes } from 'react';
import styles from './Slider.module.css';

type SliderProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & {
  ariaLabel?: string;
  /** 0–1 fraction of the track to paint as "filled", for the progress look. Defaults from value/min/max. */
  fill?: number;
};

// A token-styled range input. The filled portion is painted via a CSS variable so the track shows
// progress; pass `fill` (0–1) to override, otherwise it's derived from value/min/max.
const Slider = forwardRef<HTMLInputElement, SliderProps>(function Slider(
  { className, ariaLabel, fill, value, min = 0, max = 100, style, ...rest },
  ref,
) {
  const v = Number(value ?? 0);
  const lo = Number(min);
  const hi = Number(max);
  const pct = fill != null ? fill : hi > lo ? (v - lo) / (hi - lo) : 0;
  return (
    <input
      ref={ref}
      type="range"
      aria-label={ariaLabel}
      className={`${styles.slider} ${className ?? ''}`}
      value={value}
      min={min}
      max={max}
      style={{ ['--slider-fill' as string]: `${Math.max(0, Math.min(1, pct)) * 100}%`, ...style }}
      {...rest}
    />
  );
});

export default Slider;

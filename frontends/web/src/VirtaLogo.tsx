// Virta "V" mark — a clean geometric letterform on a 24×24 grid.
// Renders just the V shape using currentColor, so it works inside any
// colored container (accent background in About, app icon, etc.).
export default function VirtaLogo({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      className={className}
    >
      {/*
        Two-arm V letterform. The path traces the outer silhouette of both arms:
        start at top-left, sweep down to the bottom tip, back up to top-right,
        then cut back inward along the inner edges to close the hollow centre.
      */}
      <path
        fillRule="evenodd"
        d="M5.5 6L9.8 6L12 13.2L14.2 6L18.5 6L12 19L5.5 6Z
           M9.1 7.4L12 15.1L14.9 7.4L16.8 7.4L12 17.2L7.2 7.4Z"
      />
    </svg>
  );
}

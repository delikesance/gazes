import { useId } from "react";

/** Hard-edged grid pattern behind a section. The parent must be `position: relative`. */
export function PageGrid({ size = 48, opacity = 0.05 }: { size?: number; opacity?: number }) {
  const id = useId();
  return (
    <svg className="page-grid" width="100%" height="100%" aria-hidden="true">
      <defs>
        <pattern id={id} width={size} height={size} patternUnits="userSpaceOnUse">
          <path d={`M${size} 0H0V${size}`} fill="none" stroke="var(--foreground)" strokeOpacity={opacity} strokeWidth="1" />
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill={`url(#${id})`} />
    </svg>
  );
}

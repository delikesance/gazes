import type { CSSProperties } from "react";

const SHAPES = {
  a: "M30 40 C 90 -20, 190 20, 140 90 S 10 120, 70 190 S 190 190, 215 140",
  b: "M15 210 C 60 110, 140 230, 185 130 S 270 50, 320 125",
} as const;

interface ScribbleProps {
  shape?: keyof typeof SHAPES;
  width?: number;
  rotate?: number;
  opacity?: number;
  strokeWidth?: number;
  style?: CSSProperties;
}

/** Thick, soft squiggle used as quiet decoration. Position it with `style` (absolute offsets). */
export function Scribble({ shape = "a", width = 300, rotate = 0, opacity = 0.05, strokeWidth = 28, style }: ScribbleProps) {
  return (
    <svg
      className="scribble"
      width={width}
      height={Math.round((width * 240) / 340)}
      viewBox="0 0 340 240"
      fill="none"
      aria-hidden="true"
      style={{ transform: `rotate(${rotate}deg)`, ...style }}
    >
      <path d={SHAPES[shape]} stroke="var(--foreground)" strokeOpacity={opacity} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

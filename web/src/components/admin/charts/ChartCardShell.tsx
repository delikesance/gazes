import React from "react";

export const MONO = "'Geist Mono', ui-monospace, monospace";
export const SANS = "'DM Sans', system-ui, sans-serif";
export const MUTED = "#a1a1aa";

interface ChartCardShellProps {
  title: string;
  subtitle?: string;
  /** Vertical gap between the card sections (px). */
  gap?: number;
  grow?: number;
  basis?: number;
  children: React.ReactNode;
}

/** Shared card frame of the chart kit: #111113 surface, 20px radius, title + subtitle header. */
export const ChartCardShell: React.FC<ChartCardShellProps> = ({ title, subtitle, gap = 16, grow = 1, basis = 320, children }) => (
  <section
    style={{
      display: "flex",
      flexDirection: "column",
      gap,
      flex: `${grow} 1 ${basis}px`,
      width: "100%",
      minWidth: 0,
      boxSizing: "border-box",
      padding: 24,
      borderRadius: 20,
      background: "#111113",
      boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
      fontFamily: SANS,
      fontSize: 14,
      lineHeight: 1.4,
      color: "#fafafa",
    }}
  >
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
      <h2 style={{ margin: 0, fontSize: 24, fontWeight: 500, letterSpacing: "-0.02em", lineHeight: "30px" }}>{title}</h2>
      {subtitle ? <span style={{ fontSize: 13, color: MUTED }}>{subtitle}</span> : null}
    </div>
    {children}
  </section>
);

export const ChartFootnote: React.FC<{ text?: string }> = ({ text }) =>
  text ? <span style={{ fontSize: 12, color: MUTED }}>{text}</span> : null;

ChartCardShell.displayName = "ChartCardShell";
ChartFootnote.displayName = "ChartFootnote";

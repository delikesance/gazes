import type { CSSProperties } from "react";

/** Surface of a page section that is not a ready-made card of the kit (same look as the design). */
export const SURFACE: CSSProperties = {
  minWidth: 0,
  boxSizing: "border-box",
  padding: 24,
  borderRadius: 20,
  background: "#111113",
  boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
  display: "flex",
  flexDirection: "column",
  gap: 16,
};

export const WRAP_ROW: CSSProperties = { display: "flex", flexWrap: "wrap", gap: 16 };

export const MONO_LABEL: CSSProperties = {
  fontFamily: "'Geist Mono', monospace",
  fontSize: 11,
  fontWeight: 500,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "#a1a1aa",
};

export const MUTED_TEXT: CSSProperties = { margin: 0, fontSize: 13, color: "#a1a1aa" };

export const LIST_RESET: CSSProperties = { listStyle: "none", margin: 0, padding: 0 };

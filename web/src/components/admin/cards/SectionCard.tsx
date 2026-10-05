import React from "react";

export interface SectionCardProps {
  title: string;
  subtitle?: string;
  subtitlePlacement?: "side" | "below";
  badge?: string;
  badgeTone?: "accent" | "neutral";
  /** Without the card surface (no padding, background or border). */
  bare?: boolean;
  /** Section content, rendered under the header. */
  children?: React.ReactNode;
}

export const SectionCard: React.FC<SectionCardProps> = ({
  title,
  subtitle,
  subtitlePlacement = "side",
  badge,
  badgeTone = "accent",
  bare = false,
  children,
}) => {
  const below = subtitlePlacement === "below";
  const subBelow = !!subtitle && below;
  const subSide = !!subtitle && !below;

  return (
    <section
      style={{
        width: "100%",
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        fontFamily: "'DM Sans', system-ui, sans-serif",
        color: "#fafafa",
        fontSize: 14,
        lineHeight: 1.4,
        ...(bare
          ? null
          : { padding: 24, borderRadius: 20, background: "#111113", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)" }),
      }}
    >
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
        <div style={{ flex: "1 1 200px", minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
          <h2 style={{ margin: 0, fontSize: 24, fontWeight: 500, letterSpacing: "-0.02em", lineHeight: "30px" }}>{title}</h2>
          {subBelow && <span style={{ fontSize: 13, color: "#a1a1aa" }}>{subtitle}</span>}
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "flex-end", gap: "8px 12px" }}>
          {subSide && <span style={{ fontSize: 13, color: "#a1a1aa" }}>{subtitle}</span>}
          {badge && (
            <span
              style={{
                fontFamily: "'Geist Mono', monospace",
                fontSize: 11,
                letterSpacing: "0.1em",
                textTransform: "uppercase",
                color: badgeTone === "neutral" ? "#a1a1aa" : "#9b8afb",
              }}
            >
              {badge}
            </span>
          )}
        </div>
      </div>
      {children}
    </section>
  );
};

SectionCard.displayName = "SectionCard";

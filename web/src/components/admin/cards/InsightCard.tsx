import React from "react";
import Link from "next/link";

export type InsightLevel = "haute" | "moyenne" | "basse";

export interface InsightCardProps {
  level?: InsightLevel;
  title: string;
  text?: string;
  action?: string;
  linkLabel?: string;
  href?: string;
  /** MCP tool name backing the insight. */
  tool?: string;
  surface?: "card" | "tile";
  basis?: number;
}

const LEVELS: Record<InsightLevel, { label: string; color: string; shape: string }> = {
  haute: { label: "Haute", color: "#f87171", shape: "M5 0L10 10H0Z" },
  moyenne: { label: "Moyenne", color: "#9b8afb", shape: "M5 0L10 5L5 10L0 5Z" },
  basse: { label: "Basse", color: "#a1a1aa", shape: "M1 1H9V9H1Z" },
};

export const InsightCard: React.FC<InsightCardProps> = ({
  level = "haute",
  title,
  text,
  action,
  linkLabel,
  href,
  tool,
  surface = "card",
  basis = 280,
}) => {
  const lv = LEVELS[String(level).toLowerCase() as InsightLevel] ?? LEVELS.moyenne;
  const tile = surface === "tile";

  return (
    <div
      style={{
        flex: `1 1 ${basis}px`,
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        alignItems: "flex-start",
        gap: 10,
        padding: tile ? 16 : 20,
        borderRadius: 20,
        background: tile ? "#17171a" : "#111113",
        boxShadow: tile ? undefined : "inset 0 0 0 1px rgba(255,255,255,0.07)",
        fontFamily: "'DM Sans', system-ui, sans-serif",
        color: "#fafafa",
        fontSize: 14,
        lineHeight: 1.4,
      }}
    >
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          fontFamily: "'Geist Mono', monospace",
          fontSize: 11,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
          padding: "4px 10px",
          borderRadius: 999,
          color: lv.color,
          background: tile ? "#151517" : "#17171a",
          boxShadow: tile ? "inset 0 0 0 1px rgba(255,255,255,0.07)" : undefined,
        }}
      >
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d={lv.shape} fill="currentColor"></path>
        </svg>
        {lv.label}
      </span>
      <span style={{ fontWeight: 500, fontSize: 15, lineHeight: 1.35 }}>{title}</span>
      {text && <span style={{ fontSize: 13, color: "#a1a1aa" }}>{text}</span>}
      {action && <span style={{ fontSize: 13, color: "#a1a1aa" }}>{action}</span>}
      {tool && (
        <span style={{ fontFamily: "'Geist Mono', monospace", fontSize: 11, color: "#a1a1aa" }}>Outil MCP : {tool}</span>
      )}
      {linkLabel && href && (
        <Link
          href={href}
          className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#fafafa]"
          style={{
            display: "inline-flex",
            alignItems: "center",
            minHeight: 44,
            fontSize: 13,
            fontWeight: 500,
            color: "#fafafa",
          }}
        >
          {linkLabel}
        </Link>
      )}
    </div>
  );
};

InsightCard.displayName = "InsightCard";

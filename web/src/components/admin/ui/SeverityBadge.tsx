"use client";

import React from "react";

export type SeverityLevel = "haute" | "moyenne" | "basse" | "info";

interface SeverityBadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  level?: SeverityLevel;
}

const LEVELS: Record<SeverityLevel, { label: string; color: string; shape: string }> = {
  haute: { label: "Haute", color: "#f87171", shape: "M5 0L10 10H0Z" },
  moyenne: { label: "Moyenne", color: "#9b8afb", shape: "M5 0L10 5L5 10L0 5Z" },
  basse: { label: "Basse", color: "#d4d4d8", shape: "M5 0a5 5 0 1 0 0 10 5 5 0 0 0 0-10z" },
  info: { label: "Info", color: "#a1a1aa", shape: "M1 1H9V9H1Z" },
};

export const SeverityBadge: React.FC<SeverityBadgeProps> = ({ level = "haute", className = "", ...props }) => {
  const config = LEVELS[level];

  const baseStyles =
    "inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full font-mono text-xs uppercase tracking-[0.08em]";

  return (
    <span className={`${baseStyles} ${className}`} style={{ color: config.color }} {...props}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" style={{ flex: "none" }}>
        <path d={config.shape} fill="currentColor"></path>
      </svg>
      {config.label}
    </span>
  );
};

SeverityBadge.displayName = "SeverityBadge";

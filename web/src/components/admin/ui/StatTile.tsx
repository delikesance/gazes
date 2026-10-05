"use client";

import React from "react";

export type StatTone = "neutral" | "accent" | "danger";
export type StatSize = "sm" | "md" | "lg";
export type StatSwatch = "none" | "accent" | "light" | "muted" | "danger";

interface StatTileProps {
  label: string;
  value: string | number;
  unit?: string;
  hint?: string;
  mono?: boolean;
  hintMono?: boolean;
  tone?: StatTone;
  size?: StatSize;
  swatch?: StatSwatch;
  basis?: number | string;
}

const TONES: Record<StatTone, string> = {
  accent: "#9b8afb",
  danger: "#f87171",
  neutral: "#fafafa",
};

const SWATCHES: Record<StatSwatch, string | null> = {
  none: null,
  accent: "#9b8afb",
  light: "#fafafa",
  muted: "#52525b",
  danger: "#f87171",
};

export const StatTile: React.FC<StatTileProps> = ({
  label,
  value,
  unit,
  hint,
  mono = false,
  hintMono = false,
  tone = "neutral",
  size = "md",
  swatch = "none",
  basis = 120,
}) => {
  const missing = value === null || value === undefined || value === "";
  const displayValue = missing ? "[À MESURER]" : String(value);

  const fontSizeMap: Record<StatSize, number> = { sm: 20, md: 22, lg: 28 };
  const fontSize = missing ? 14 : fontSizeMap[size];

  const col = TONES[tone];
  const sw = SWATCHES[swatch];

  const hintColor = tone === "accent" ? "#9b8afb" : tone === "danger" ? "#f87171" : "#fafafa";

  const shape = tone === "danger" ? "M5 0L10 10H0Z" : "M5 0L10 5L5 10L0 5Z";

  return (
    <div
      style={{
        flex: `1 1 ${typeof basis === "string" ? basis : basis + "px"}`,
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        gap: "4px",
        padding: "14px 16px",
        borderRadius: "20px",
        background: "#17171a",
      }}
    >
      <span className="inline-flex items-center gap-1.5 text-xs text-[#a1a1aa]">
        {sw && <span style={{ width: "10px", height: "10px", borderRadius: "3px", flex: "none", background: sw }}></span>}
        {label}
      </span>
      <span
        style={{
          fontSize: fontSize + "px",
          lineHeight: "1.2",
          fontVariantNumeric: "tabular-nums",
          color: missing ? "#a1a1aa" : col,
          fontFamily: missing || mono ? "'Geist Mono', monospace" : "inherit",
        }}
      >
        {displayValue}
        {unit && !missing && <span className="text-xs text-[#a1a1aa]"> {unit}</span>}
      </span>
      {hint && (
        <span
          className="inline-flex items-center gap-1.5 text-xs text-[#a1a1aa]"
          style={{ fontFamily: hintMono ? "'Geist Mono', monospace" : "inherit" }}
        >
          {tone !== "neutral" && (
            <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" style={{ flex: "none", color: hintColor }}>
              <path d={shape} fill="currentColor"></path>
            </svg>
          )}
          {hint}
        </span>
      )}
    </div>
  );
};

StatTile.displayName = "StatTile";

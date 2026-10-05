"use client";

import React from "react";

export type ProgressTone = "accent" | "light" | "muted" | "danger";

interface ProgressBarProps {
  value: number | string;
  label?: string;
  tone?: ProgressTone;
  layout?: "stacked" | "bar";
  height?: number | string;
  showValue?: boolean;
  valueText?: string;
}

const TONE_COLORS: Record<ProgressTone, string> = {
  accent: "#9b8afb",
  light: "#fafafa",
  muted: "#52525b",
  danger: "#f87171",
};

export const ProgressBar: React.FC<ProgressBarProps> = ({
  value,
  label,
  tone = "accent",
  layout = "stacked",
  height = 6,
  showValue = true,
  valueText,
}) => {
  const v = Math.max(0, Math.min(100, Number(value) || 0));
  const labelText = label ? String(label) : "";

  const displayValue = valueText ? String(valueText) : Math.round(v) + " %";
  const showHeader = layout !== "bar" && (labelText || showValue);

  const heightPx = Number(height) || 6;

  return (
    <div className="w-full min-w-0 box-border flex flex-col gap-1.5 text-sm text-[#fafafa]">
      {showHeader && (
        <span className="flex justify-between gap-3">
          <span>{labelText}</span>
          <span className="font-variant-numeric-tabular-nums text-[#a1a1aa]">{displayValue}</span>
        </span>
      )}
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(v)}
        aria-valuetext={displayValue}
        aria-label={(labelText || "Progression") + " : " + displayValue}
        className="block rounded-full bg-[#1c1c1f] overflow-hidden"
        style={{ height: heightPx + "px" }}
      >
        <div
          className="block rounded-full transition-all duration-300"
          style={{
            height: "100%",
            width: v.toFixed(1) + "%",
            background: TONE_COLORS[tone],
          }}
        ></div>
      </div>
    </div>
  );
};

ProgressBar.displayName = "ProgressBar";

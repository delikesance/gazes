"use client";

import React from "react";

interface SparklineProps {
  values: number[];
  height?: number | string;
  color?: string;
  strokeWidth?: number | string;
  area?: boolean;
  ariaLabel?: string;
}

export const Sparkline: React.FC<SparklineProps> = ({
  values = [],
  height = 28,
  color = "#9b8afb",
  strokeWidth = 1.6,
  area = false,
  ariaLabel,
}) => {
  const a = (Array.isArray(values) ? values : []).map(Number).filter((v) => Number.isFinite(v));

  let line = "";
  if (a.length === 1) {
    line = "M0 14 L100 14";
  } else if (a.length > 1) {
    const mn = Math.min(...a);
    const mx = Math.max(...a);
    const r = mx - mn || 1;
    line = a
      .map(
        (v, i) =>
          (i ? "L" : "M") +
          ((i / (a.length - 1)) * 100).toFixed(2) +
          " " +
          (26 - ((v - mn) / r) * 24).toFixed(2)
      )
      .join(" ");
  }

  const hasArea = area && a.length > 1;
  const areaPath = line + " L100 28 L0 28 Z";

  const label = ariaLabel ? String(ariaLabel) : "";

  return (
    <div className="w-full min-w-0 box-border" style={{ lineHeight: 0 }}>
      <svg
        viewBox="0 0 100 28"
        preserveAspectRatio="none"
        role={label ? "img" : "presentation"}
        aria-hidden={label ? "false" : "true"}
        aria-label={label}
        style={{
          width: "100%",
          height: Number(height) + "px",
          display: "block",
        }}
      >
        {hasArea && <path d={areaPath} fill={color} fillOpacity="0.12" stroke="none"></path>}
        <path
          d={line}
          fill="none"
          stroke={color}
          strokeWidth={Number(strokeWidth)}
          strokeLinejoin="round"
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
        ></path>
      </svg>
    </div>
  );
};

Sparkline.displayName = "Sparkline";

"use client";

import React from "react";

export interface SplitBarSegment {
  label: string;
  value: number;
  tone?: "accent" | "light" | "muted" | "danger";
}

interface SplitBarProps {
  title?: string;
  segments: SplitBarSegment[];
  total?: number;
  legend?: "inline" | "list" | "none";
  legendValue?: "percent" | "value" | "both" | "none";
  unit?: string;
  height?: number;
  marker?: number;
  markerLabel?: string;
  freeLabel?: string;
}

const TONES: Record<string, string> = {
  accent: "#9b8afb",
  light: "#fafafa",
  muted: "#52525b",
  danger: "#f87171",
};

export const SplitBar: React.FC<SplitBarProps> = ({
  title,
  segments = [],
  total: totalProp = 0,
  legend = "inline",
  legendValue = "percent",
  unit = "",
  marker,
  markerLabel = "Seuil d'alerte",
  freeLabel = "Libre",
}) => {
  const segs = segments.map((s) => ({
    label: String(s.label || ""),
    value: Math.max(0, Number(s.value) || 0),
    color: TONES[s.tone || "accent"] || TONES["accent"],
  }));

  const sum = segs.reduce((a, s) => a + s.value, 0);
  const total = Math.max(Math.max(Number(totalProp) || 0, sum), 1);
  const free = total - sum;
  const hasFree = free > total * 0.0005 && Number(totalProp) > 0;

  const unitStr = unit ? " " + unit : "";

  const pctOf = (v: number) => {
    const x = (v / total) * 100;
    return x > 0 && x < 1 ? "<1 %" : Math.round(x) + " %";
  };

  const valOf = (v: number) => {
    const val = v.toLocaleString("fr-FR", { maximumFractionDigits: 1 });
    if (legendValue === "percent") return pctOf(v);
    if (legendValue === "value") return val + unitStr;
    if (legendValue === "both") return val + unitStr + " · " + pctOf(v);
    return "";
  };

  const markerPct = marker !== undefined ? Math.max(0, Math.min(100, (marker / total) * 100)) : null;
  const mode = ["inline", "list", "none"].includes(legend) ? legend : "inline";

  const isInline = mode === "inline";
  const isList = mode === "list";

  return (
    <div className="w-full min-w-0 box-border flex flex-col gap-2 text-sm text-[#fafafa]">
      {title && <span className="font-mono text-xs font-medium tracking-[0.08em] uppercase text-[#a1a1aa]">{title}</span>}

      <div
        role="img"
        aria-label={title ? title + " : " + segs.map((s) => s.label + " " + valOf(s.value)).join(", ") : ""}
        className="relative flex h-3.5 rounded-full overflow-hidden gap-0.5 bg-[#1c1c1f]"
      >
        {segs.map((s, idx) => (
          <div
            key={idx}
            style={{
              flex: "none",
              height: "100%",
              width: ((s.value / total) * 100).toFixed(2) + "%",
              background: s.color,
              minWidth: s.value > 0 ? "3px" : "0px",
              display: s.value > 0 ? "block" : "none",
              borderRadius: "999px",
            }}
          ></div>
        ))}
        {markerPct !== null && (
          <div
            style={{
              position: "absolute",
              top: 0,
              bottom: 0,
              left: markerPct.toFixed(1) + "%",
              width: "2px",
              background: "#fafafa",
            }}
          ></div>
        )}
      </div>

      {isInline && (
        <div className="flex flex-wrap gap-2 text-xs text-[#a1a1aa] font-variant-numeric-tabular-nums">
          {segs.map((s, idx) => (
            <span key={idx} className="inline-flex items-center gap-1.5">
              <span
                style={{
                  width: "10px",
                  height: "10px",
                  borderRadius: "3px",
                  background: s.color,
                }}
              ></span>
              {s.label} {valOf(s.value)}
            </span>
          ))}
          {hasFree && (
            <span className="inline-flex items-center gap-1.5">
              <span
                style={{
                  width: "10px",
                  height: "10px",
                  borderRadius: "3px",
                  boxShadow: "inset 0 0 0 1px #71717a",
                }}
              ></span>
              {freeLabel} {valOf(free)}
            </span>
          )}
          {markerPct !== null && (
            <span className="inline-flex items-center gap-1.5">
              <span style={{ width: "2px", height: "3px", background: "#fafafa" }}></span>
              {markerLabel} {marker?.toLocaleString("fr-FR", { maximumFractionDigits: 1 })} {unit}
            </span>
          )}
        </div>
      )}

      {isList && (
        <div className="flex flex-col gap-2 mt-1">
          {segs.map((s, idx) => (
            <div key={idx} className="flex items-center justify-between gap-3 text-xs">
              <span className="inline-flex items-center gap-2">
                <span
                  style={{
                    width: "10px",
                    height: "10px",
                    borderRadius: "3px",
                    background: s.color,
                  }}
                ></span>
                {s.label}
              </span>
              <span className="font-variant-numeric-tabular-nums text-[#a1a1aa]">{valOf(s.value)}</span>
            </div>
          ))}
          {hasFree && (
            <div className="flex items-center justify-between gap-3 text-xs">
              <span className="inline-flex items-center gap-2">
                <span style={{ width: "10px", height: "10px", borderRadius: "3px", boxShadow: "inset 0 0 0 1px #71717a" }}></span>
                {freeLabel}
              </span>
              <span className="font-variant-numeric-tabular-nums text-[#a1a1aa]">{valOf(free)}</span>
            </div>
          )}
          {markerPct !== null && (
            <div className="flex items-center gap-2 text-xs text-[#a1a1aa]">
              <span style={{ width: "2px", height: "3px", background: "#fafafa" }}></span>
              {markerLabel} {marker?.toLocaleString("fr-FR", { maximumFractionDigits: 1 })} {unit}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

SplitBar.displayName = "SplitBar";

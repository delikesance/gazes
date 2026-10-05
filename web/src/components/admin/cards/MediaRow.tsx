import React from "react";
import { Thumb } from "../ui/Thumb";
import { clampPercent, formatFr, formatTrend } from "./cards.logic";

export interface MediaRowProps {
  variant?: "ranked" | "live";
  rank?: number | null;
  title: string;
  meta?: string;
  value?: string | number | null;
  /** Visually hidden text appended to the value for screen readers. */
  valueLabel?: string;
  /** 0-100 relative share (ranked variant). */
  bar?: number | null;
  /** Percent trend (ranked variant). */
  trend?: number | null;
  goodWhen?: "up" | "down";
  size?: "sm" | "lg";
  tint?: number | string;
  poster?: string | null;
}

const num = (v: number | null | undefined): number | null =>
  v === null || v === undefined || !Number.isFinite(Number(v)) ? null : Number(v);

export const MediaRow: React.FC<MediaRowProps> = ({
  variant = "ranked",
  rank,
  title,
  meta,
  value,
  valueLabel,
  bar,
  trend,
  goodWhen = "up",
  size = "sm",
  tint,
  poster,
}) => {
  const ranked = variant !== "live";
  const rankN = num(rank);
  const barN = num(bar);
  const trN = num(trend);
  const hasValue = !(value === undefined || value === null || value === "");
  const valueText = !hasValue ? "" : typeof value === "number" ? formatFr(value, null) : String(value);
  const showBar = ranked && barN !== null;
  const b = clampPercent(barN ?? 0);
  const tr = ranked && trN !== null ? formatTrend(trN) : null;
  const good = goodWhen === "down" ? !(tr?.up ?? true) : (tr?.up ?? true);

  return (
    <div
      style={{
        width: "100%",
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        alignItems: "center",
        gap: ranked ? 14 : 12,
        padding: "8px 0",
        fontFamily: "'DM Sans', system-ui, sans-serif",
        color: "#fafafa",
        fontSize: 14,
        lineHeight: 1.4,
      }}
    >
      {ranked && rankN !== null && (
        <span style={{ flex: "none", width: 20, fontFamily: "'Geist Mono', monospace", fontSize: 12, color: "#a1a1aa" }}>
          {rankN}
        </span>
      )}
      <Thumb title={title} size={size === "lg" ? "lg" : "sm"} tint={tint === "" ? undefined : tint} decorative poster={poster} />
      {showBar ? (
        <span style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 8 }}>
          <span style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: "4px 12px" }}>
            <span style={{ fontWeight: 500, fontSize: 15 }}>{title}</span>
            <span style={{ fontSize: 12, color: "#a1a1aa" }}>{meta}</span>
          </span>
          <span
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(b)}
            aria-label={"Part relative de " + title}
            style={{ display: "block", height: 6, borderRadius: 999, background: "#1c1c1f" }}
          >
            <span style={{ display: "block", height: 6, borderRadius: 999, background: "#9b8afb", width: b.toFixed(1) + "%" }} />
          </span>
        </span>
      ) : (
        <span style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
          <span style={{ fontWeight: 500, fontSize: 14, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
            {title}
          </span>
          {meta && <span style={{ fontSize: 11, color: "#a1a1aa" }}>{meta}</span>}
        </span>
      )}
      {hasValue && (
        <span
          style={{
            flex: "none",
            display: "flex",
            flexDirection: "column",
            gap: 2,
            textAlign: "right",
            ...(ranked ? { width: 76 } : null),
          }}
        >
          <span
            style={{
              position: "relative",
              fontVariantNumeric: "tabular-nums",
              ...(ranked
                ? { fontSize: 15, fontWeight: 500 }
                : { fontFamily: "'Geist Mono', monospace", fontSize: 13 }),
            }}
          >
            {valueText}
            {valueLabel && (
              <span style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" }}>
                {" "}
                {valueLabel}
              </span>
            )}
          </span>
          {tr && (
            <span
              style={{
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "flex-end",
                gap: 3,
                fontSize: 11,
                fontWeight: 600,
                color: good ? "#9b8afb" : "#f87171",
              }}
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d={tr.up ? "M12 19V5 M5 12l7-7 7 7" : "M12 5v14 M19 12l-7 7-7-7"}></path>
              </svg>
              {tr.text}
            </span>
          )}
        </span>
      )}
    </div>
  );
};

MediaRow.displayName = "MediaRow";

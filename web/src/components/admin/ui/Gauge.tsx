"use client";

import React from "react";

interface GaugeProps {
  value: number | string;
  max?: number | string;
  threshold?: number | string;
  thresholdLabel?: string;
  alertFrom?: number | string;
  warnFrom?: number | string;
  alertWhen?: "above" | "below";
  label?: string;
  unit?: string;
  size?: "md" | "lg";
  height?: number | string;
}

export const Gauge: React.FC<GaugeProps> = ({
  value,
  max = 100,
  threshold,
  thresholdLabel = "Seuil d'alerte",
  alertFrom,
  warnFrom,
  alertWhen = "above",
  label,
  unit = "",
  size = "md",
  height = 16,
}) => {
  const maxNum = Math.max(Number(max) || 100, 1);
  const v = Number(value) || 0;
  const pct = Math.max(0, Math.min(100, (v / maxNum) * 100));

  const thNum = threshold !== undefined && threshold !== null && threshold !== "" ? Number(threshold) : null;
  const hasThreshold = thNum !== null;
  const thPct = hasThreshold && thNum !== null ? Math.max(0, Math.min(100, (thNum / maxNum) * 100)) : 0;

  const below = alertWhen === "below";
  const aFrom = alertFrom !== undefined && alertFrom !== null && alertFrom !== "" ? Number(alertFrom) : null;
  const wFrom = warnFrom !== undefined && warnFrom !== null && warnFrom !== "" ? Number(warnFrom) : null;

  const hasState = hasThreshold || aFrom !== null || wFrom !== null;
  const alertAt = aFrom !== null ? aFrom : thPct;
  const alertOn = hasThreshold || aFrom !== null;
  const over = alertOn && (aFrom !== null ? below ? pct < alertAt : pct >= alertAt : below ? (thNum !== null && v < thNum) : (thNum !== null && v >= thNum));
  const near = !over && wFrom !== null && (below ? pct < wFrom && pct >= alertAt : pct >= wFrom && pct < alertAt);

  const unitStr = unit ? " " + unit : "";
  const valueText = v.toLocaleString("fr-FR", { maximumFractionDigits: 1 }) + unitStr;
  const ofText = "sur " + maxNum.toLocaleString("fr-FR", { maximumFractionDigits: 1 }) + unitStr + " · " + Math.round(pct) + " %";

  let status = "";
  if (hasState) {
    status = over
      ? below
        ? "Sous le seuil d'alerte"
        : "Seuil d'alerte dépassé"
      : near
        ? "Proche du seuil"
        : below
          ? "Au-dessus du seuil"
          : "Sous le seuil d'alerte";
  }

  const big = size === "lg";
  const color = over ? "#f87171" : "#9b8afb";
  const shape = over ? "M5 0L10 10H0Z" : "M5 0L10 5L5 10L0 5Z";

  return (
    <div className="w-full min-w-0 box-border flex flex-col gap-2.5 text-sm text-[#fafafa]">
      <div className="flex flex-wrap justify-between gap-1.5 items-baseline">
        <span className="font-mono text-xs font-medium tracking-[0.08em] uppercase text-[#a1a1aa]">{label}</span>
        {hasState && (
          <span className="inline-flex items-center gap-1.5 text-xs font-semibold" style={{ color }}>
            <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
              <path d={shape} fill="currentColor"></path>
            </svg>
            {status}
          </span>
        )}
      </div>

      <span style={{ fontSize: big ? 36 : 24 + "px", lineHeight: "1.05", letterSpacing: "-0.02em", fontVariantNumeric: "tabular-nums" }}>
        {valueText} <span style={{ fontSize: 14, letterSpacing: 0, color: "#a1a1aa" }}>{ofText}</span>
      </span>

      <div
        role="img"
        aria-label={
          (label ? label + " : " : "") +
          valueText +
          " " +
          ofText +
          (hasThreshold ? ". " + thresholdLabel + " à " + thNum + unitStr + ". " + status + "." : hasState ? ". " + status + "." : "")
        }
        className="relative rounded-full bg-[#1c1c1f]"
        style={{ height: Number(height) + "px" }}
      >
        <div
          className="rounded-full transition-all duration-300"
          style={{
            height: "100%",
            width: pct.toFixed(1) + "%",
            background: color,
          }}
        ></div>
        {hasThreshold && thNum !== null && (
          <div
            style={{
              position: "absolute",
              top: "-3px",
              bottom: "-3px",
              left: thPct.toFixed(1) + "%",
              width: "2px",
              marginLeft: "-1px",
              borderRadius: "2px",
              background: "#fafafa",
            }}
          ></div>
        )}
      </div>

      <div className="relative h-4 font-mono text-xs text-[#a1a1aa] whitespace-nowrap">
        {!(hasThreshold && thPct < 22) && <span style={{ position: "absolute", left: 0 }}>0</span>}
        {hasThreshold && thNum !== null && (
          <span style={{ position: "absolute", left: thPct.toFixed(1) + "%", transform: "translateX(-50%)", color: "#fafafa" }}>
            {thresholdLabel} {thNum?.toLocaleString("fr-FR", { maximumFractionDigits: 1 })} {unit}
          </span>
        )}
        {!(hasThreshold && thPct > 78) && <span style={{ position: "absolute", right: 0 }}>{maxNum.toLocaleString("fr-FR", { maximumFractionDigits: 1 })} {unit}</span>}
      </div>
    </div>
  );
};

Gauge.displayName = "Gauge";

"use client";

import React from "react";

interface DeltaProps {
  value: number;
  direction?: "auto" | "up" | "down";
  vs?: string;
  invert?: boolean;
  decimals?: number;
  deltaUnit?: string;
  deltaUnitPlural?: string;
  deltaDecimals?: number;
}

export const Delta: React.FC<DeltaProps> = ({
  value = 0,
  direction = "auto",
  vs = "vs période préc.",
  invert = false,
  decimals,
  deltaUnit = "%",
  deltaUnitPlural,
  deltaDecimals,
}) => {
  const v = Number(value) || 0;
  const dir = direction === "up" || direction === "down" ? direction : v >= 0 ? "up" : "down";
  const up = dir === "up";
  const good = invert ? !up : up;

  const unit = deltaUnit && deltaUnit !== "" ? deltaUnit : "%";
  const isPct = unit === "%";

  const dRaw = decimals !== undefined && decimals !== null ? decimals : deltaDecimals;
  const hasDec = dRaw !== undefined && dRaw !== null && Number.isFinite(Number(dRaw));
  const d = hasDec ? Number(dRaw) : isPct || !Number.isInteger(v) ? 1 : 0;

  const num = Math.abs(v).toLocaleString("fr-FR", { minimumFractionDigits: d, maximumFractionDigits: d });

  let uText = unit;
  if (!isPct && Number(Math.abs(v).toFixed(d)) >= 2) {
    if (deltaUnitPlural) {
      uText = deltaUnitPlural;
    } else if (typeof unit === "string" && /^[A-Za-zÀ-ÿ]{4,}$/.test(unit) && !/[sxz]$/i.test(unit)) {
      uText = unit + "s";
    }
  }

  const arrow = up ? "M12 19V5 M5 12l7-7 7 7" : "M12 5v14 M19 12l-7 7-7-7";
  const color = good ? "#9b8afb" : "#f87171";
  const text = (up ? "+" : "−") + num + " " + uText;

  return (
    <span className="inline-flex flex-wrap items-center gap-1.5 text-sm font-semibold tabular-nums" style={{ color }}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d={arrow}></path>
      </svg>
      {text}
      {vs && <span className="font-normal text-[#a1a1aa]">{vs}</span>}
    </span>
  );
};

Delta.displayName = "Delta";

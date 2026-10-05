import React from "react";
import { Delta } from "../ui/Delta";
import { Sparkline } from "../ui/Sparkline";
import { formatKpiValue } from "./cards.logic";

export interface KpiCardProps {
  label: string;
  /** Number (formatted fr-FR) or ready-made text. Empty shows "[À MESURER]". */
  value?: string | number | null;
  unit?: string;
  /** Variation vs previous period; hidden when null/undefined. */
  delta?: number | null;
  deltaDecimals?: number;
  deltaUnit?: string;
  deltaUnitPlural?: string;
  /** MCP tool name backing the figure. */
  tool?: string;
  goodWhen?: "up" | "down";
  vs?: string;
  series?: number[];
  showSpark?: boolean;
  sparkColor?: string;
  note?: string;
  tag?: string;
  /** Decimals of the main value when it is a number. */
  decimals?: number;
  basis?: number;
  /** Text shown when the value is empty (default "[À MESURER]"; "[À RENSEIGNER]" for a price to fill in). */
  missingLabel?: string;
}

export const KpiCard: React.FC<KpiCardProps> = ({
  label,
  value,
  unit,
  delta,
  deltaDecimals,
  deltaUnit = "%",
  deltaUnitPlural,
  tool,
  goodWhen = "up",
  vs = "vs 30 j préc.",
  series = [],
  showSpark = true,
  sparkColor = "#9b8afb",
  note,
  tag,
  decimals,
  basis = 200,
  missingLabel,
}) => {
  const formatted = formatKpiValue(value, decimals);
  const missing = formatted.missing;
  const text = missing && missingLabel ? missingLabel : formatted.text;
  const hasDelta = delta !== null && delta !== undefined && Number.isFinite(Number(delta));
  const d = hasDelta ? Number(delta) : 0;
  const unitStr = deltaUnit === "" ? "%" : deltaUnit;
  const decOut = deltaDecimals === undefined && unitStr !== "%" ? undefined : (deltaDecimals ?? 1);

  return (
    <div
      style={{
        flex: `1 1 ${basis}px`,
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        gap: 10,
        padding: 20,
        borderRadius: 20,
        background: "#111113",
        boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
        fontFamily: "'DM Sans', system-ui, sans-serif",
        color: "#fafafa",
        fontSize: 14,
        lineHeight: 1.4,
      }}
    >
      <span
        style={{
          fontFamily: "'Geist Mono', monospace",
          fontSize: 11,
          fontWeight: 500,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
          color: "#a1a1aa",
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontSize: missing ? 20 : 36,
          lineHeight: 1.05,
          letterSpacing: "-0.02em",
          fontVariantNumeric: "tabular-nums",
          ...(missing ? { fontFamily: "'Geist Mono', monospace", color: "#a1a1aa" } : null),
        }}
      >
        {text}
        {unit && !missing && <span style={{ fontSize: 14, letterSpacing: 0, color: "#a1a1aa" }}> {unit}</span>}
      </span>
      {tool && (
        <span style={{ fontFamily: "'Geist Mono', monospace", fontSize: 11, color: "#a1a1aa" }}>Outil MCP : {tool}</span>
      )}
      {hasDelta && (
        <Delta
          value={Math.abs(d)}
          direction={d >= 0 ? "up" : "down"}
          invert={goodWhen === "down"}
          decimals={decOut}
          deltaUnit={unitStr}
          deltaUnitPlural={deltaUnitPlural || undefined}
          vs={vs}
        />
      )}
      {note && <span style={{ fontSize: 12, color: "#a1a1aa" }}>{note}</span>}
      {tag && (
        <span
          style={{
            alignSelf: "flex-start",
            fontFamily: "'Geist Mono', monospace",
            fontSize: 11,
            color: "#a1a1aa",
            background: "#17171a",
            borderRadius: 999,
            padding: "3px 10px",
          }}
        >
          {tag}
        </span>
      )}
      {showSpark && series.length > 0 && <Sparkline values={series} height={28} color={sparkColor} />}
    </div>
  );
};

KpiCard.displayName = "KpiCard";

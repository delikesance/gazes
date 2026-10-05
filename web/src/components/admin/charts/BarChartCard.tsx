import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { computeBarChart } from "./bar-chart.logic";
import type { BarChartInput } from "./bar-chart.logic";

export interface BarChartCardProps extends BarChartInput {
  subtitle?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
}

const ACCENT = "#9b8afb";
const OTHER = "#52525b";
const monoLabel: React.CSSProperties = { fontFamily: MONO, fontSize: 11, color: MUTED };

export const BarChartCard: React.FC<BarChartCardProps> = ({ subtitle = "", footnote = "", grow = 1, basis = 320, ...input }) => {
  const m = computeBarChart(input);
  const { gap } = m;
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} gap={20} grow={grow} basis={basis}>
      <div role="img" aria-label={m.ariaLabel} style={{ display: "flex", alignItems: "flex-end", gap, height: m.height }}>
        {m.bars.map((b, i) => (
          <div
            key={i}
            title={b.tip}
            style={{ flex: 1, minWidth: 0, height: "100%", display: "flex", flexDirection: "column", alignItems: "stretch", justifyContent: "flex-end", gap: 6 }}
          >
            {b.val && (
              <span
                style={{ textAlign: "center", fontFamily: MONO, fontSize: 10, lineHeight: "14px", color: "#fafafa", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap", overflow: "visible" }}
              >
                {b.val}
              </span>
            )}
            <div style={{ height: b.height, borderRadius: `${m.radius}px ${m.radius}px 0 0`, background: b.dimmed ? OTHER : ACCENT }} />
          </div>
        ))}
      </div>
      {m.each && (
        <div style={{ display: "flex", gap }} aria-hidden="true">
          {m.bars.map((b, i) => (
            <div key={i} style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", alignItems: "center", gap: 2, textAlign: "center" }}>
              <span style={monoLabel}>{b.label}</span>
              <span style={{ fontSize: 11, color: MUTED, fontVariantNumeric: "tabular-nums" }}>{b.sub}</span>
            </div>
          ))}
        </div>
      )}
      {m.everyRow && (
        <div style={{ display: "flex", gap }} aria-hidden="true">
          {m.bars.map((b, i) => (
            <div key={i} style={{ flex: 1, minWidth: 0, display: "flex", justifyContent: "center", textAlign: "center" }}>
              <span style={{ ...monoLabel, whiteSpace: "nowrap" }}>{b.every}</span>
            </div>
          ))}
        </div>
      )}
      {m.ends && (
        <div style={{ ...monoLabel, display: "flex", justifyContent: "space-between" }}>
          <span>{m.startLabel}</span>
          <span>{m.endLabel}</span>
        </div>
      )}
      {m.legend && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px 16px", fontSize: 12, color: MUTED }}>
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
            <span aria-hidden="true" style={{ width: 10, height: 10, borderRadius: 3, background: ACCENT }} />
            {m.highlightLabel}
          </span>
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
            <span aria-hidden="true" style={{ width: 10, height: 10, borderRadius: 3, background: OTHER }} />
            {m.otherLabel}
          </span>
        </div>
      )}
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

BarChartCard.displayName = "BarChartCard";

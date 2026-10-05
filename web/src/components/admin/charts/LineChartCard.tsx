import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { computeLineChart } from "./line-chart.logic";
import type { LineChartInput } from "./line-chart.logic";

export interface LineChartCardProps extends LineChartInput {
  subtitle?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
}

const MARK_STYLE: React.CSSProperties = {
  width: 22,
  height: 22,
  borderRadius: 999,
  background: "#fafafa",
  color: "#111111",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontFamily: MONO,
  fontSize: 11,
  fontWeight: 500,
};

export const LineChartCard: React.FC<LineChartCardProps> = ({ subtitle = "", footnote = "", grow = 1, basis = 320, ...input }) => {
  const m = computeLineChart(input);
  const H = m.height;
  const tickText: React.CSSProperties = { fontFamily: MONO, fontSize: 11, color: MUTED };
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} gap={20} grow={grow} basis={basis}>
      {m.legend && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: "8px 20px", fontSize: 12, color: MUTED }}>
          {m.series.map((s, i) => (
            <span key={i} style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
              <span style={{ display: "inline-block", width: 22, height: 0, borderTop: `3px ${s.dashed ? "dashed" : "solid"} ${s.stroke}` }} aria-hidden="true" />
              {s.label}
            </span>
          ))}
        </div>
      )}
      <div style={{ display: "flex", gap: 12 }}>
        <div
          aria-hidden="true"
          style={{ ...tickText, display: "flex", flexDirection: "column", justifyContent: "space-between", height: H, textAlign: "right", fontVariantNumeric: "tabular-nums" }}
        >
          {m.yTicks.map((t, i) => (
            <span key={i}>{t}</span>
          ))}
        </div>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 10 }}>
          <div style={{ position: "relative", height: H }}>
            {m.bands.map((b, i) => (
              <span
                key={i}
                aria-hidden="true"
                style={{
                  position: "absolute",
                  top: 0,
                  bottom: 0,
                  left: `${b.leftPct.toFixed(2)}%`,
                  width: `${b.widthPct.toFixed(2)}%`,
                  background: b.danger ? "#f87171" : "#fafafa",
                  opacity: 0.12,
                  pointerEvents: "none",
                }}
              />
            ))}
            <svg viewBox={m.viewBox} preserveAspectRatio="none" role="img" aria-label={m.ariaLabel} style={{ width: "100%", height: H, display: "block", overflow: "visible" }}>
              <path d={m.gridPath} stroke="#fafafa" strokeOpacity="0.07" strokeWidth="1" fill="none" vectorEffect="non-scaling-stroke" />
              {m.areaPath && <path d={m.areaPath} fill={m.areaFill} fillOpacity="0.12" />}
              {[...m.series].reverse().map((s, k) => (
                <path
                  key={m.series.length - 1 - k}
                  d={s.path}
                  fill="none"
                  stroke={s.stroke}
                  strokeWidth={s.strokeWidth}
                  strokeDasharray={s.dash}
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
              ))}
            </svg>
            {m.marks.map((mk) => (
              <span
                key={mk.n}
                aria-hidden="true"
                style={{ ...MARK_STYLE, position: "absolute", left: `${mk.leftPct.toFixed(2)}%`, top: `${mk.topPct.toFixed(2)}%`, transform: "translate(-50%, -50%)" }}
              >
                {mk.n}
              </span>
            ))}
          </div>
          {m.xTicks.length > 0 && (
            <div aria-hidden="true" style={{ ...tickText, display: "flex", justifyContent: "space-between" }}>
              {m.xTicks.map((t, i) => (
                <span key={i}>{t}</span>
              ))}
            </div>
          )}
        </div>
      </div>
      {m.marks.length > 0 && (
        <ol style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
          {m.marks.map((mk) => (
            <li
              key={mk.n}
              style={{ display: "flex", alignItems: mk.range || mk.detail || mk.cause ? "flex-start" : "center", gap: 10, fontSize: 13 }}
            >
              <span aria-hidden="true" style={{ ...MARK_STYLE, flex: "none" }}>
                {mk.n}
              </span>
              <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                <span>{mk.label}</span>
                {mk.range && <span style={{ fontSize: 12, color: MUTED }}>{mk.range}</span>}
                {mk.detail && <span style={{ fontSize: 12, color: MUTED }}>{mk.detail}</span>}
                {mk.cause && <span style={{ fontSize: 12, color: MUTED }}>{mk.cause}</span>}
              </span>
            </li>
          ))}
        </ol>
      )}
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

LineChartCard.displayName = "LineChartCard";

import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { QUADRANT_TEXT_COLORS, computeQuadrant } from "./quadrant.logic";
import type { QuadrantInput, QuadrantKey } from "./quadrant.logic";

export interface QuadrantCardProps extends QuadrantInput {
  subtitle?: string;
  xLabel?: string;
  yLabel?: string;
  showSummary?: boolean;
  height?: number;
  footnote?: string;
  grow?: number;
  basis?: number;
}

const cornerStyle: React.CSSProperties = { position: "absolute", fontFamily: MONO, fontSize: 11, letterSpacing: "0.08em", textTransform: "uppercase" };
const axisText: React.CSSProperties = { fontFamily: MONO, fontSize: 11, color: MUTED };

export const QuadrantCard: React.FC<QuadrantCardProps> = ({
  subtitle = "",
  xLabel = "axe X",
  yLabel = "axe Y",
  showSummary = true,
  height = 440,
  footnote = "",
  grow = 1,
  basis = 520,
  ...input
}) => {
  const m = computeQuadrant(input);
  const H = Math.max(200, Number.isFinite(height) ? height : 440);
  const corners: Array<[QuadrantKey, React.CSSProperties]> = [
    ["topLeft", { top: 12, left: 14 }],
    ["topRight", { top: 12, right: 14 }],
    ["bottomLeft", { bottom: 12, left: 14 }],
    ["bottomRight", { bottom: 12, right: 14 }],
  ];
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} grow={grow} basis={basis}>
      <div style={{ display: "flex", gap: 12 }}>
        <div aria-hidden="true" style={{ ...axisText, display: "flex", flexDirection: "column", justifyContent: "space-between", height: H, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
          {m.yTicks.map((t, i) => (
            <span key={i}>{t}</span>
          ))}
        </div>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 10 }}>
          <div role="img" aria-label={m.ariaLabel} style={{ position: "relative", height: H, borderRadius: 20, background: "#17171a", overflow: "hidden" }}>
            <span aria-hidden="true" style={{ position: "absolute", top: 0, bottom: 0, left: `${m.thrX.toFixed(2)}%`, width: 0, borderLeft: "1px dashed rgba(250,250,250,0.25)" }} />
            <span aria-hidden="true" style={{ position: "absolute", left: 0, right: 0, bottom: `${m.thrY.toFixed(2)}%`, height: 0, borderTop: "1px dashed rgba(250,250,250,0.25)" }} />
            {corners.map(([k, pos]) => (
              <span key={k} aria-hidden="true" style={{ ...cornerStyle, ...pos, color: QUADRANT_TEXT_COLORS[k] }}>
                {m.names[k]}
              </span>
            ))}
            {m.points.map((d, i) => {
              const r = d.diameter / 2;
              return (
                <span
                  key={i}
                  title={d.tip}
                  // Clamped so a point on an edge (0 % or 100 %) stays whole instead of being cut by the frame.
                  style={{
                    position: "absolute",
                    left: `clamp(${r}px, ${d.leftPct.toFixed(2)}%, calc(100% - ${r}px))`,
                    bottom: `clamp(${r}px, ${d.bottomPct.toFixed(2)}%, calc(100% - ${r}px))`,
                    width: 0,
                    height: 0,
                  }}
                >
                  <span style={{ position: "absolute", left: -r, top: -r, width: d.diameter, height: d.diameter, borderRadius: 999, background: d.color }} />
                  <span
                    style={{ position: "absolute", ...(d.flip ? { right: r + 4 } : { left: r + 4 }), top: -8, whiteSpace: "nowrap", fontSize: 11, color: "#fafafa" }}
                  >
                    {d.name}
                  </span>
                </span>
              );
            })}
          </div>
          <div aria-hidden="true" style={{ ...axisText, display: "flex", justifyContent: "space-between" }}>
            {m.xTicks.map((t, i) => (
              <span key={i}>{t}</span>
            ))}
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", gap: "4px 16px", fontSize: 12, color: MUTED }}>
            <span>Axe horizontal : {xLabel}</span>
            <span>Axe vertical : {yLabel}</span>
          </div>
        </div>
      </div>
      {showSummary && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
          {m.quadrants.map((q) => (
            <div key={q.key} style={{ flex: "1 1 220px", minWidth: 0, display: "flex", flexDirection: "column", gap: 6, padding: "14px 16px", borderRadius: 20, background: "#17171a" }}>
              <span style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                <span style={{ fontWeight: 500, fontSize: 14 }}>{q.name}</span>
                <span style={{ fontFamily: MONO, fontSize: 12, color: q.color }}>{q.n}</span>
              </span>
              {q.rule && <span style={{ fontSize: 12, color: MUTED }}>{q.rule}</span>}
              <span style={{ fontSize: 13 }}>{q.titles}</span>
            </div>
          ))}
        </div>
      )}
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

QuadrantCard.displayName = "QuadrantCard";

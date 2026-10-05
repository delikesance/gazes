import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { computeHeatmap } from "./heatmap.logic";
import type { HeatmapInput } from "./heatmap.logic";

export interface HeatmapCardProps extends HeatmapInput {
  subtitle?: string;
  metaLabel?: string;
  rowHeader?: string;
  legendLow?: string;
  legendHigh?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
}

const head: React.CSSProperties = { padding: 0, fontWeight: 400, fontFamily: MONO, fontSize: 11, color: MUTED };
const SR_ONLY: React.CSSProperties = { position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" };

export const HeatmapCard: React.FC<HeatmapCardProps> = ({
  subtitle = "",
  metaLabel = "",
  rowHeader = "",
  legendLow = "Moins",
  legendHigh = "Plus",
  footnote = "",
  grow = 1,
  basis = 420,
  ...input
}) => {
  const m = computeHeatmap(input);
  const fill = (opacity: number): React.CSSProperties => ({ position: "absolute", inset: 0, background: m.color, opacity });
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} grow={grow} basis={basis}>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", minWidth: m.minWidth, tableLayout: "fixed", borderCollapse: "separate", borderSpacing: m.gap }}>
          <caption style={SR_ONLY}>{m.ariaLabel}</caption>
          <thead>
            <tr>
              <th scope="col" style={{ ...head, width: m.rowLabelWidth, textAlign: "left" }}>
                {rowHeader}
              </th>
              {m.hasMeta && (
                <th scope="col" style={{ ...head, width: 76, padding: "0 10px", textAlign: "right", letterSpacing: "0.08em", textTransform: "uppercase" }}>
                  {metaLabel}
                </th>
              )}
              {m.colHeads.map((c, i) => (
                <th key={i} scope="col" style={{ ...head, position: "relative", height: 18, textAlign: "center" }}>
                  <span style={c.floating ? { position: "absolute", top: 0, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap" } : { whiteSpace: "nowrap" }}>{c.label}</span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {m.rows.map((r, ri) => (
              <tr key={ri}>
                <th scope="row" style={{ ...head, textAlign: "left", whiteSpace: "nowrap" }}>
                  {r.label}
                </th>
                {m.hasMeta && (
                  <td style={{ padding: "0 10px", textAlign: "right", fontSize: 12, fontVariantNumeric: "tabular-nums", color: MUTED, whiteSpace: "nowrap" }}>{r.meta}</td>
                )}
                {r.cells.map((c, ci) => (
                  <td key={ci} style={{ padding: 0 }} title={c.tip}>
                    <div
                      style={{
                        position: "relative",
                        height: m.cellHeight,
                        borderRadius: m.radius,
                        overflow: "hidden",
                        background: "#17171a",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                      }}
                    >
                      {!c.missing && <span style={fill(c.opacity)} />}
                      {c.missing && !c.text && <span style={SR_ONLY}>non mesurable</span>}
                      <span
                        style={{
                          position: "relative",
                          fontFamily: MONO,
                          fontSize: 12,
                          fontVariantNumeric: "tabular-nums",
                          color: c.missing ? MUTED : c.darkText ? "#111111" : "#fafafa",
                        }}
                      >
                        {c.text}
                      </span>
                    </div>
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px 16px", fontSize: 12, color: MUTED }}>
        <span>{legendLow}</span>
        <span style={{ display: "inline-flex", gap: 3 }} aria-hidden="true">
          {m.legend.map((o, i) => (
            <span key={i} style={{ display: "block", width: 28, height: 14, borderRadius: 4, background: "#17171a", position: "relative", overflow: "hidden" }}>
              <span style={fill(o)} />
            </span>
          ))}
        </span>
        <span>{legendHigh}</span>
        {m.missingNote && <span>{m.missingNote}</span>}
      </div>
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

HeatmapCard.displayName = "HeatmapCard";

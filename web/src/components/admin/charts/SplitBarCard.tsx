import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { computeSplitBarCard } from "./split-bar.logic";
import type { SplitBarCardInput } from "./split-bar.logic";

export interface SplitBarCardProps extends SplitBarCardInput {
  subtitle?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
}

export const SplitBarCard: React.FC<SplitBarCardProps> = ({ title = "Répartition", subtitle = "", footnote = "", grow = 1, basis = 300, ...input }) => {
  const m = computeSplitBarCard({ ...input, title });
  const bh = m.barHeight;
  return (
    <ChartCardShell title={title} subtitle={subtitle} grow={grow} basis={basis}>
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        {m.groups.map((g, gi) => (
          <div key={gi} style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {g.label && (
              <span style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
                <span style={{ fontFamily: MONO, fontSize: 11, fontWeight: 500, letterSpacing: "0.08em", textTransform: "uppercase", color: MUTED }}>{g.label}</span>
                <span style={{ fontSize: 12, color: MUTED, fontVariantNumeric: "tabular-nums" }}>{g.total}</span>
              </span>
            )}
            <div role="img" aria-label={g.ariaLabel} style={{ display: "flex", height: bh, borderRadius: 999, overflow: "hidden", gap: 2 }}>
              {g.segs.map((s, si) => (
                <div key={si} title={s.tip} style={{ flex: "0 0 auto", height: bh, width: `${s.widthPct.toFixed(2)}%`, minWidth: s.hasValue ? 3 : 0, background: s.color }} />
              ))}
            </div>
            <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexWrap: "wrap", gap: "6px 16px", fontSize: 12, color: MUTED }}>
              {g.segs.map((s, si) => (
                <li key={si} style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <span aria-hidden="true" style={{ display: "inline-block", width: 10, height: 10, borderRadius: 3, background: s.color }} />
                  {s.text}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

SplitBarCard.displayName = "SplitBarCard";

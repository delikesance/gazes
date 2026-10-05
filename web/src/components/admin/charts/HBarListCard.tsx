import React from "react";
import { ChartCardShell, ChartFootnote, MONO, MUTED } from "./ChartCardShell";
import { computeHBarList } from "./hbar-list.logic";
import type { HBarListInput } from "./hbar-list.logic";

export interface HBarListCardProps extends HBarListInput {
  title?: string;
  subtitle?: string;
  emptyText?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
}

const ARROW = { up: "M12 19V5 M5 12l7-7 7 7", down: "M12 5v14 M19 12l-7 7-7-7" } as const;

export const HBarListCard: React.FC<HBarListCardProps> = ({
  title = "Classement",
  subtitle = "",
  emptyText = "Aucune donnée sur la période.",
  footnote = "",
  grow = 1,
  basis = 300,
  ...input
}) => {
  const m = computeHBarList(input);
  const bh = m.barHeight;
  return (
    <ChartCardShell title={title} subtitle={subtitle} grow={grow} basis={basis}>
      {m.rows.length > 0 ? (
        <ol style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: m.rowGap }}>
          {m.rows.map((r, i) => (
            <li key={i} style={{ display: "flex", alignItems: "center", gap: 14 }}>
              {m.showRank && <span style={{ flex: "none", width: 20, fontFamily: MONO, fontSize: 12, color: MUTED }}>{r.rank}</span>}
              <span style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 6 }}>
                <span style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: "2px 12px" }}>
                  <span style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", gap: "2px 10px", minWidth: 0 }}>
                    <span style={{ fontSize: 13, fontWeight: 500 }}>{r.label}</span>
                    {r.hint && <span style={{ fontSize: 12, color: MUTED }}>{r.hint}</span>}
                  </span>
                  <span style={{ display: "inline-flex", alignItems: "baseline", gap: 10, fontVariantNumeric: "tabular-nums" }}>
                    {m.showValue && <span style={{ fontSize: 13, color: MUTED }}>{r.value}</span>}
                    {r.delta && (
                      <span
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 4,
                          fontSize: 12,
                          fontWeight: 600,
                          color: r.deltaDir === "down" ? "#f87171" : r.deltaDir === "up" ? "#9b8afb" : MUTED,
                        }}
                      >
                        {r.deltaDir && (
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ verticalAlign: -1 }}>
                            <path d={ARROW[r.deltaDir]} />
                          </svg>
                        )}
                        {r.delta}
                      </span>
                    )}
                  </span>
                </span>
                <span aria-hidden="true" style={{ display: "block", height: bh, borderRadius: 999, background: "#1c1c1f" }}>
                  <span style={{ display: "block", height: bh, borderRadius: 999, minWidth: r.rawValue > 0 ? bh : 0, background: r.color, width: `${r.widthPct.toFixed(1)}%` }} />
                </span>
              </span>
            </li>
          ))}
        </ol>
      ) : (
        <p role="status" style={{ margin: 0, fontSize: 13, color: MUTED }}>
          {emptyText}
        </p>
      )}
      <ChartFootnote text={footnote} />
    </ChartCardShell>
  );
};

HBarListCard.displayName = "HBarListCard";

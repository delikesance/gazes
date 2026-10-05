import React from "react";
import { ChartCardShell, MONO, MUTED } from "./ChartCardShell";
import { computeFunnel } from "./funnel.logic";
import type { FunnelInput } from "./funnel.logic";

export interface FunnelCardProps extends FunnelInput {
  subtitle?: string;
  note?: string;
  grow?: number;
  basis?: number;
}

export const FunnelCard: React.FC<FunnelCardProps> = ({ subtitle = "", note = "", grow = 1, basis = 520, ...input }) => {
  const m = computeFunnel(input);
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} gap={20} grow={grow} basis={basis}>
      <ol aria-label={m.ariaLabel} style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 10 }}>
        {m.rows.map((f, i) => (
          <li
            key={i}
            style={{
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              gap: "10px 20px",
              padding: "14px 16px",
              borderRadius: 20,
              background: "#17171a",
              boxShadow: f.isWorst ? "inset 0 0 0 2px #f87171" : undefined,
            }}
          >
            <div style={{ flex: "1 1 190px", minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
              <span style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
                <span style={{ fontWeight: 500, fontSize: 15 }}>{f.label}</span>
                {f.isWorst && (
                  <span
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      fontFamily: MONO,
                      fontSize: 11,
                      letterSpacing: "0.08em",
                      textTransform: "uppercase",
                      padding: "3px 10px",
                      borderRadius: 999,
                      background: "#7f1d1d",
                      color: "#fecaca",
                    }}
                  >
                    Plus forte perte
                  </span>
                )}
              </span>
              {f.source && <span style={{ fontFamily: MONO, fontSize: 11, color: MUTED }}>{f.source}</span>}
            </div>
            <div aria-hidden="true" style={{ flex: "3 1 240px", minWidth: 0, display: "flex", alignItems: "center" }}>
              <span style={{ display: "block", width: "100%", height: 28, borderRadius: 8, background: "#1c1c1f" }}>
                <span
                  style={{
                    display: "block",
                    height: 28,
                    borderRadius: 8,
                    background: f.isWorst ? "#f87171" : "#9b8afb",
                    minWidth: f.hasValue ? 4 : 0,
                    width: `${f.widthPct.toFixed(1)}%`,
                  }}
                />
              </span>
            </div>
            <div style={{ flex: "1 1 190px", minWidth: 0, display: "flex", flexDirection: "column", gap: 4, alignItems: "flex-end", textAlign: "right" }}>
              <span style={{ fontSize: 20, fontVariantNumeric: "tabular-nums" }}>
                {f.count} <span style={{ fontSize: 12, color: MUTED }}>({f.pctStart})</span>
              </span>
              {f.lossText !== null && (
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, fontWeight: 600, color: f.isWorst ? "#f87171" : MUTED }}>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M12 5v14 M19 12l-7 7-7-7" />
                  </svg>
                  {f.lossText}
                </span>
              )}
              <span style={{ fontSize: 12, color: MUTED }}>{f.convText}</span>
            </div>
          </li>
        ))}
      </ol>
      {note && <p style={{ margin: 0, fontSize: 12, color: MUTED }}>{note}</p>}
    </ChartCardShell>
  );
};

FunnelCard.displayName = "FunnelCard";

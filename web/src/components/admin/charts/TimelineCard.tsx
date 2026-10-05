import React from "react";
import { ChartCardShell, MONO, MUTED } from "./ChartCardShell";
import { TIMELINE_SEVERITIES, computeTimeline } from "./timeline.logic";
import type { TimelineInput } from "./timeline.logic";

export interface TimelineCardProps extends TimelineInput {
  subtitle?: string;
  emptyText?: string;
  grow?: number;
  basis?: number;
}

export const TimelineCard: React.FC<TimelineCardProps> = ({ subtitle = "", emptyText = "Aucun incident sur la période.", grow = 1, basis = 520, ...input }) => {
  const m = computeTimeline(input);
  const rule = "1px solid rgba(255,255,255,0.07)";
  return (
    <ChartCardShell title={m.title} subtitle={subtitle} gap={20} grow={grow} basis={basis}>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        <div role="img" aria-label={m.ariaLabel} style={{ display: "flex", alignItems: "flex-end", gap: m.gap, height: 44 }}>
          {m.strip.map((c, i) => (
            <div key={i} title={c.title} style={{ flex: 1, minWidth: 0, borderRadius: 3, height: c.height, background: c.color }} />
          ))}
        </div>
        <div aria-hidden="true" style={{ display: "flex", justifyContent: "space-between", fontFamily: MONO, fontSize: 11, color: MUTED }}>
          <span>{m.firstDay}</span>
          <span>{m.lastDay}</span>
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px 16px", fontSize: 12, color: MUTED }}>
          {m.legend.map((g) => (
            <span key={g.severity} style={{ display: "inline-flex", alignItems: "center", gap: 6, color: TIMELINE_SEVERITIES[g.severity].color }}>
              <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
                <path d={TIMELINE_SEVERITIES[g.severity].shape} fill="currentColor" />
              </svg>
              <span style={{ color: MUTED }}>{g.label}</span>
            </span>
          ))}
        </div>
      </div>
      {m.incidents.length > 0 && (
        <ol style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column" }}>
          {m.incidents.map((x, i) => {
            const s = TIMELINE_SEVERITIES[x.severity];
            return (
              <li key={i} style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", gap: "8px 20px", padding: "16px 0", borderTop: rule }}>
                <span style={{ flex: "0 0 150px", display: "flex", flexDirection: "column", gap: 2 }}>
                  <span style={{ fontWeight: 500 }}>{x.date}</span>
                  {x.meta && <span style={{ fontFamily: MONO, fontSize: 12, color: MUTED }}>{x.meta}</span>}
                </span>
                <span style={{ flex: "0 0 auto" }}>
                  <span
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 6,
                      fontFamily: MONO,
                      fontSize: 11,
                      letterSpacing: "0.08em",
                      textTransform: "uppercase",
                      padding: "4px 10px",
                      borderRadius: 999,
                      background: s.pillBg,
                      color: s.pillFg,
                    }}
                  >
                    <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
                      <path d={s.shape} fill="currentColor" />
                    </svg>
                    {s.label}
                  </span>
                </span>
                <span style={{ flex: "1 1 280px", minWidth: 0, display: "flex", flexDirection: "column", gap: 4 }}>
                  <span style={{ fontWeight: 500 }}>{x.title}</span>
                  {x.cause && <span style={{ fontSize: 13, color: MUTED }}>{x.cause}</span>}
                </span>
              </li>
            );
          })}
        </ol>
      )}
      {m.incidents.length === 0 && (
        <p role="status" style={{ margin: 0, padding: "16px 0", borderTop: rule, fontSize: 13, color: MUTED }}>
          {emptyText}
        </p>
      )}
    </ChartCardShell>
  );
};

TimelineCard.displayName = "TimelineCard";

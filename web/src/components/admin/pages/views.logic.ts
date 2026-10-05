// Pure mapping from GET /views to the props of the Visionnages blocks.
import type { AdminViews } from "@/lib/admin/types";
import { cap, dayLabel, fmtDec, fmtInt, sum, tzLabel } from "./fr-date";

export interface ViewsKpi {
  label: string;
  value: string | number | null;
  unit?: string;
  delta: number | null;
  deltaUnit: string;
  series: number[];
}

export function totalSessions(views: AdminViews): { current: number; previous: number } {
  return {
    current: sum(views.sessions_series.map((d) => d.sessions)),
    previous: sum(views.sessions_series.map((d) => d.prev_sessions)),
  };
}

/** Share of sessions that reached the end, over the whole period (from the per-weekday breakdown). */
export function completionPct(views: AdminViews): number | null {
  const sessions = sum(views.completion_by_weekday.map((d) => d.sessions));
  if (sessions === 0) return null;
  return (sum(views.completion_by_weekday.map((d) => d.completed)) / sessions) * 100;
}

export function viewsKpis(views: AdminViews): ViewsKpi[] {
  const { current, previous } = totalSessions(views);
  const completion = completionPct(views);
  const epu = views.episodes_per_active_user;
  const hasSessions = current > 0;
  return [
    {
      label: "Séances",
      value: current,
      delta: previous > 0 ? ((current - previous) / previous) * 100 : null,
      deltaUnit: "%",
      series: views.sessions_series.map((d) => d.sessions),
    },
    { label: "Durée moyenne", value: hasSessions ? fmtDec(views.avg_session_minutes, 1) : null, unit: hasSessions ? "min" : undefined, delta: null, deltaUnit: "%", series: [] },
    { label: "Épisodes terminés", value: completion === null ? null : `${fmtDec(completion, 1)} %`, delta: null, deltaUnit: "pt", series: [] },
    { label: "Épisodes par actif", value: epu.value > 0 ? fmtDec(epu.value, 1) : null, delta: epu.delta_pct, deltaUnit: "%", series: [] },
    {
      label: "Part de reprises",
      value: hasSessions ? `${fmtDec(views.resume_vs_first.resume_pct, 1)} %` : null,
      delta: null,
      deltaUnit: "pt",
      series: [],
    },
  ];
}

export interface ViewsSessionsChart {
  current: number[];
  previous: number[];
  labels: string[];
  top: number;
  summary: string;
}

export function sessionsChart(views: AdminViews): ViewsSessionsChart {
  const rows = views.sessions_series;
  const current = rows.map((d) => d.sessions);
  const previous = rows.map((d) => d.prev_sessions);
  const max = Math.max(0, ...current, ...previous);
  const n = rows.length || 1;
  return {
    current,
    previous,
    labels: rows.map((d) => dayLabel(d.day)),
    top: Math.max(4, Math.ceil((max * 1.08) / 4) * 4),
    summary: `Moyenne : ${fmtInt(sum(current) / n)} par jour · période précédente : ${fmtInt(sum(previous) / n)}`,
  };
}

const BUCKET_LABELS: Record<string, string> = {
  "<5": "< 5 min",
  "5-15": "5 à 15 min",
  "15-30": "15 à 30 min",
  "30-60": "30 à 60 min",
  ">60": "> 60 min",
};

export function durationBars(views: AdminViews): { values: number[]; labels: string[]; counts: string[]; ariaLabel: string; summary: string } {
  const b = views.session_length_buckets;
  const labels = b.map((x) => BUCKET_LABELS[x.range_minutes] ?? x.range_minutes);
  return {
    values: b.map((x) => x.share_pct),
    labels,
    counts: b.map((x) => fmtInt(x.sessions)),
    ariaLabel: `Distribution des durées de séance : ${b.map((x, i) => `${labels[i]} ${fmtDec(x.share_pct, 1)} %`).join(", ")}.`,
    summary: `Séances de ${fmtInt(sum(b.map((x) => x.sessions)))} sur la période`,
  };
}

export interface RetentionMark {
  index: number;
  to: number;
  label: string;
  detail: string;
  tone: "danger";
}

export function retention(views: AdminViews): { values: number[]; xLabels: string[]; yMin: number; marks: RetentionMark[]; ariaLabel: string; hasData: boolean } {
  const curve = views.retention_curve;
  const hasData = curve.some((c) => c.eligible > 0);
  const values = curve.map((c) => c.retained_pct);
  const lowest = values.length ? Math.min(...values) : 0;
  const yMin = hasData ? Math.max(0, Math.floor((lowest - 5) / 10) * 10) : 0;
  const marks = views.drop_points.map((d) => ({
    index: d.from_decile,
    to: d.to_decile,
    label: `De ${d.from_decile * 10} à ${d.to_decile * 10} % de l'épisode`,
    detail: `−${fmtDec(d.drop_pts, 1)} pt`,
    tone: "danger" as const,
  }));
  return {
    values,
    xLabels: curve.map((c) => `${c.from_pct} %`),
    yMin,
    marks,
    hasData,
    ariaLabel: `Courbe de rétention : ${curve.map((c) => `${c.from_pct} % de l'épisode, ${fmtDec(c.retained_pct, 1)} % encore présents`).join(" ; ")}.`,
  };
}

export interface WeekRow {
  label: string;
  value: number;
  delta: string | number | null;
  tone: "accent";
}

export function weekdayRows(views: AdminViews): { rows: WeekRow[]; summary: string } {
  const days = views.completion_by_weekday;
  const overall = completionPct(views);
  const rows = days.map((d) => ({
    label: cap(d.name),
    value: d.completion_pct,
    delta: overall === null || d.sessions === 0 ? null : d.completion_pct - overall,
    tone: "accent" as const,
  }));
  const withSessions = days.filter((d) => d.sessions > 0);
  const best = withSessions.length ? withSessions.reduce((a, b) => (b.completion_pct > a.completion_pct ? b : a)) : null;
  return { rows, summary: best ? `Meilleur jour : ${best.name}, ${fmtDec(best.completion_pct, 1)} %` : "Aucune séance sur la période" };
}

export function hourBars(views: AdminViews): { values: number[]; labels: string[]; peak: number[]; summary: string; ariaLabel: string } {
  const hours = views.sessions_by_hour;
  const total = sum(hours.map((h) => h.sessions));
  const values = hours.map((h) => (total > 0 ? (h.sessions / total) * 100 : 0));
  // Highlight the four busiest hours (only when there is something to highlight).
  const peak = total > 0
    ? [...hours].sort((a, b) => b.sessions - a.sessions || a.hour - b.hour).slice(0, 4).map((h) => h.hour).sort((a, b) => a - b)
    : [];
  const peakShare = sum(peak.map((h) => values[h] ?? 0));
  return {
    values,
    labels: hours.map((h) => `${h.hour} h`),
    peak,
    summary: total > 0 ? `${fmtDec(peakShare, 1)} % des séances sur les 4 heures les plus chargées` : "Aucune séance sur la période",
    ariaLabel: total > 0
      ? `Séances par heure UTC, 24 barres. Les heures les plus chargées sont ${peak.map((h) => `${h} h`).join(", ")}, ${fmtDec(peakShare, 1)} % des séances.`
      : "Séances par heure UTC : aucune séance sur la période.",
  };
}

export function timezoneItems(views: AdminViews): Array<{ label: string; hint: string; value: number; tone: "light" }> {
  return views.by_timezone.map((z) => ({
    label: tzLabel(z.tz_offset),
    hint: `${fmtInt(z.sessions)} séance${z.sessions > 1 ? "s" : ""}`,
    value: z.share_pct,
    tone: "light" as const,
  }));
}

export function resumeBlock(views: AdminViews): { segments: Array<{ label: string; value: number; tone: "accent" | "light" }>; resumePct: string; firstPct: string; resumeHint: string; firstHint: string; hasData: boolean } {
  const r = views.resume_vs_first;
  const total = r.resume + r.first;
  return {
    segments: [
      { label: "Reprises", value: r.resume, tone: "accent" },
      { label: "Premières lectures", value: r.first, tone: "light" },
    ],
    resumePct: total > 0 ? `${fmtDec(r.resume_pct, 1)} %` : "",
    firstPct: total > 0 ? `${fmtDec(100 - r.resume_pct, 1)} %` : "",
    resumeHint: `${fmtInt(r.resume)} séance${r.resume > 1 ? "s" : ""}`,
    firstHint: `${fmtInt(r.first)} séance${r.first > 1 ? "s" : ""}`,
    hasData: total > 0,
  };
}

export type LeaverRow = {
  id: string;
  title: string;
  ep: string;
  pct: number;
  min: number;
  n: number;
};

export function leaverRows(views: AdminViews): LeaverRow[] {
  return views.drop_episodes.map((d) => ({
    id: `${d.anime_id}-${d.season_id}-${d.episode}`,
    title: d.title || `Anime n° ${d.anime_id}`,
    ep: `Épisode ${d.episode}`,
    pct: d.abandon_pct,
    min: d.median_drop_minute,
    n: d.sessions,
  }));
}

export type ViewsInsightLevel = "haute" | "moyenne" | "basse";
export interface ViewsInsight {
  level: ViewsInsightLevel;
  title: string;
  text: string;
}

const RANK: Record<ViewsInsightLevel, number> = { haute: 0, moyenne: 1, basse: 2 };

/** Chiffres-constats computed from the response; at most three, most severe first. */
export function viewsInsights(views: AdminViews): ViewsInsight[] {
  const out: ViewsInsight[] = [];

  const worst = views.drop_episodes[0];
  if (worst) {
    const level: ViewsInsightLevel = worst.abandon_pct >= 30 ? "haute" : worst.abandon_pct >= 20 ? "moyenne" : "basse";
    out.push({
      level,
      title: `${worst.title || `Anime n° ${worst.anime_id}`}, épisode ${worst.episode} : ${fmtDec(worst.abandon_pct, 0)} % d'abandon avant 25 %, minute médiane ${fmtDec(worst.median_drop_minute, 1)} (${fmtInt(worst.sessions)} séance${worst.sessions > 1 ? "s" : ""}).`,
      text: "Piste : contrôler la source et les sous-titres de cet épisode, puis la qualité du flux.",
    });
  }

  const point = views.drop_points[0];
  if (point) {
    const early = views.drop_points.filter((d) => d.from_decile < 3);
    const earlyLoss = sum(early.map((d) => d.drop_pts));
    out.push({
      level: "basse",
      title: `Le plus gros décrochage se situe entre ${point.from_decile * 10} % et ${point.to_decile * 10} % de l'épisode : −${fmtDec(point.drop_pts, 1)} pt.`,
      text: earlyLoss > 0
        ? `${fmtDec(earlyLoss, 1)} pt perdus avant 30 % de l'épisode. Piste : tester un saut d'intro plus visible.`
        : "Piste : regarder le milieu d'épisode (scènes d'action, coupures publicitaires, sous-titres).",
    });
  }

  const comp = completionPct(views);
  const { current } = totalSessions(views);
  if (comp !== null && current > 0 && comp < 40) {
    out.push({
      level: "moyenne",
      title: `Seulement ${fmtDec(comp, 1)} % des séances vont au bout de l'épisode.`,
      text: "Piste : croiser avec les épisodes qui font décrocher ci-dessous.",
    });
  }

  return out.sort((a, b) => RANK[a.level] - RANK[b.level]).slice(0, 3);
}

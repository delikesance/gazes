// Pure mapping from the admin API responses to the props of the Vue d'ensemble blocks.
import type { AdminOverview, AdminPlaybackErrors, AdminPlaybackHealth, Kpi } from "@/lib/admin/types";
import { dayLabel, fmtDec, fmtInt, MISSING, pointsDelta, relativeFr, sum } from "./fr-date";

export interface OverviewKpi {
  label: string;
  value: string | number | null;
  unit?: string;
  delta: number | null;
  deltaUnit: string;
  decimals?: number;
  series: number[];
}

export type OverviewInsightLevel = "haute" | "moyenne" | "basse";
export interface OverviewInsight {
  level: OverviewInsightLevel;
  title: string;
  text: string;
  href: string;
  linkLabel: string;
}

const LEVEL_RANK: Record<OverviewInsightLevel, number> = { haute: 0, moyenne: 1, basse: 2 };

export function overviewKpis(overview: AdminOverview): OverviewKpi[] {
  const k = overview.kpis;
  const pct = (kpi: Kpi): number | null => kpi.delta_pct;
  const completion = pointsDelta(k.completion_pct.value, k.completion_pct.previous);
  return [
    { label: "Inscrits au total", value: k.total_users.value, delta: pct(k.total_users), deltaUnit: "%", series: [] },
    { label: "Nouveaux inscrits", value: k.new_users.value, delta: pct(k.new_users), deltaUnit: "%", series: overview.series.map((d) => d.new_users) },
    { label: "Visionnages", value: k.sessions.value, delta: pct(k.sessions), deltaUnit: "%", series: overview.series.map((d) => d.sessions) },
    { label: "Actifs par jour", value: fmtDec(k.active_users_avg.value, 1), delta: pct(k.active_users_avg), deltaUnit: "%", series: overview.series.map((d) => d.active_users) },
    { label: "Temps regardé", value: fmtDec(k.watch_hours.value, 1), unit: "h", delta: pct(k.watch_hours), deltaUnit: "%", series: [] },
    { label: "Épisodes terminés", value: `${fmtDec(k.completion_pct.value, 1)} %`, delta: completion, deltaUnit: "pt", series: [] },
  ];
}

export interface OverviewSessionsChart {
  values: number[];
  labels: string[];
  top: number;
  summary: string;
}

export function sessionsChart(overview: AdminOverview): OverviewSessionsChart {
  const values = overview.series.map((d) => d.sessions);
  const max = values.length ? Math.max(...values) : 0;
  const avg = values.length ? sum(values) / values.length : 0;
  return {
    values,
    labels: overview.series.map((d) => dayLabel(d.day)),
    top: Math.max(4, Math.ceil((max * 1.08) / 4) * 4),
    summary: `Pic : ${fmtInt(max)} · moyenne : ${fmtInt(avg)}`,
  };
}

export interface OverviewSignups {
  values: number[];
  firstDay: string;
  lastDay: string;
  total: number;
  best: string;
}

export function signupsBlock(overview: AdminOverview): OverviewSignups {
  const days = overview.series;
  const values = days.map((d) => d.new_users);
  const max = values.length ? Math.max(...values) : 0;
  const bestDay = max > 0 ? days[values.indexOf(max)] : undefined;
  return {
    values,
    firstDay: days.length ? dayLabel(days[0].day) : "",
    lastDay: days.length ? dayLabel(days[days.length - 1].day) : "",
    total: sum(values),
    best: bestDay ? `${fmtInt(max)} inscrit${max > 1 ? "s" : ""} (${dayLabel(bestDay.day)})` : "",
  };
}

export interface OverviewTopRow {
  rank: number;
  title: string;
  meta: string;
  value: number;
  bar: number;
  trend: number | null;
  poster: string | null;
}

export function topAnimeRows(overview: AdminOverview, posters: Record<number, string> = {}): OverviewTopRow[] {
  const top = overview.top_anime;
  const max = top.length ? Math.max(...top.map((a) => a.share_pct)) : 0;
  return top.map((a, i) => ({
    rank: i + 1,
    title: a.title || `Anime n° ${a.anime_id}`,
    meta: `${fmtDec(a.share_pct, 1)} % des visionnages`,
    value: a.sessions,
    bar: max > 0 ? Math.round((a.share_pct / max) * 100) : 0,
    trend: a.delta_pct,
    poster: posters[a.anime_id] ?? null,
  }));
}

export interface OverviewHeatmap {
  rowLabels: string[];
  colLabels: string[];
  values: number[][];
  ariaLabel: string;
}

export function heatmapBlock(overview: AdminOverview): OverviewHeatmap {
  const h = overview.heatmap;
  const cols = h.normalized.reduce((m, row) => Math.max(m, row.length), 0);
  const peak = overview.peak;
  return {
    rowLabels: h.weekdays.map((d) => d.charAt(0).toUpperCase() + d.slice(1)),
    colLabels: Array.from({ length: cols }, (_, i) => `${i} h`),
    values: h.normalized,
    ariaLabel: peak
      ? `Carte de chaleur des séances par jour de la semaine et par heure UTC. Le pic se situe le ${peak.weekday_name} à ${peak.hour} h UTC, avec ${peak.sessions} séance${peak.sessions > 1 ? "s" : ""}.`
      : "Carte de chaleur des séances par jour de la semaine et par heure UTC. Aucune séance sur la période.",
  };
}

const ERROR_LEVELS: Array<{ min: number; level: OverviewInsightLevel }> = [
  { min: 5, level: "haute" },
  { min: 1, level: "moyenne" },
];

/** Constats chiffrés derived from what the API returned; at most three, most severe first. */
export function overviewInsights(overview: AdminOverview, health: AdminPlaybackHealth | null): OverviewInsight[] {
  const out: OverviewInsight[] = [];

  const rate = health?.error_rate;
  const rateValue = rate?.value ?? null;
  if (rate && rate.measured && rateValue !== null) {
    const level = ERROR_LEVELS.find((l) => rateValue >= l.min)?.level ?? "basse";
    const trend = rate.delta_pct === null ? "" : ` (${rate.delta_pct >= 0 ? "+" : "−"}${fmtDec(Math.abs(rate.delta_pct), 1)} % sur la période précédente)`;
    out.push({
      level,
      title: `Erreurs de lecture : ${fmtDec(rateValue, 1)} % des séances`,
      text: `${fmtInt(rate.errors)} erreur${rate.errors > 1 ? "s" : ""} enregistrée${rate.errors > 1 ? "s" : ""} pour ${fmtInt(rate.sessions)} séance${rate.sessions > 1 ? "s" : ""}${trend}.`,
      href: "/admin/playback",
      linkLabel: "Ouvrir Lecteur et flux",
    });
  }

  const completion = overview.kpis.completion_pct.value;
  if (overview.kpis.sessions.value > 0 && completion < 40) {
    out.push({
      level: "moyenne",
      title: `Seulement ${fmtDec(completion, 1)} % des épisodes sont terminés`,
      text: "Repérer les épisodes qui font décrocher pour agir sur les sources ou les sous-titres.",
      href: "/admin/views",
      linkLabel: "Ouvrir Visionnages",
    });
  }

  const lead = overview.top_anime[0];
  if (lead && lead.share_pct >= 50 && overview.top_anime.length > 1) {
    out.push({
      level: "moyenne",
      title: `${lead.title || `Anime n° ${lead.anime_id}`} concentre ${fmtDec(lead.share_pct, 1)} % des séances`,
      text: "L'audience repose sur très peu de titres : le reste du catalogue pèse peu sur la période.",
      href: "/admin/catalog",
      linkLabel: "Ouvrir Catalogue",
    });
  }

  if (overview.peak) {
    const p = overview.peak;
    out.push({
      level: "basse",
      title: `Pic d'audience le ${p.weekday_name} à ${p.hour} h UTC`,
      text: `${fmtInt(p.sessions)} séance${p.sessions > 1 ? "s" : ""} sur ce créneau : c'est la fenêtre où la charge du lecteur est la plus forte.`,
      href: "/admin/views",
      linkLabel: "Ouvrir Visionnages",
    });
  }

  return out.sort((a, b) => LEVEL_RANK[a.level] - LEVEL_RANK[b.level]).slice(0, 3);
}

export interface OverviewPlayback {
  activeSessions: string;
  startup: string;
  errorRate: string;
}

export function playbackTiles(health: AdminPlaybackHealth | null): OverviewPlayback {
  if (!health) return { activeSessions: "", startup: "", errorRate: "" };
  const a = health.active_sessions;
  const s = health.startup_ms;
  const r = health.error_rate;
  return {
    activeSessions: a.measured && a.value !== null ? fmtInt(a.value) : "",
    startup: s.measured && s.p50 !== null ? `${fmtDec(s.p50 / 1000, 1)} s` : "",
    errorRate: r.measured && r.value !== null ? `${fmtDec(r.value, 1)} %` : "",
  };
}

export interface OverviewErrorRow {
  key: string;
  code: string;
  text: string;
  when: string;
}

/** Latest error groups (the API already sorts them by last_seen, newest first). */
export function latestErrors(errors: AdminPlaybackErrors | null, nowIso: string, count = 3): OverviewErrorRow[] {
  if (!errors) return [];
  return errors.items.slice(0, count).map((e, i) => {
    const where = [
      e.anime_id !== null ? `Anime n° ${e.anime_id}` : "Anime inconnu",
      e.episode !== null ? `ép. ${e.episode}` : null,
      e.source ?? null,
    ].filter(Boolean).join(" · ");
    const times = e.occurrences > 1 ? `, ${fmtInt(e.occurrences)} occurrences` : "";
    return { key: `${e.code}-${e.anime_id}-${e.episode}-${e.source}-${i}`, code: e.code, text: `${where}${times}`, when: relativeFr(e.last_seen, nowIso) };
  });
}

export { MISSING };

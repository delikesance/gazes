// Pure mapping from GET /growth to the props of the Croissance blocks.
import type { AdminGrowth } from "@/lib/admin/types";
import { dayLabel, fmtDec, fmtInt, pointsDelta } from "./fr-date";

/** True when the API lists this figure among those it cannot measure yet. */
export function isNotMeasured(growth: AdminGrowth, key: string): boolean {
  return growth.not_measured.includes(key);
}

/** Accounts at the end of the period (last point of the cumulative curve). */
export function totalUsers(growth: AdminGrowth): number | null {
  const last = growth.cumulative_users[growth.cumulative_users.length - 1];
  return last ? last.users : null;
}

export interface GrowthKpi {
  label: string;
  value: string | number | null;
  note: string;
  delta: number | null;
  deltaUnit: string;
  vs: string;
  series: number[];
}

export function growthKpis(growth: AdminGrowth, periodDays: number): GrowthKpi[] {
  const a = growth.activity;
  const total = totalUsers(growth);
  const share = (v: number) => (total && total > 0 ? `${fmtDec((v / total) * 100, 1)} % des inscrits` : "");
  const vs = `vs ${periodDays} j préc.`;
  const sticky = growth.activity_series.map((d) => (d.mau > 0 ? (d.dau / d.mau) * 100 : 0));
  return [
    {
      label: "DAU, actifs du jour",
      value: a.dau.value,
      note: "Dernier jour de la période",
      delta: a.dau.delta_pct,
      deltaUnit: "%",
      vs,
      series: growth.activity_series.map((d) => d.dau),
    },
    {
      label: "WAU, actifs sur 7 j",
      value: a.wau.value,
      note: share(a.wau.value),
      delta: a.wau.delta_pct,
      deltaUnit: "%",
      vs,
      series: growth.activity_series.map((d) => d.wau),
    },
    {
      label: "MAU, actifs sur 30 j",
      value: a.mau.value,
      note: share(a.mau.value),
      delta: a.mau.delta_pct,
      deltaUnit: "%",
      vs,
      series: growth.activity_series.map((d) => d.mau),
    },
    {
      label: "Accroche DAU/MAU",
      value: a.mau.value > 0 ? `${fmtDec(a.stickiness_pct.value, 1)} %` : null,
      note: "Part des actifs du mois qui reviennent chaque jour",
      delta: pointsDelta(a.stickiness_pct.value, a.stickiness_pct.previous),
      deltaUnit: "pt",
      vs,
      series: sticky,
    },
  ];
}

const FUNNEL_SOURCES: Record<string, string> = {
  signup: "Date de création du compte",
  first_session: "Au moins une séance enregistrée",
  three_episodes: "Trois épisodes distincts vus",
  active_d7: "Séance le 7e jour après l'inscription",
  active_d30: "Séance le 30e jour après l'inscription",
};

export interface FunnelStep {
  label: string;
  value: number;
  source: string;
  eligible: number;
  unmeasured: boolean;
  detail: string;
}

export function funnelSteps(growth: AdminGrowth): FunnelStep[] {
  return growth.funnel.steps.map((s, i) => {
    const tooRecent = s.excluded_too_recent;
    const unmeasured = i > 0 && s.eligible === 0;
    const recent = `${fmtInt(tooRecent)} ${tooRecent > 1 ? "comptes trop récents exclus" : "compte trop récent exclu"}`;
    const excluded = `${fmtInt(tooRecent)} ${tooRecent > 1 ? "comptes exclus" : "compte exclu"}`;
    return {
      label: s.label,
      value: s.users,
      source: FUNNEL_SOURCES[s.key] ?? "",
      eligible: s.eligible,
      unmeasured,
      detail: unmeasured ? (tooRecent > 0 ? `Trop récent pour être mesuré : ${excluded}` : "Aucun compte éligible à cette étape") : tooRecent > 0 ? recent : "",
    };
  });
}

export function funnelNote(growth: AdminGrowth, periodDays: number): string {
  const n = growth.funnel.cohort_size;
  return `Cohorte : ${fmtInt(n)} ${n > 1 ? "comptes inscrits" : "compte inscrit"} sur les ${periodDays} derniers jours`;
}

/** "2026-03-02" + 6 -> "2026-03-08" (UTC). */
export function addDays(day: string, n: number): string {
  const t = Date.parse(`${day}T00:00:00Z`);
  if (!Number.isFinite(t)) return day;
  return new Date(t + n * 86_400_000).toISOString().slice(0, 10);
}

export interface CohortModel {
  rowLabels: string[];
  rowMeta: string[];
  colLabels: string[];
  values: Array<Array<number | null>>;
  texts: string[][];
  max: number;
  hasData: boolean;
}

const COHORT_COLS = ["J1", "J7", "J14", "J30"] as const;

export function cohortModel(growth: AdminGrowth): CohortModel {
  const cohorts = growth.cohorts;
  const rows = cohorts.map((c) => [c.d1, c.d7, c.d14, c.d30]);
  const average = COHORT_COLS.map((_, j) => {
    let num = 0;
    let den = 0;
    cohorts.forEach((c, i) => {
      const v = rows[i][j];
      if (v !== null && c.users > 0) {
        num += v * c.users;
        den += c.users;
      }
    });
    return den > 0 ? num / den : null;
  });
  const values = [...rows, average];
  const measured = values.flat().filter((v): v is number => v !== null);
  const top = measured.length ? Math.max(...measured) : 0;
  const max = top > 0 ? Math.min(100, Math.max(5, Math.ceil(top / 5) * 5)) : 100;
  return {
    rowLabels: [...cohorts.map((c) => `${dayLabel(c.week_start)} – ${dayLabel(addDays(c.week_start, 6))}`), "Moyenne pondérée"],
    rowMeta: [...cohorts.map((c) => fmtInt(c.users)), ""],
    colLabels: [...COHORT_COLS],
    values,
    texts: values.map((row) => row.map((v) => (v === null ? "" : `${Math.round(v)} %`))),
    max,
    hasData: measured.length > 0,
  };
}

export interface CumulativeModel {
  values: number[];
  labels: string[];
  yMin: number;
  summary: string;
  ariaLabel: string;
}

export function cumulativeModel(growth: AdminGrowth): CumulativeModel {
  const rows = growth.cumulative_users;
  const values = rows.map((r) => r.users);
  const min = values.length ? Math.min(...values) : 0;
  const max = values.length ? Math.max(...values) : 0;
  const gained = growth.funnel.cohort_size;
  return {
    values,
    labels: rows.map((r) => dayLabel(r.day)),
    yMin: min >= 100 ? Math.floor(min / 100) * 100 : 0,
    summary: `${gained > 0 ? "+" : ""}${fmtInt(gained)} sur la période`,
    ariaLabel: `Courbe du nombre cumulé d'inscrits, de ${fmtInt(min)} à ${fmtInt(max)}.`,
  };
}

export interface WeeksModel {
  values: number[];
  labels: string[];
  best: number;
  current: number;
  ariaLabel: string;
  subtitle: string;
}

export function weeksModel(growth: AdminGrowth): WeeksModel {
  const rows = growth.signups_per_week;
  const values = rows.map((r) => r.signups);
  const min = values.length ? Math.min(...values) : 0;
  const best = values.length ? Math.max(...values) : 0;
  return {
    values,
    labels: rows.map((r) => dayLabel(r.week_start)),
    best,
    current: values.length ? values[values.length - 1] : 0,
    ariaLabel: `Inscrits par semaine sur ${values.length} ${values.length > 1 ? "semaines" : "semaine"}, de ${fmtInt(min)} à ${fmtInt(best)} par semaine.`,
    subtitle: rows.length ? `${rows.length} ${rows.length > 1 ? "semaines" : "semaine"}, du lundi ${dayLabel(rows[0].week_start)}` : "",
  };
}

export interface DelayModel {
  hasData: boolean;
  tiles: Array<{ label: string; value: string }>;
  bars: Array<{ key: string; name: string; value: number; text: string; tone: "accent" | "muted" }>;
}

export function delayModel(growth: AdminGrowth): DelayModel {
  const t = growth.time_to_first_session;
  const share = (k: string) => t.buckets.find((b) => b.key === k)?.share_pct ?? 0;
  const never = share("never");
  const within24 = share("lt_1h") + share("1h_24h");
  return {
    hasData: t.cohort_size > 0,
    tiles: [
      { label: "Délai médian", value: "" },
      { label: "Première séance sous 24 h", value: t.cohort_size > 0 ? `${fmtDec(within24, 1)} %` : "" },
      { label: "Ont déjà regardé", value: t.cohort_size > 0 ? `${fmtDec(100 - never, 1)} %` : "" },
    ],
    bars: t.buckets.map((b) => ({
      key: b.key,
      name: b.label,
      value: b.share_pct,
      text: `${fmtInt(b.users)} · ${fmtDec(b.share_pct, 1)} %`,
      tone: b.key === "never" ? "muted" : "accent",
    })),
  };
}

export interface ChurnModel {
  measured: boolean;
  value: number;
  tiles: Array<{ label: string; value: string }>;
  note: string;
}

export function churnModel(growth: AdminGrowth): ChurnModel {
  const c = growth.churn;
  const pctValue = c.churn_pct;
  return {
    measured: pctValue !== null,
    value: pctValue ?? 0,
    tiles: [
      { label: "Actifs la fenêtre précédente", value: fmtInt(c.previous_window_active) },
      { label: "Comptes perdus", value: fmtInt(c.churned) },
      { label: "Rétention", value: pctValue === null ? "" : `${fmtDec(100 - pctValue, 1)} %` },
      { label: "Objectif de churn", value: "" },
    ],
    note:
      pctValue === null
        ? `Aucun compte n'était actif sur les ${c.window_days} jours précédents : le churn ne peut pas être calculé.`
        : `Churn = comptes actifs sur les ${c.window_days} jours précédents et absents sur les ${c.window_days} derniers jours, divisés par les actifs de la fenêtre précédente. Fenêtres glissantes : indépendantes de la période choisie.`,
  };
}

export interface GrowthInsight {
  level: "haute" | "moyenne" | "basse";
  title: string;
  text: string;
}

export function growthInsights(growth: AdminGrowth): GrowthInsight[] {
  const out: GrowthInsight[] = [];

  let worst: { label: string; loss: number; lost: number } | null = null;
  growth.funnel.steps.forEach((s, i) => {
    if (i === 0 || s.conversion_pct === null || s.eligible <= 0) return;
    const loss = 100 - s.conversion_pct;
    if (loss > 0 && (!worst || loss > worst.loss)) worst = { label: s.label, loss, lost: s.eligible - s.users };
  });
  if (worst) {
    const w: { label: string; loss: number; lost: number } = worst;
    out.push({
      level: w.loss >= 50 ? "haute" : w.loss >= 25 ? "moyenne" : "basse",
      title: `La plus forte perte de l'entonnoir est « ${w.label} » : −${fmtDec(w.loss, 1)} % (${fmtInt(w.lost)} ${w.lost > 1 ? "comptes" : "compte"}).`,
      text: "Regarder ce qui se passe juste avant cette étape : fin d'épisode, enchaînement automatique, erreurs de lecture (voir Lecteur et flux).",
    });
  }

  const churn = growth.churn;
  if (churn.churn_pct !== null && churn.churned > 0) {
    out.push({
      level: churn.churn_pct >= 40 ? "haute" : churn.churn_pct >= 20 ? "moyenne" : "basse",
      title: `Le churn est de ${fmtDec(churn.churn_pct, 1)} % : ${fmtInt(churn.churned)} ${churn.churned > 1 ? "comptes" : "compte"} sur ${fmtInt(churn.previous_window_active)} ${churn.previous_window_active > 1 ? "actifs" : "actif"} ${churn.window_days} jours plus tôt ${churn.churned > 1 ? "ne sont pas revenus" : "n'est pas revenu"}.`,
      text: "Aucun objectif de churn n'est défini : en fixer un pour que la jauge indique un écart.",
    });
  }

  const ttf = growth.time_to_first_session;
  const never = ttf.buckets.find((b) => b.key === "never");
  if (ttf.cohort_size > 0 && never && never.share_pct >= 10) {
    out.push({
      level: never.share_pct >= 50 ? "haute" : never.share_pct >= 25 ? "moyenne" : "basse",
      title: `${fmtDec(never.share_pct, 1)} % des comptes créés sur la période (${fmtInt(never.users)}) n'ont encore lancé aucune séance.`,
      text: "Tester une relance le lendemain de l'inscription qui reprend directement le premier épisode proposé.",
    });
  }
  return out;
}

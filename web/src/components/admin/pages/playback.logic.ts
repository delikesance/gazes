// Pure mapping from the playback endpoints to the props of the Lecteur et flux blocks.
import type {
  AdminCosts,
  AdminErrorsSummary,
  AdminIssue,
  AdminIssuesList,
  AdminPlaybackErrorItem,
  AdminPlaybackHealth,
  AdminSources,
} from "@/lib/admin/types";
import { dateTimeFr, dateTimeShortFr, dayLabel, fmtDec, fmtInt, MISSING } from "./fr-date";

export type Level = "haute" | "moyenne" | "basse";
const LEVEL_RANK: Record<Level, number> = { haute: 0, moyenne: 1, basse: 2 };

// ---------- KPI ----------

export interface PlaybackKpi {
  label: string;
  value: string | number | null;
  unit?: string;
  delta?: number | null;
  /** Decreasing is good (errors, failures). */
  goodDown?: boolean;
  note?: string;
  tag?: string;
  series?: number[];
}

export type CacheStatus = "disabled" | "ok" | "unreachable" | "unknown";

export function cacheStatus(health: AdminPlaybackHealth): CacheStatus {
  const cache = health.cache;
  if (!cache.measured || !cache.data) return "unknown";
  const redis = cache.data.redis;
  if (redis === "disabled") return "disabled";
  return redis.status === "ok" ? "ok" : "unreachable";
}

const CACHE_LABEL: Record<CacheStatus, string> = {
  disabled: "Désactivé",
  ok: "Connecté",
  unreachable: "Injoignable",
  unknown: "",
};

export function playbackKpis(health: AdminPlaybackHealth, summary: AdminErrorsSummary, costs: AdminCosts | null): PlaybackKpi[] {
  const active = health.active_sessions;
  const start = health.startup_ms;
  const rate = health.error_rate;
  const redis = health.cache.data?.redis;
  const latency = redis && redis !== "disabled" && redis.latency_ms !== undefined ? `Latence ${fmtDec(redis.latency_ms, 0)} ms` : undefined;
  const peak = costs?.usage.peak_concurrent_sessions;
  const kpis: PlaybackKpi[] = [
    { label: "Séances actives", value: active.measured && active.value !== null ? active.value : null },
    { label: "Démarrage médian", value: start.measured && start.p50 !== null ? fmtDec(start.p50 / 1000, 1) : null, unit: start.measured && start.p50 !== null ? "s" : undefined },
    { label: "Démarrage p95", value: start.measured && start.p95 !== null ? fmtDec(start.p95 / 1000, 1) : null, unit: start.measured && start.p95 !== null ? "s" : undefined },
    {
      label: "Erreurs de lecture",
      value: rate.measured && rate.value !== null ? fmtDec(rate.value, 1) : null,
      unit: rate.measured && rate.value !== null ? "%" : undefined,
      delta: rate.delta_pct,
      goodDown: true,
      note: rate.sessions > 0 ? `${fmtInt(rate.errors)} erreur${rate.errors > 1 ? "s" : ""} pour ${fmtInt(rate.sessions)} séance${rate.sessions > 1 ? "s" : ""}` : "Aucune séance sur la période",
    },
    {
      label: "Erreurs enregistrées",
      value: summary.total.value,
      delta: summary.total.delta_pct,
      goodDown: true,
      series: summary.daily.map((d) => d.total),
    },
    {
      label: "Sources en échec",
      value: health.sources.failing,
      goodDown: true,
      note: health.sources.measured && health.sources.active !== null && health.sources.total !== null
        ? `${health.sources.active} actives sur ${health.sources.total}`
        : "Sources actives : [À MESURER]",
    },
    { label: "Cache Redis", value: CACHE_LABEL[cacheStatus(health)] || null, note: latency },
  ];
  if (peak && peak.value !== null) {
    kpis.push({ label: "Pic de séances simultanées", value: peak.value, tag: peak.estimated ? "Estimation" : undefined });
  }
  return kpis;
}

// ---------- error codes ----------

export function errorsByCode(summary: AdminErrorsSummary): Array<{ label: string; value: number; hint: string; delta: number | null; tone: "accent" }> {
  const total = summary.total.value;
  return summary.by_code
    .filter((c) => c.count.value > 0)
    .map((c) => ({
      label: c.code,
      value: c.count.value,
      hint: total > 0 ? `${fmtDec((c.count.value / total) * 100, 1)} %` : "",
      delta: c.count.delta_pct,
      tone: "accent" as const,
    }));
}

export function dailyTrend(summary: AdminErrorsSummary): { values: number[]; labels: string[]; max: number; maxText: string; ariaLabel: string } {
  const values = summary.daily.map((d) => d.total);
  const max = values.length ? Math.max(...values) : 0;
  return {
    values,
    labels: summary.daily.map((d) => dayLabel(d.day)),
    max,
    maxText: `max : ${fmtInt(max)} / jour`,
    ariaLabel: `Erreurs de lecture par jour sur ${values.length} jours, jusqu'à ${fmtInt(max)} par jour.`,
  };
}

// ---------- journal ----------

export const COPY_COLUMN = "ctx";

export type JournalRow = {
  id: string;
  time: string;
  code: string;
  show: string;
  src: string;
  n: number;
  ctx: { label: string; ariaLabel: string };
  ctxTone: "accent" | "danger" | "";
};

export function errorLabel(e: AdminPlaybackErrorItem): string {
  if (e.anime_id === null) return "—";
  return e.episode === null ? `Anime n° ${e.anime_id}` : `Anime n° ${e.anime_id} · ép. ${e.episode}`;
}

export function journalId(e: AdminPlaybackErrorItem, index: number): string {
  return `${e.code}|${e.anime_id}|${e.episode}|${e.source}|${index}`;
}

export type CopyState = { id: string; status: "copied" | "failed" } | null;

export function journalRows(items: ReadonlyArray<AdminPlaybackErrorItem>, copy: CopyState): JournalRow[] {
  return items.map((e, i) => {
    const id = journalId(e, i);
    const mine = copy && copy.id === id ? copy.status : null;
    const label = mine === "copied" ? "Copié" : mine === "failed" ? "Échec de la copie" : "Copier le contexte";
    return {
      id,
      time: dateTimeShortFr(e.last_seen),
      code: e.code,
      show: errorLabel(e),
      src: e.source ?? "—",
      n: e.occurrences,
      ctx: { label, ariaLabel: `${label} : ${e.code}, ${errorLabel(e)}` },
      ctxTone: mine === "copied" ? "accent" : mine === "failed" ? "danger" : "",
    };
  });
}

export function journalTabs(items: ReadonlyArray<AdminPlaybackErrorItem>, allValue: string): Array<{ label: string; value: string }> {
  const counts = new Map<string, number>();
  for (const e of items) counts.set(e.code, (counts.get(e.code) ?? 0) + 1);
  return [
    { label: `Tous (${items.length})`, value: allValue },
    ...[...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([code, n]) => ({ label: `${code} (${n})`, value: code })),
  ];
}

/** Plain-text report of one error group, meant to be pasted into a conversation with Claude. */
export function copyText(e: AdminPlaybackErrorItem, summary: AdminErrorsSummary): string {
  const cause = summary.probable_causes.find((c) => c.code === e.code);
  const lines = [
    "Gazes, diagnostic du lecteur",
    `Dernière occurrence : ${dateTimeFr(e.last_seen)}`,
    `Première occurrence : ${dateTimeFr(e.first_seen)}`,
    `Code : ${e.code}`,
    `Anime : ${e.anime_id === null ? "inconnu" : `n° ${e.anime_id}`}`,
    `Épisode : ${e.episode === null ? "inconnu" : e.episode}`,
    `Source : ${e.source ?? "inconnue"}`,
    `Occurrences : ${e.occurrences}`,
  ];
  if (cause) {
    lines.push(`Cause probable : ${cause.probable_cause}`);
    if (cause.files.length > 0) lines.push(`Fichiers à examiner : ${cause.files.join(", ")}`);
  }
  return lines.join("\n");
}

// ---------- sources ----------

export type SourceRow = {
  id: string;
  source: string;
  failures: number;
  share: number;
  last: string;
  delta: number | null;
};

export function sourceRows(sources: AdminSources): SourceRow[] {
  return sources.sources.map((s) => ({
    id: s.source,
    source: s.source,
    failures: s.failures.value,
    share: s.share_pct,
    last: dateTimeShortFr(s.last_seen),
    delta: s.failures.delta_pct,
  }));
}

// ---------- cache ----------

export interface CacheTile {
  label: string;
  value: string;
}

export function cacheBlock(health: AdminPlaybackHealth): { subtitle: string; tiles: CacheTile[] } {
  const status = cacheStatus(health);
  const data = health.cache.data;
  const redis = data?.redis;
  const detail = redis && redis !== "disabled" ? redis : null;
  const subtitle = status === "unknown" ? MISSING : `Redis : ${CACHE_LABEL[status].toLowerCase()}`;
  const tiles: CacheTile[] = [
    { label: "Statut", value: CACHE_LABEL[status] },
    { label: "Latence", value: detail && detail.latency_ms !== undefined ? `${fmtDec(detail.latency_ms, 0)} ms` : "" },
    { label: "Clés", value: detail && detail.keys !== undefined ? fmtInt(detail.keys) : "" },
    { label: "Évictions", value: "" },
  ];
  if (data && data.anilist_cooldown_ms !== undefined) {
    tiles.push({ label: "Pause AniList", value: `${fmtInt(data.anilist_cooldown_ms)} ms` });
  }
  return { subtitle, tiles };
}

// ---------- À regarder ----------

export interface Finding {
  level: Level;
  title: string;
  text?: string;
  action?: string;
}

const ISSUE_LEVEL: Record<AdminIssue["severity"], Level> = { critical: "haute", high: "haute", medium: "moyenne", low: "basse" };

export function issueFindings(issues: AdminIssuesList | null): Finding[] {
  if (!issues) return [];
  return issues.items
    .filter((i) => i.status !== "resolved")
    .map((i) => ({
      level: ISSUE_LEVEL[i.severity],
      title: i.title,
      text: [i.evidence, i.source ? `Signalé par ${i.source}` : null].filter(Boolean).join(" · ") || undefined,
      action: i.suggested_fix ? `Piste : ${i.suggested_fix}` : undefined,
    }));
}

export function computedFindings(health: AdminPlaybackHealth, summary: AdminErrorsSummary, sources: AdminSources, periodDays: number): Finding[] {
  const out: Finding[] = [];
  const total = summary.total.value;
  const top = summary.by_code.filter((c) => c.count.value > 0).sort((a, b) => b.count.value - a.count.value)[0];
  if (top && total > 0) {
    const share = (top.count.value / total) * 100;
    if (share >= 50 || (top.count.delta_pct !== null && top.count.delta_pct >= 50 && top.count.value >= 5)) {
      const d = top.count.delta_pct;
      const trend = d === null ? "" : `, ${d >= 0 ? "+" : "−"}${fmtDec(Math.abs(d), 1)} % vs ${periodDays} j préc.`;
      const cause = summary.probable_causes.find((c) => c.code === top.code);
      out.push({
        level: d !== null && d >= 50 && top.count.value >= 5 ? "haute" : "moyenne",
        title: `${top.code} représente ${fmtDec(share, 1)} % des erreurs (${fmtInt(top.count.value)} sur ${fmtInt(total)})${trend}.`,
        action: cause ? `Cause probable : ${cause.probable_cause}${cause.files.length ? ` Fichiers : ${cause.files.join(", ")}.` : ""}` : undefined,
      });
    }
  }
  const rate = health.error_rate;
  if (rate.measured && rate.value !== null && rate.value >= 1) {
    out.push({
      level: rate.value >= 5 ? "haute" : "moyenne",
      title: `${fmtDec(rate.value, 1)} % des séances ont rencontré une erreur (${fmtInt(rate.errors)} sur ${fmtInt(rate.sessions)}).`,
      action: "Seuls les sites instrumentés sont comptés : le taux réel peut être plus élevé.",
    });
  }
  if (health.sources.failing > 0) {
    const worst = sources.sources[0];
    out.push({
      level: "moyenne",
      title: `${health.sources.failing} source${health.sources.failing > 1 ? "s" : ""} en échec sur la période${worst ? ` : ${worst.source} pèse ${fmtDec(worst.share_pct, 1)} % des échecs` : ""}.`,
      action: "Piste : retirer les sources mortes de la rotation ou basculer sur une autre source avant d'abandonner.",
    });
  }
  if (!health.startup_ms.measured) {
    out.push({
      level: "basse",
      title: `Temps de démarrage : ${MISSING}.`,
      action: "Piste : instrumenter la durée entre la demande de lecture et la première image pour obtenir p50 et p95.",
    });
  }
  return out;
}

export function allFindings(health: AdminPlaybackHealth, summary: AdminErrorsSummary, sources: AdminSources, issues: AdminIssuesList | null, periodDays: number): Finding[] {
  return [...issueFindings(issues), ...computedFindings(health, summary, sources, periodDays)]
    .sort((a, b) => LEVEL_RANK[a.level] - LEVEL_RANK[b.level])
    .slice(0, 6);
}

// ---------- causes probables ----------

export function probableCauses(summary: AdminErrorsSummary): AdminErrorsSummary["probable_causes"] {
  return summary.probable_causes.filter((c) => c.count > 0);
}

// ---------- not measured ----------

export interface NotMeasured {
  label: string;
  reason: string;
}

export function notMeasured(health: AdminPlaybackHealth, sources: AdminSources, costs: AdminCosts | null): NotMeasured[] {
  const out: NotMeasured[] = [];
  if (!health.startup_ms.measured) out.push({ label: "Temps de démarrage (p50, p95)", reason: "Le lecteur n'envoie pas encore cette mesure." });
  out.push({ label: "Rebuffering par heure", reason: "Aucune mesure des coupures de lecture n'est enregistrée." });
  if (!health.sources.measured) out.push({ label: "Sources actives sur le total", reason: "Le parc de sources n'est pas exposé par l'API." });
  if (sources.not_measured.includes("attempts_per_source")) out.push({ label: "Taux d'échec par source", reason: "Les tentatives par source ne sont pas comptées, seulement les échecs." });
  out.push({ label: "Torrents actifs", reason: "Aucun endpoint d'administration n'expose les torrents (pairs, débit, ratio de remux)." });
  out.push({ label: "Évictions du cache Redis", reason: "Les évictions ne sont pas exposées par le diagnostic du cache." });
  const storage = costs?.costs.storage;
  out.push({
    label: "Stockage disque",
    reason: storage && storage.missing_inputs && storage.missing_inputs.length > 0
      ? `Données manquantes : ${storage.missing_inputs.join(", ")}.`
      : "Le volume stocké n'est pas mesuré.",
  });
  out.push({ label: "Chronologie d'incidents", reason: "Aucun journal d'incidents n'est enregistré ; seuls les constats ci-dessus sont disponibles." });
  return out;
}



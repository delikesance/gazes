// Pure mapping from GET /catalog to the props of the Catalogue blocks.
import type { AdminCatalog } from "@/lib/admin/types";
import { dayLabel, fmtDec, fmtInt, sum } from "./fr-date";

export type CatalogFormat = AdminCatalog["format"];

export const CATALOG_FORMATS: ReadonlyArray<{ id: CatalogFormat; label: string }> = [
  { id: "tv", label: "TV" },
  { id: "movie", label: "Film" },
  { id: "ova", label: "OVA" },
  { id: "all", label: "Tous" },
];

export const DEFAULT_CATALOG_FORMAT: CatalogFormat = "all";

/** Reads a raw `?format=` value; anything the API would refuse falls back to "all". */
export function parseCatalogFormat(raw: string | string[] | null | undefined): CatalogFormat {
  const value = (Array.isArray(raw) ? raw[0] : raw) ?? "";
  return CATALOG_FORMATS.some((f) => f.id === value) ? (value as CatalogFormat) : DEFAULT_CATALOG_FORMAT;
}

/** The API groups the free-form session formats like this (internal/admin viewsFormatMatches). */
const FORMAT_KEYS: Record<Exclude<CatalogFormat, "all">, readonly string[]> = {
  tv: ["tv", "tv_short", "tv short"],
  movie: ["movie"],
  ova: ["ova", "ona", "special"],
};

/** Abandon threshold of "séries à fort abandon": at least 35 % of the sessions are not finished. */
export const LEAVER_MIN_ABANDON_PCT = 35;
/** A series needs this share of the sessions (and at least 5 sessions) to count as a leaver, so that one-off plays do not. */
export const LEAVER_MIN_SHARE_PCT = 0.5;
export const LEAVER_MIN_SESSIONS = 5;
/** Points drawn on the matrix (the labels would overlap beyond that); medians still cover every series. */
export const MATRIX_POINTS = 25;

/** Sessions of the period over every format (the format shares ignore the format filter). */
export function totalSessions(catalog: AdminCatalog): number {
  const byFormat = sum(catalog.formats.map((f) => f.sessions));
  return byFormat > 0 ? byFormat : sum(catalog.anime.map((a) => a.sessions));
}

export function pct(part: number, whole: number): number {
  return whole > 0 ? (part / whole) * 100 : 0;
}

/** Share of the sessions that belong to a format tab, from the per-format shares. */
export function formatShare(catalog: AdminCatalog, format: CatalogFormat): number | null {
  if (format === "all") return 100;
  const keys = FORMAT_KEYS[format];
  const total = sum(catalog.formats.map((f) => f.sessions));
  if (total <= 0) return null;
  return pct(sum(catalog.formats.filter((f) => keys.includes(f.key)).map((f) => f.sessions)), total);
}

export interface LeaverRow {
  id: number;
  title: string;
  format: string;
  abandon: number;
  sessions: number;
}

export function formatName(key: string): string {
  const names: Record<string, string> = { tv: "TV", movie: "Film", ova: "OVA", ona: "ONA", special: "Spécial", tv_short: "TV court", "tv short": "TV court", unknown: "Inconnu" };
  return names[key.toLowerCase()] ?? key.toUpperCase();
}

const LANGUAGES: Record<string, string> = { jp: "Japonais", ja: "Japonais", fr: "Français", en: "Anglais", es: "Espagnol", de: "Allemand", it: "Italien", pt: "Portugais", ko: "Coréen", zh: "Chinois", unknown: "Inconnue" };

export function langName(key: string): string {
  return LANGUAGES[key.toLowerCase()] ?? key.toUpperCase();
}

export function leavers(catalog: AdminCatalog): LeaverRow[] {
  const total = totalSessions(catalog);
  return catalog.anime
    .filter((a) => 100 - a.completion_pct >= LEAVER_MIN_ABANDON_PCT && a.sessions >= LEAVER_MIN_SESSIONS && pct(a.sessions, total) >= LEAVER_MIN_SHARE_PCT)
    .map((a) => ({ id: a.anime_id, title: a.title, format: formatName(a.format), abandon: 100 - a.completion_pct, sessions: a.sessions }))
    .sort((a, b) => b.abandon - a.abandon || b.sessions - a.sessions);
}

/** Average completion weighted by sessions over the listed series (null without any session). */
export function weightedCompletion(catalog: AdminCatalog): number | null {
  const sessions = sum(catalog.anime.map((a) => a.sessions));
  if (sessions <= 0) return null;
  return sum(catalog.anime.map((a) => a.sessions * a.completion_pct)) / sessions;
}

/** True when the table lists fewer series than the period saw (aggregates are then computed on the top of the list). */
export function isTruncated(catalog: AdminCatalog): boolean {
  return catalog.anime_total > catalog.anime.length;
}

export interface CatalogKpi {
  label: string;
  value: string | number | null;
  unit?: string;
  note?: string;
  tag?: string;
}

export function catalogKpis(all: AdminCatalog): CatalogKpi[] {
  const total = totalSessions(all);
  const top10 = sum(all.anime.slice(0, 10).map((a) => a.sessions));
  const completion = weightedCompletion(all);
  const leaverCount = leavers(all).length;
  const hasAnime = all.anime.length > 0;
  const truncated = isTruncated(all);
  const scope = truncated ? `Sur les ${fmtInt(all.anime.length)} séries les plus vues` : undefined;
  return [
    { label: "Animes regardés", value: all.anime_total },
    { label: "Part du top 10", value: hasAnime && total > 0 ? `${fmtDec(pct(top10, total), 1)} %` : null },
    { label: "Complétion moyenne", value: completion === null ? null : `${fmtDec(completion, 1)} %`, note: scope },
    { label: "Séries à fort abandon", value: hasAnime ? leaverCount : null, note: scope },
    { label: "Demandes sans source", value: null, tag: "À instrumenter" },
    { label: "Copies AV1 prêtes", value: null, tag: "À instrumenter" },
  ];
}

export interface CatalogInsight {
  level: "haute" | "moyenne" | "basse";
  title: string;
  text: string;
}

export function catalogInsights(all: AdminCatalog): CatalogInsight[] {
  const out: CatalogInsight[] = [];
  const total = totalSessions(all);
  const medC = all.quadrant.median_completion_pct;

  const watch = all.quadrant.points
    .filter((p) => p.quadrant_key === "watch")
    .sort((a, b) => b.sessions - a.sessions)[0];
  if (watch) {
    out.push({
      level: "haute",
      title: `${watch.title} : ${fmtDec(pct(watch.sessions, total), 1)} % des séances mais seulement ${fmtDec(watch.completion_pct, 1)} % de complétion, soit ${fmtDec(medC - watch.completion_pct, 1)} pt sous la médiane.`,
      text: "Piste : contrôler les épisodes où les spectateurs décrochent (voir Visionnages) et la qualité de la source.",
    });
  }

  const lv = leavers(all);
  if (lv.length > 0) {
    const worst = lv[0];
    out.push({
      level: "moyenne",
      title: `${fmtInt(lv.length)} ${lv.length > 1 ? "séries" : "série"} à fort abandon : ${worst.title} perd ${fmtDec(worst.abandon, 1)} % de ses séances avant la fin.`,
      text: "Piste : comparer avec le format et l'épisode le plus touché dans Visionnages.",
    });
  }

  const nv = all.new_vs_catalog;
  if (nv.new_series.series > 0 && nv.catalog.series > 0 && nv.catalog.sessions > 0) {
    const perNew = nv.new_series.sessions / nv.new_series.series;
    const perOld = nv.catalog.sessions / nv.catalog.series;
    if (perOld > 0 && perNew / perOld >= 1.2) {
      out.push({
        level: "basse",
        title: `Une nouveauté attire ${fmtDec(perNew / perOld, 1)} fois plus de séances qu'une série du fonds (${fmtDec(perNew, 1)} contre ${fmtDec(perOld, 1)} séances par série).`,
        text: "Piste : mettre en avant, dans les sélections, des séries du fonds que les spectateurs terminent.",
      });
    } else if (perNew > 0 && perOld / perNew >= 1.2) {
      out.push({
        level: "basse",
        title: `Une série du fonds attire ${fmtDec(perOld / perNew, 1)} fois plus de séances qu'une nouveauté (${fmtDec(perOld, 1)} contre ${fmtDec(perNew, 1)} séances par série).`,
        text: "Piste : vérifier la mise en avant des nouveautés sur l'accueil.",
      });
    }
  }
  return out;
}

export interface TopRow {
  id: number;
  rank: string;
  title: string;
  format: string;
  share: number;
  sessions: number;
  hours: number;
  completion: number;
  completionTone?: "danger";
  fresh: number;
  delta: number | null;
}

export function topRows(tab: AdminCatalog, total: number): TopRow[] {
  return tab.anime.map((a, i) => ({
    id: a.anime_id,
    rank: String(tab.offset + i + 1),
    title: a.title,
    format: formatName(a.format),
    share: pct(a.sessions, total),
    sessions: a.sessions,
    hours: a.watch_hours,
    completion: a.completion_pct,
    completionTone: a.completion_pct < 60 ? "danger" : undefined,
    fresh: a.new_viewers,
    delta: a.delta_pct,
  }));
}

export function topSummary(tab: AdminCatalog, all: AdminCatalog): string {
  const share = formatShare(all, tab.format);
  const count = `${fmtInt(tab.anime_total)} ${tab.anime_total > 1 ? "séries" : "série"}`;
  if (tab.format === "all") return `${count} ${tab.anime_total > 1 ? "regardées" : "regardée"} sur la période`;
  const label = tab.format === "tv" ? "Les formats TV" : tab.format === "movie" ? "Les films" : "Les OVA";
  return share === null ? count : `${label} représentent ${fmtDec(share, 1)} % des visionnages · ${count}`;
}

export function topNote(tab: AdminCatalog): string {
  const low = tab.anime.filter((a) => a.completion_pct < 60).slice(0, 6).map((a) => `${a.title} (${fmtDec(a.completion_pct, 1)} %)`);
  const parts = [
    "Nouveaux spectateurs : comptes dont c'est la première séance sur cet anime.",
    `Complétion sous 60 % : ${low.length ? low.join(", ") : "aucune série"}.`,
  ];
  if (isTruncated(tab)) parts.push(`Les ${fmtInt(tab.anime.length)} premières séries sur ${fmtInt(tab.anime_total)} sont affichées.`);
  return parts.join(" ");
}

export interface MatrixModel {
  points: Array<{ label: string; x: number; y: number }>;
  xMax: number;
  yMin: number;
  yMax: number;
  xThreshold: number;
  yThreshold: number;
  summary: string;
  rules: { topRight: string; topLeft: string; bottomRight: string; bottomLeft: string };
}

const EPS = 1e-6;

/** Matrix of popularity (share of the sessions) against completion; thresholds are the API medians (strictly above = high). */
export function matrix(all: AdminCatalog): MatrixModel {
  const total = totalSessions(all);
  const q = all.quadrant;
  const shown = [...q.points].sort((a, b) => b.sessions - a.sessions).slice(0, MATRIX_POINTS);
  const points = shown.map((p) => ({ label: p.title, x: pct(p.sessions, total), y: p.completion_pct }));
  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  const xThreshold = pct(q.median_sessions + EPS, total);
  const yThreshold = q.median_completion_pct + EPS;
  const xMax = Math.max(5, Math.ceil((Math.max(0, ...xs, xThreshold) * 1.1) / 5) * 5);
  const yMin = Math.max(0, Math.floor((Math.min(100, ...ys, yThreshold) - 5) / 10) * 10);
  const yMax = Math.min(100, Math.max(yMin + 10, Math.ceil((Math.max(0, ...ys, yThreshold) + 5) / 10) * 10));
  const medX = fmtDec(pct(q.median_sessions, total), 1);
  const medY = fmtDec(q.median_completion_pct, 1);
  return {
    points,
    xMax,
    yMin,
    yMax,
    xThreshold,
    yThreshold,
    summary: `${fmtInt(shown.length)} séries les plus vues sur ${fmtInt(all.anime_total)} · seuils : médiane des séances (${medX} %) et de la complétion (${medY} %)`,
    rules: {
      topRight: `Séances au-dessus de la médiane et complétion au-dessus de la médiane (${medY} %)`,
      topLeft: "Peu de séances mais complétion au-dessus de la médiane",
      bottomRight: "Beaucoup de séances mais complétion sous la médiane",
      bottomLeft: "Peu de séances et complétion sous la médiane",
    },
  };
}

export interface GenreRow {
  name: string;
  share: number;
  completion: number;
  shareBar: number;
  completionBar: number;
}

export const GENRES_SHOWN = 12;

export function genreRows(all: AdminCatalog): GenreRow[] {
  const maxShare = Math.max(1e-9, ...all.genres.map((g) => g.share_pct));
  return all.genres.slice(0, GENRES_SHOWN).map((g) => ({
    name: g.genre,
    share: g.share_pct,
    completion: g.completion_pct,
    shareBar: (g.share_pct / maxShare) * 100,
    completionBar: g.completion_pct,
  }));
}

export interface ShareItem {
  label: string;
  value: number;
  hint: string;
}

export function shareItems(rows: AdminCatalog["formats"], name: (key: string) => string): ShareItem[] {
  return rows.map((r) => ({ label: name(r.key), value: r.share_pct, hint: `${fmtInt(r.sessions)} ${r.sessions > 1 ? "séances" : "séance"}` }));
}

export interface SeasonCard {
  name: string;
  swatch: "accent" | "light";
  series: string;
  share: string;
  sessions: string;
  perSeries: string;
}

export function seasonCards(all: AdminCatalog): { segments: Array<{ label: string; value: number; tone: "accent" | "light" }>; cards: SeasonCard[]; note: string; hasData: boolean } {
  const nv = all.new_vs_catalog;
  const card = (name: string, swatch: "accent" | "light", g: AdminCatalog["new_vs_catalog"]["new_series"]): SeasonCard => ({
    name,
    swatch,
    series: fmtInt(g.series),
    share: `${fmtDec(g.share_pct, 1)} %`,
    sessions: fmtInt(g.sessions),
    perSeries: g.series > 0 ? fmtDec(g.sessions / g.series, 1) : "—",
  });
  const hasData = nv.new_series.series + nv.catalog.series > 0;
  const note = nv.history_from
    ? `Une série est une nouveauté quand elle n'avait aucune séance avant le ${dayLabel(nv.history_from)}, début de l'historique disponible : la part des nouveautés est surestimée tant que l'historique est court.`
    : "Aucun historique disponible : toutes les séries comptent comme des nouveautés.";
  return {
    segments: [
      { label: "Nouveautés", value: nv.new_series.sessions, tone: "accent" },
      { label: "Fonds de catalogue", value: nv.catalog.sessions, tone: "light" },
    ],
    cards: [card("Nouveautés", "accent", nv.new_series), card("Fonds de catalogue", "light", nv.catalog)],
    note,
    hasData,
  };
}

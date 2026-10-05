// Pure logic of the Business page: measured baseline, 12-month projection simulator, saturation date,
// constats and local persistence of the assumptions. No runtime import (type-only), so `node --test` runs it as is.
//
// Rules of the model:
// - an empty assumption is `null`, never 0: everything that depends on it is `null` too (shown "[À MESURER]");
// - no division by zero: every ratio goes through `safeDiv`, which answers `null` instead of Infinity/NaN;
// - amounts are in "the unit of the configured prices": no currency is assumed.
import type { AdminCosts, AdminOverview } from "@/lib/admin/types";

export const MISSING_MEASURE = "[À MESURER]";
export const MISSING_FILL = "[À RENSEIGNER]";
export const TARIFF_PLACEHOLDER = "[TARIF]";
export const HORIZON = 12;
export const MILESTONE_MONTHS: readonly number[] = [0, 3, 6, 9, 12];
export const STORAGE_KEY = "gazes-admin-business-v1";

// Growth guard rails: a measure on a tiny sample is not a trend, and no simulated value may explode.
export const MIN_BASE_USERS = 50;
export const MIN_PERIOD_DAYS = 14;
export const GROWTH_MIN = -50;
export const GROWTH_MAX = 100;
export const MAX_RENDERED = 1e9;
export const GROWTH_SAMPLE_NOTE = "Échantillon trop faible pour estimer une croissance, saisissez une hypothèse.";

/** Monthly growth (% / month) brought back to [-50, +100]. Non-finite input gives null. */
export function clampMonthlyGrowth(n: number | null | undefined): number | null {
  if (!isNum(n)) return null;
  return Math.min(GROWTH_MAX, Math.max(GROWTH_MIN, n));
}

/** Short visible message when a typed growth was brought back to the bounds, null when it was in range. */
export function growthClampNotice(n: number | null | undefined): string | null {
  if (!isNum(n)) return null;
  if (n > GROWTH_MAX) return `ramené à +${GROWTH_MAX} %`;
  if (n < GROWTH_MIN) return `ramené à −${Math.abs(GROWTH_MIN)} %`;
  return null;
}

/** A value to render: finite and not above 1e9 (in absolute value), otherwise null ("[À MESURER]"). */
export function renderable(n: number | null | undefined): number | null {
  return isNum(n) && Math.abs(n) <= MAX_RENDERED ? n : null;
}

// ---------- number helpers ----------

export function isNum(n: unknown): n is number {
  return typeof n === "number" && Number.isFinite(n);
}

/** a / b, or null when one side is missing or b is 0. */
export function safeDiv(a: number | null | undefined, b: number | null | undefined): number | null {
  if (!isNum(a) || !isNum(b) || b === 0) return null;
  const r = a / b;
  return isNum(r) ? r : null;
}

/** a * b, or null when one side is missing. */
export function mul(a: number | null | undefined, b: number | null | undefined): number | null {
  if (!isNum(a) || !isNum(b)) return null;
  const r = a * b;
  return isNum(r) ? r : null;
}

export function fmtInt(n: number): string {
  return Math.round(n).toLocaleString("fr-FR");
}

export function fmtDec(n: number, decimals = 1): string {
  return n.toLocaleString("fr-FR", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

/** Integer text, or the placeholder when the value is missing. */
export function intOr(n: number | null, placeholder = MISSING_MEASURE): string {
  return !isNum(n) ? placeholder : fmtInt(n);
}

export function decOr(n: number | null, decimals: number, placeholder = MISSING_MEASURE): string {
  return !isNum(n) ? placeholder : fmtDec(n, decimals);
}

// ---------- assumptions ----------

export type AssumptionKey = "growth" | "share" | "hoursPerActive" | "gbPerHour" | "costPerGb" | "streamLimit" | "tariff" | "conversion";
export type Assumptions = Record<AssumptionKey, number | null>;

export interface AssumptionDef {
  key: AssumptionKey;
  label: string;
  unit: string;
  hint: string;
  step: number;
  /** Smallest value reachable with the "-" button; below it a nullable assumption becomes empty. */
  min: number;
  max: number;
  decimals: number;
  /** Value given by the first "+" on an empty assumption. */
  start: number;
  /** Shown while empty. */
  placeholder: string;
  /** "-" below `min` empties the field. */
  nullable: boolean;
}

export const ASSUMPTION_DEFS: readonly AssumptionDef[] = [
  { key: "growth", label: "Croissance mensuelle des inscrits", unit: "% / mois", hint: "Mesurée : nouveaux inscrits sur la période, rapportés aux inscrits d'avant, ramenés à 30 jours. Proposée seulement avec au moins 50 inscrits avant la période et 14 jours. Bornée à −50 / +100 %. Prudent ×0,5, ambitieux ×1,5.", step: 0.5, min: 0, max: 100, decimals: 1, start: 5, placeholder: MISSING_MEASURE, nullable: false },
  { key: "share", label: "Part d'inscrits actifs", unit: "%", hint: "Utilisateurs actifs sur la période, rapportés au total des inscrits.", step: 1, min: 0, max: 100, decimals: 0, start: 30, placeholder: MISSING_MEASURE, nullable: false },
  { key: "hoursPerActive", label: "Heures par actif et par mois", unit: "h", hint: "Heures regardées ramenées à 30 jours, divisées par les actifs.", step: 0.1, min: 0, max: 200, decimals: 2, start: 5, placeholder: MISSING_MEASURE, nullable: false },
  { key: "gbPerHour", label: "Débit moyen par heure regardée", unit: "Go / h", hint: "Non mesuré côté serveur. Vide : le coût de bande passante mesuré par heure est utilisé.", step: 0.1, min: 0.1, max: 20, decimals: 1, start: 1, placeholder: MISSING_MEASURE, nullable: true },
  { key: "costPerGb", label: "Coût par Go", unit: "unité / Go", hint: "Selon l'hébergeur. Vide : le coût de bande passante mesuré par heure est utilisé.", step: 0.0005, min: 0.0005, max: 1, decimals: 4, start: 0.001, placeholder: MISSING_FILL, nullable: true },
  { key: "streamLimit", label: "Limite de flux simultanés", unit: "flux", hint: "Non mesurée : valeur à fixer par un test de charge. Vide : pas de jauge ni de date de saturation.", step: 10, min: 1, max: 100000, decimals: 0, start: 50, placeholder: MISSING_MEASURE, nullable: true },
  { key: "tariff", label: "Tarif de l'offre de confort", unit: "unité / mois", hint: "Non décidé. Vide tant qu'aucun tarif n'est choisi.", step: 0.5, min: 0.5, max: 1000, decimals: 2, start: 3, placeholder: TARIFF_PLACEHOLDER, nullable: true },
  { key: "conversion", label: "Taux de conversion", unit: "% des actifs", hint: "Aucune offre en ligne : rien à mesurer aujourd'hui.", step: 0.5, min: 0.5, max: 100, decimals: 1, start: 2, placeholder: MISSING_MEASURE, nullable: true },
];

export function assumptionDef(key: AssumptionKey): AssumptionDef {
  const def = ASSUMPTION_DEFS.find((d) => d.key === key);
  if (!def) throw new Error(`unknown assumption ${key}`);
  return def;
}

export function emptyAssumptions(): Assumptions {
  return { growth: null, share: null, hoursPerActive: null, gbPerHour: null, costPerGb: null, streamLimit: null, tariff: null, conversion: null };
}

function round6(n: number): number {
  return Number(n.toFixed(6));
}

/** One "-" (dir -1) or "+" (dir 1) click. Empty + "-" stays empty; empty + "+" starts at `start`. */
export function stepAssumption(def: AssumptionDef, current: number | null, dir: 1 | -1): number | null {
  if (current === null) return dir > 0 ? def.start : null;
  const next = round6(current + dir * def.step);
  if (next < def.min) return def.nullable ? null : def.min;
  return Math.min(def.max, next);
}

/**
 * Typed text -> value. "" is an empty assumption (null); a French or English decimal is a number;
 * anything else (letters, negative) is invalid (undefined) and the caller keeps the previous value.
 */
export function parseAssumptionInput(text: string): number | null | undefined {
  // A trailing separator ("8,") is a number being typed, not an error.
  const t = text.replace(/[\s  ]/g, "").replace(/[.,]$/, "").replace(",", ".");
  if (t === "") return null;
  if (!/^\d+(\.\d+)?$/.test(t)) return undefined;
  const n = Number(t);
  return isNum(n) ? n : undefined;
}

export function clampAssumption(def: AssumptionDef, n: number): number {
  const v = Math.min(def.max, Math.max(0, n));
  if (def.key === "growth") return clampMonthlyGrowth(v) ?? 0;
  return v;
}

/** Text shown in the field. */
export function assumptionText(def: AssumptionDef, value: number | null): string {
  return value === null ? def.placeholder : fmtDec(value, def.decimals);
}

/** Text to edit (no thousands separator, comma decimal). */
export function assumptionEditText(def: AssumptionDef, value: number | null): string {
  if (value === null) return "";
  return Number(value.toFixed(def.decimals)).toString().replace(".", ",");
}

// ---------- measured baseline ----------

export interface Baseline {
  /** Registered users today. */
  users: number | null;
  /** Estimated peak of simultaneous sessions over the period. */
  peak: number | null;
  peakEstimated: boolean;
  peakTruncated: boolean;
  /** Server cost per month: period value / server_prorata. */
  serverMonthly: number | null;
  /** Storage cost per month (null as long as the stored volume is not measured). */
  storageMonthly: number | null;
  /** Measured bandwidth cost per watched hour. */
  bandwidthPerHour: number | null;
  /** Watched hours over 30 days. */
  hoursPerMonth: number | null;
  /** Share of the monthly bill covered by the period (days / 30). */
  prorata: number | null;
}

export interface MeasuredFacts {
  baseline: Baseline;
  /** Initial assumptions: measured when they exist, empty otherwise. */
  defaults: Assumptions;
  /** Monthly growth of registrations from the measurement (% / 30 days), null when it cannot be computed. */
  measuredGrowth: number | null;
  /** Why no growth is proposed (sample too small), null otherwise. */
  growthNote: string | null;
  /** Raw counts behind the growth, for the hint. */
  newUsers: number | null;
  totalUsers: number | null;
}

/**
 * Baseline and initial assumptions from /costs and /overview.
 * Monthly server cost = value / server_prorata; measured growth = new / (total - new) brought back to 30 days.
 */
export function buildBaseline(costs: AdminCosts, overview: AdminOverview, periodDays: number): MeasuredFacts {
  const c = costs.costs;
  const u = costs.usage;
  const prorata = isNum(c.server_prorata) && c.server_prorata > 0 ? c.server_prorata : isNum(periodDays) && periodDays > 0 ? periodDays / 30 : null;
  const watch = u.watch_hours.value;
  const totalUsers = isNum(overview.kpis.total_users.value) ? overview.kpis.total_users.value : null;
  const newUsers = isNum(overview.kpis.new_users.value) ? overview.kpis.new_users.value : null;
  const activeUsers = isNum(u.active_users.value) ? u.active_users.value : null;

  const serverMonthly = c.server.measured && isNum(c.server.value) ? safeDiv(c.server.value, prorata) : null;
  const storageMonthly = c.storage.measured && isNum(c.storage.value) ? c.storage.value : null;
  const bandwidthPerHour = c.bandwidth.measured && isNum(c.bandwidth.value) ? safeDiv(c.bandwidth.value, watch) : null;
  const hoursPerMonth = isNum(periodDays) && periodDays > 0 && isNum(watch) ? (watch * 30) / periodDays : null;

  // Registrations: ratio of new users to the users who were there before the period, scaled linearly to 30 days.
  const before = totalUsers !== null && newUsers !== null ? totalUsers - newUsers : null;
  const ratio = safeDiv(newUsers, before);
  // Only proposed when the starting base is large enough and the period long enough to mean something.
  const enough = before !== null && before >= MIN_BASE_USERS && isNum(periodDays) && periodDays >= MIN_PERIOD_DAYS;
  const measuredGrowth =
    enough && ratio !== null && ratio >= 0 ? clampMonthlyGrowth(ratio * (30 / periodDays) * 100) : null;

  const activeShare = safeDiv(activeUsers, totalUsers);
  const share = activeShare === null ? null : activeShare * 100;
  const hoursPerActive = safeDiv(hoursPerMonth, activeUsers);

  const peak = isNum(u.peak_concurrent_sessions.value) ? u.peak_concurrent_sessions.value : null;

  return {
    baseline: {
      users: totalUsers,
      peak,
      peakEstimated: u.peak_concurrent_sessions.estimated,
      peakTruncated: u.peak_concurrent_sessions.truncated,
      serverMonthly,
      storageMonthly,
      bandwidthPerHour,
      hoursPerMonth,
      prorata,
    },
    defaults: {
      ...emptyAssumptions(),
      growth: measuredGrowth,
      share,
      hoursPerActive,
    },
    measuredGrowth,
    growthNote: measuredGrowth === null ? GROWTH_SAMPLE_NOTE : null,
    newUsers,
    totalUsers,
  };
}

// ---------- scenarios and projection ----------

export type ScenarioId = "prudent" | "tendance" | "ambitieux";

export interface ScenarioDef {
  id: ScenarioId;
  label: string;
  mult: number;
  note: string;
}

export const SCENARIOS: readonly ScenarioDef[] = [
  { id: "prudent", label: "Prudent", mult: 0.5, note: "croissance ÷ 2" },
  { id: "tendance", label: "Tendance", mult: 1, note: "croissance mesurée" },
  { id: "ambitieux", label: "Ambitieux", mult: 1.5, note: "croissance × 1,5" },
];

/**
 * Monthly growth (%) of a scenario: the typed or pre-filled growth is bounded first, then the multiplier applies,
 * then the result is bounded again. With a decline the multiplier is inverted so "prudent" is always the lowest.
 */
export function scenarioGrowth(growth: number | null, id: ScenarioId): number | null {
  const base = clampMonthlyGrowth(growth);
  if (base === null) return null;
  const mult = scenarioDef(id).mult;
  return clampMonthlyGrowth(base >= 0 ? base * mult : base / mult);
}

export function scenarioDef(id: ScenarioId): ScenarioDef {
  return SCENARIOS.find((s) => s.id === id) ?? SCENARIOS[1];
}

export function isScenarioId(v: unknown): v is ScenarioId {
  return v === "prudent" || v === "tendance" || v === "ambitieux";
}

export interface MonthRow {
  month: number;
  users: number | null;
  actives: number | null;
  hours: number | null;
  /** Estimated peak of simultaneous sessions: today's peak scaled by the watched hours. */
  peak: number | null;
  /** Machines needed for that peak; null without a limit. */
  servers: number | null;
  serverCost: number | null;
  bandwidthCost: number | null;
  storageCost: number | null;
  /** Sum of the known cost lines; null when none is known. */
  cost: number | null;
  /** At least one cost line is unknown, so `cost` is a lower bound. */
  costPartial: boolean;
  costPerHour: number | null;
  costPerActive: number | null;
  revenue: number | null;
}

/** Bandwidth cost per watched hour: typed GB/h x cost/GB when both are filled, otherwise the measured one. */
export function bandwidthPerHour(base: Baseline, a: Assumptions): number | null {
  const typed = mul(a.gbPerHour, a.costPerGb);
  return typed !== null ? typed : base.bandwidthPerHour;
}

/** The 13 points (month 0 = today .. month 12) of one scenario. */
export function projectScenario(base: Baseline, a: Assumptions, scenario: ScenarioId): MonthRow[] {
  const pct = scenarioGrowth(a.growth, scenario);
  const growth = pct === null ? null : pct / 100;
  const perHour = bandwidthPerHour(base, a);
  const limit = a.streamLimit !== null && a.streamLimit > 0 ? a.streamLimit : null;

  const rows: MonthRow[] = [];
  let hours0: number | null = null;
  for (let m = 0; m <= HORIZON; m++) {
    let users: number | null = null;
    if (base.users !== null) {
      if (m === 0) users = base.users;
      else if (growth !== null) users = renderable(base.users * Math.pow(1 + growth, m));
    }
    const actives = a.share === null || users === null ? null : renderable((users * a.share) / 100);
    const hours = renderable(mul(actives, a.hoursPerActive));
    if (m === 0) hours0 = hours;
    const peak = renderable(mul(base.peak, safeDiv(hours, hours0)));
    const servers = peak !== null && limit !== null ? Math.max(1, Math.ceil(peak / limit - 1e-9)) : null;
    const serverCost = base.serverMonthly === null ? null : renderable(base.serverMonthly * (servers ?? 1));
    const bandwidthCost = renderable(mul(hours, perHour));
    const storageCost = base.storageMonthly;
    const known = [serverCost, bandwidthCost, storageCost].filter(isNum);
    const cost = known.length > 0 ? renderable(known.reduce((x, y) => x + y, 0)) : null;
    const revenue = actives === null || a.tariff === null || a.conversion === null ? null : renderable((actives * a.conversion * a.tariff) / 100);
    rows.push({
      month: m,
      users,
      actives,
      hours,
      peak,
      servers,
      serverCost,
      bandwidthCost,
      storageCost,
      cost,
      costPartial: known.length < 3,
      costPerHour: renderable(safeDiv(cost, hours)),
      costPerActive: renderable(safeDiv(cost, actives)),
      revenue,
    });
  }
  return rows;
}

export function projectAll(base: Baseline, a: Assumptions): Record<ScenarioId, MonthRow[]> {
  return {
    prudent: projectScenario(base, a, "prudent"),
    tendance: projectScenario(base, a, "tendance"),
    ambitieux: projectScenario(base, a, "ambitieux"),
  };
}

/** The rows of the milestones (default 0, 3, 6, 9, 12 months). */
export function milestones(rows: readonly MonthRow[], months: readonly number[] = MILESTONE_MONTHS): MonthRow[] {
  return months.map((m) => rows[m]).filter((r): r is MonthRow => r !== undefined);
}

/** The column of a row set as numbers when every month is known, otherwise null (the chart is not drawn). */
export function seriesOf(rows: readonly MonthRow[], key: "users" | "hours" | "cost" | "revenue" | "peak"): number[] | null {
  const out: number[] = [];
  for (const r of rows) {
    const v = r[key];
    if (!isNum(v)) return null;
    out.push(v);
  }
  return out;
}

/** Labels of the assumptions to fill to compute a column. */
export function missingFor(a: Assumptions, needs: readonly AssumptionKey[]): string[] {
  return needs.filter((k) => a[k] === null).map((k) => assumptionDef(k).label);
}

export const USERS_NEEDS: readonly AssumptionKey[] = ["growth"];
export const HOURS_NEEDS: readonly AssumptionKey[] = ["growth", "share", "hoursPerActive"];

// ---------- saturation ----------

export interface Saturation {
  /** unknown: no limit or no peak; already: peak >= limit today; at: crossed within 12 months; none: not within 12 months. */
  status: "unknown" | "already" | "at" | "none";
  /** Fractional months from today (0 when already reached). */
  months: number | null;
}

export function saturation(rows: readonly MonthRow[], limit: number | null): Saturation {
  if (limit === null || !(limit > 0) || rows.length === 0) return { status: "unknown", months: null };
  const first = rows[0].peak;
  if (first === null) return { status: "unknown", months: null };
  if (first >= limit) return { status: "already", months: 0 };
  for (let m = 1; m < rows.length; m++) {
    const prev = rows[m - 1].peak;
    const cur = rows[m].peak;
    if (prev === null || cur === null) return { status: "unknown", months: null };
    if (cur >= limit) {
      const frac = m - 1 + (limit - prev) / (cur - prev);
      return { status: "at", months: Math.min(m, Math.max(m - 1, frac)) };
    }
  }
  return { status: "none", months: null };
}

// ---------- month labels ----------

const MONTHS_SHORT = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."];
const MONTHS_LONG = ["janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"];

/** "oct. 2026" / "octobre 2026" for `offset` months after the month of `isoDate`; "M+n" if the date is unreadable. */
export function monthLabel(isoDate: string, offset: number, long = false): string {
  const match = /^(\d{4})-(\d{2})/.exec(isoDate);
  if (!match) return offset === 0 ? "aujourd'hui" : `M+${offset}`;
  const index = Number(match[1]) * 12 + (Number(match[2]) - 1) + offset;
  const year = Math.floor(index / 12);
  const names = long ? MONTHS_LONG : MONTHS_SHORT;
  return `${names[index % 12]} ${year}`;
}

export function monthLabels(isoDate: string): string[] {
  return Array.from({ length: HORIZON + 1 }, (_, m) => monthLabel(isoDate, m));
}

export interface SaturationText {
  date: string;
  detail: string;
}

export function saturationText(s: Saturation, isoDate: string): SaturationText {
  switch (s.status) {
    case "unknown":
      return { date: MISSING_MEASURE, detail: "limite de flux non renseignée" };
    case "already":
      return { date: "Déjà atteinte", detail: "au-dessus de la limite" };
    case "none":
      return { date: `Après ${monthLabel(isoDate, HORIZON)}`, detail: "au-delà de 12 mois" };
    case "at": {
      const months = s.months ?? 0;
      return { date: monthLabel(isoDate, Math.min(HORIZON, Math.round(months)), true), detail: `dans environ ${fmtDec(months, 1)} mois` };
    }
  }
}

// ---------- measured cost lines ----------

export interface CostLine {
  id: string;
  label: string;
  detail: string;
  /** Per month, null when not measured. */
  value: number | null;
  tag: string;
  /** Share of the known total, 0-100. */
  bar: number;
}

export interface CostBreakdown {
  lines: CostLine[];
  /** Sum of the measured lines (monthly). */
  total: number | null;
  partial: boolean;
}

function missingDetail(inputs: string[] | undefined, fallback: string): string {
  return inputs && inputs.length > 0 ? `Entrées manquantes : ${inputs.join(", ")}` : fallback;
}

/** Monthly infrastructure bill from /costs. Lines the API does not measure stay [À RENSEIGNER]. */
export function costBreakdown(costs: AdminCosts, base: Baseline): CostBreakdown {
  const c = costs.costs;
  const prorata = base.prorata;
  const bandwidthMonthly = c.bandwidth.measured && isNum(c.bandwidth.value) ? safeDiv(c.bandwidth.value, prorata) : null;
  const raw: Array<Omit<CostLine, "bar">> = [
    {
      id: "server",
      label: "Serveur dédié",
      detail: base.serverMonthly === null ? missingDetail(c.server.missing_inputs, "Prix mensuel du serveur non configuré") : `Prix de la période ramené au mois (prorata ${fmtDec(prorata ?? 0, 2)})`,
      value: base.serverMonthly,
      tag: base.serverMonthly === null ? MISSING_FILL : "mesuré",
    },
    {
      id: "bandwidth",
      label: "Bande passante",
      detail: bandwidthMonthly === null ? missingDetail(c.bandwidth.missing_inputs, "Coût par Go ou débit par heure non configuré") : `${intOr(base.hoursPerMonth)} h par mois au coût de bande passante mesuré`,
      value: bandwidthMonthly,
      tag: bandwidthMonthly === null ? MISSING_FILL : "mesuré",
    },
    {
      id: "storage",
      label: "Stockage",
      detail: base.storageMonthly === null ? missingDetail(c.storage.missing_inputs, "Volume stocké non mesuré") : "Coût mensuel configuré",
      value: base.storageMonthly,
      tag: base.storageMonthly === null ? MISSING_FILL : "mesuré",
    },
    { id: "domain", label: "Nom de domaine", detail: "Pas de mesure dans l'API : non chiffré", value: null, tag: MISSING_FILL },
    { id: "backup", label: "Sauvegardes", detail: "Pas de mesure dans l'API : non chiffré", value: null, tag: MISSING_FILL },
    { id: "network", label: "Protection réseau (CDN, anti-DDoS)", detail: "Pas de mesure dans l'API : non chiffré", value: null, tag: MISSING_FILL },
  ];
  const known = raw.map((l) => l.value).filter(isNum);
  const total = known.length > 0 ? known.reduce((x, y) => x + y, 0) : null;
  const lines = raw.map((l) => ({
    ...l,
    bar: l.value === null || total === null || total <= 0 ? 0 : Math.max(2, Math.round((l.value / total) * 100)),
  }));
  return { lines, total, partial: lines.some((l) => l.value === null) };
}

// ---------- constats ("À regarder") ----------

export interface Finding {
  id: string;
  level: "haute" | "moyenne" | "basse";
  title: string;
  text: string;
}

export interface FindingsInput {
  breakdown: CostBreakdown;
  assumptions: Assumptions;
  scenario: ScenarioId;
  rows: readonly MonthRow[];
  saturation: Saturation;
  isoDate: string;
  peak: number | null;
}

/** Up to three actionable constats, most urgent first. Only states what the data and the typed hypotheses say. */
export function findings(input: FindingsInput): Finding[] {
  const { breakdown, assumptions: a, rows, saturation: sat } = input;
  const scenario = scenarioDef(input.scenario).label.toLowerCase();
  const out: Finding[] = [];

  if (sat.status === "unknown") {
    out.push({
      id: "limite",
      level: "moyenne",
      title: "La limite de flux simultanés n'est pas mesurée : aucune saturation ne peut être projetée.",
      text: "Faire un test de charge, puis renseigner la limite dans le modèle d'hypothèses.",
    });
  } else if (sat.status === "already") {
    out.push({
      id: "saturation",
      level: "haute",
      title: `Le pic de ${intOr(input.peak)} flux atteint déjà la limite de ${intOr(a.streamLimit)} flux renseignée.`,
      text: "Vérifier la limite ou préparer une seconde machine.",
    });
  } else if (sat.status === "at") {
    const t = saturationText(sat, input.isoDate);
    out.push({
      id: "saturation",
      level: (sat.months ?? 99) <= 6 ? "haute" : "moyenne",
      title: `Saturation projetée en ${t.date} (${t.detail}, scénario ${scenario}).`,
      text: "Planifier une seconde machine ou alléger la charge de remux avant cette date.",
    });
  } else {
    out.push({
      id: "saturation",
      level: "basse",
      title: `Aucune saturation projetée sur 12 mois (scénario ${scenario}) pour une limite de ${intOr(a.streamLimit)} flux.`,
      text: "La limite est une valeur saisie : la valider par un test de charge.",
    });
  }

  const missing = breakdown.lines.filter((l) => l.value === null).length;
  if (breakdown.partial) {
    out.push({
      id: "couts-partiels",
      level: "moyenne",
      title: `Le coût mensuel est partiel : ${missing} ligne${missing > 1 ? "s" : ""} sur ${breakdown.lines.length} non chiffrée${missing > 1 ? "s" : ""}.`,
      text: "Renseigner les prix manquants (voir les entrées indiquées sur chaque ligne) pour obtenir un coût complet.",
    });
  }

  if (a.tariff === null || a.conversion === null) {
    out.push({
      id: "revenu",
      level: "moyenne",
      title: `Aucun revenu modélisable : le tarif ${TARIFF_PLACEHOLDER} et le taux de conversion ${MISSING_MEASURE} ne sont pas tous deux renseignés.`,
      text: "Mesurer l'intention d'achat avant de fixer un tarif.",
    });
  } else {
    const last = rows[rows.length - 1];
    out.push({
      id: "revenu",
      level: "basse",
      title: last && last.revenue !== null ? `Revenu potentiel à 12 mois : ${fmtInt(last.revenue)} par mois (hypothèses saisies, scénario ${scenario}).` : "Revenu potentiel non calculable : des hypothèses manquent.",
      text: "Traiter ce chiffre comme une hypothèse tant que la conversion n'est pas mesurée.",
    });
  }

  const order = { haute: 0, moyenne: 1, basse: 2 } as const;
  return out.sort((x, y) => order[x.level] - order[y.level]).slice(0, 3);
}

// ---------- local persistence ----------

export interface StoredState {
  scenario: ScenarioId;
  /** Only the assumptions the user touched (null = emptied on purpose). */
  edited: Partial<Assumptions>;
}

export function serializeState(state: StoredState): string {
  return JSON.stringify(state);
}

/** Tolerant reader: anything unreadable gives null, unknown keys and non-numbers are dropped. */
export function parseStoredState(raw: string | null | undefined): StoredState | null {
  if (!raw) return null;
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof data !== "object" || data === null) return null;
  const obj = data as { scenario?: unknown; edited?: unknown };
  const scenario: ScenarioId = isScenarioId(obj.scenario) ? obj.scenario : "tendance";
  const edited: Partial<Assumptions> = {};
  if (typeof obj.edited === "object" && obj.edited !== null) {
    const src = obj.edited as Record<string, unknown>;
    for (const def of ASSUMPTION_DEFS) {
      const v = src[def.key];
      if (v === null || (isNum(v) && v >= 0)) edited[def.key] = v;
    }
  }
  return { scenario, edited };
}

/** Measured defaults overridden by what the user typed. */
export function mergeAssumptions(defaults: Assumptions, edited: Partial<Assumptions>): Assumptions {
  const out = { ...defaults };
  for (const def of ASSUMPTION_DEFS) {
    const v = edited[def.key];
    if (v !== undefined) out[def.key] = v;
  }
  // A stored growth outside the bounds (old or edited storage) is brought back.
  if (out.growth !== null) out.growth = clampMonthlyGrowth(out.growth);
  return out;
}

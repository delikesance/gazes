// Pure logic for the admin card kit (no React, no imports: runs directly under node --test).

export type Tone = "accent" | "danger" | "muted";
export type SortDir = "asc" | "desc";
export type Row = Record<string, unknown>;

export type CellType =
  | "text"
  | "mono"
  | "chip"
  | "number"
  | "delta"
  | "bar"
  | "thumb"
  | "button"
  | "toggle"
  | "status";

export interface ColumnDef {
  key: string;
  label?: string;
  type?: CellType;
  align?: "left" | "right" | "center";
  width?: string;
  sortable?: boolean;
  decimals?: number;
  unit?: string;
  max?: number;
  tone?: Tone | "neutral";
  tones?: Record<string, string>;
  kind?: "chip" | "severity";
  goodWhen?: "up" | "down";
  mono?: boolean;
  muted?: boolean;
}

// ---------- number formatting (fr-FR) ----------

export function formatFr(n: number, decimals?: number | null): string {
  const opts: Intl.NumberFormatOptions =
    decimals === null || decimals === undefined
      ? { maximumFractionDigits: Number.isInteger(n) ? 0 : 1 }
      : { minimumFractionDigits: decimals, maximumFractionDigits: decimals };
  return n.toLocaleString("fr-FR", opts);
}

export const MISSING_VALUE = "[À MESURER]";

/** KpiCard main value: numbers are formatted fr-FR, strings kept, empty means "to measure". */
export function formatKpiValue(
  value: string | number | null | undefined,
  decimals?: number | null,
): { text: string; missing: boolean } {
  if (value === null || value === undefined || value === "") return { text: MISSING_VALUE, missing: true };
  if (typeof value === "number") return { text: formatFr(value, decimals), missing: false };
  return { text: String(value), missing: false };
}

/** Unit pluralisation used by Delta: plural form if given, else +s for plain >=4 letter words not ending in s/x/z. */
export function pluralizeUnit(unit: string, absValue: number, decimals: number, plural?: string): string {
  if (unit === "%") return unit;
  if (Number(absValue.toFixed(decimals)) < 2) return unit;
  if (plural) return plural;
  if (/^[A-Za-zÀ-ÿ]{4,}$/.test(unit) && !/[sxz]$/i.test(unit)) return unit + "s";
  return unit;
}

/** MediaRow ranked trend: "+12 %", "−3,5 %". */
export function formatTrend(trend: number): { text: string; up: boolean } {
  const up = trend >= 0;
  const text =
    (up ? "+" : "−") + Math.abs(trend).toFixed(Number.isInteger(trend) ? 0 : 1).replace(".", ",") + " %";
  return { text, up };
}

export function clampPercent(n: number): number {
  return Math.max(0, Math.min(100, n));
}

// ---------- DataTable ----------

/** Readable value of a cell for search and sort (objects: label/text/title/checked). */
export function plainValue(val: unknown): string | number | null | undefined {
  if (val && typeof val === "object") {
    const o = val as Record<string, unknown>;
    const v = o.label ?? o.text ?? o.title ?? (o.checked != null ? (o.checked ? 1 : 0) : "");
    return v as string | number;
  }
  if (typeof val === "boolean") return val ? 1 : 0;
  return val as string | number | null | undefined;
}

/** Lowercase, accent-insensitive text for search. */
export function normalizeText(s: unknown): string {
  return String(s == null ? "" : s)
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();
}

export function rowKeyOf(row: Row, index: number, rowKey: string): string {
  const v = row ? row[rowKey] : undefined;
  return String(v != null ? v : index);
}

export const ALL_TAB = "__all__";

export interface TabDef {
  label: string;
  value: string;
}

export function filterByTab(rows: Row[], tabKey: string, tab: string): Row[] {
  if (!tabKey || tab === ALL_TAB) return rows;
  return rows.filter((r) => String(r[tabKey]) === String(tab));
}

export function filterBySearch(rows: Row[], query: string, keys: string[]): Row[] {
  const q = normalizeText(query.trim());
  if (!q) return rows;
  return rows.filter((r) => keys.some((k) => normalizeText(plainValue(r[k])).indexOf(q) >= 0));
}

/** Per-tab row counts ("Tous" counts every row). */
export function tabCounts(rows: Row[], tabKey: string, tabs: TabDef[]): Record<string, number> {
  const out: Record<string, number> = { [ALL_TAB]: rows.length };
  for (const t of tabs) out[t.value] = rows.filter((r) => String(r[tabKey]) === String(t.value)).length;
  return out;
}

const ISO_RE = /^\d{4}-\d{2}-\d{2}/;

/** Sort value: row.<key>Sort (number or ISO date) overrides the displayed value. */
export function sortValueOf(row: Row, key: string): unknown {
  const sv = row[key + "Sort"];
  return sv !== undefined && sv !== null && sv !== "" ? sv : plainValue(row[key]);
}

/** Compare two non-empty values: numeric, ISO date, then french natural text. */
export function compareValues(va: unknown, vb: unknown): number {
  if (typeof va === "number" && typeof vb === "number") return va - vb;
  const sa = String(va);
  const sb = String(vb);
  if (ISO_RE.test(sa) && ISO_RE.test(sb) && Number.isFinite(Date.parse(sa)) && Number.isFinite(Date.parse(sb))) {
    return Date.parse(sa) - Date.parse(sb);
  }
  return sa.localeCompare(sb, "fr", { numeric: true });
}

/** Stable sort; empty values always last whatever the direction. Returns a new array. */
export function sortRows(rows: Row[], key: string, dir: SortDir): Row[] {
  const idx = new Map<Row, number>();
  rows.forEach((r, i) => idx.set(r, i));
  const d = dir === "desc" ? -1 : 1;
  const order = (r: Row) => idx.get(r) ?? 0;
  return [...rows].sort((a, b) => {
    const va = sortValueOf(a, key);
    const vb = sortValueOf(b, key);
    const ea = va == null || va === "";
    const eb = vb == null || vb === "";
    if (ea || eb) return ea && eb ? order(a) - order(b) : ea ? 1 : -1;
    const c = compareValues(va, vb);
    return c ? c * d : order(a) - order(b);
  });
}

/** Clicking a header: toggle direction on the active column, else numeric-like columns start desc. */
export function nextSort(
  current: { key: string; dir: SortDir },
  col: { key: string; type?: CellType },
): { key: string; dir: SortDir } {
  if (current.key === col.key) return { key: col.key, dir: current.dir === "asc" ? "desc" : "asc" };
  const numeric = col.type === "number" || col.type === "delta" || col.type === "bar";
  return { key: col.key, dir: numeric ? "desc" : "asc" };
}

export function ariaSortOf(active: boolean, dir: SortDir): "ascending" | "descending" | "none" {
  return active ? (dir === "asc" ? "ascending" : "descending") : "none";
}

export function columnAlign(c: ColumnDef): "left" | "right" | "center" {
  return c.align || (c.type === "number" || c.type === "delta" ? "right" : "left");
}

export function countText(shown: number, total: number): string {
  return shown === total ? total + (total > 1 ? " lignes" : " ligne") : shown + " sur " + total + " lignes";
}

export function formatNumberCell(v: unknown, c: { decimals?: number; unit?: string }): string {
  const n = Number(v);
  const dec = c.decimals == null ? (Number.isInteger(n) ? 0 : 1) : c.decimals;
  return formatFr(n, dec) + (c.unit ? " " + c.unit : "");
}

export interface DeltaCell {
  text: string;
  direction: "up" | "down" | "flat";
  tone: "good" | "bad" | "flat";
}

export function formatDeltaCell(v: unknown, c: { decimals?: number; unit?: string; goodWhen?: "up" | "down" }): DeltaCell {
  const n = Number(v);
  const dec = c.decimals == null ? 1 : c.decimals;
  const unit = c.unit == null ? "%" : c.unit;
  const good = (c.goodWhen || "up") === "up" ? n > 0 : n < 0;
  const text = (n > 0 ? "+" : n < 0 ? "−" : "") + formatFr(Math.abs(n), dec) + (unit ? " " + unit : "");
  return {
    text,
    direction: n > 0 ? "up" : n < 0 ? "down" : "flat",
    tone: n === 0 ? "flat" : good ? "good" : "bad",
  };
}

/** Bar column scale: explicit max if valid, else the largest value of the column (never 0). */
export function barMax(rows: Row[], c: { key: string; max?: number }): number {
  const m = Number(c.max);
  if (c.max != null && Number.isFinite(m) && m > 0) return m;
  return Math.max(1e-9, ...rows.map((r) => (Number.isFinite(Number(r?.[c.key])) ? Number(r[c.key]) : 0)));
}

export function barPercent(n: number, max: number): number {
  return clampPercent((n / max) * 100);
}

/** Id of the search field: unique per instance. */
export function searchFieldId(uid: string, id?: string): string {
  const base = id ? "dt-" + String(id).replace(/[^A-Za-z0-9_-]+/g, "-") : "dt" + uid.replace(/[^A-Za-z0-9_-]+/g, "");
  return base + "-recherche";
}

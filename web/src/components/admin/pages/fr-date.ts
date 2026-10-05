// Pure French formatting helpers shared by the Vue d'ensemble, Visionnages and Lecteur et flux pages.
// Days are ISO "YYYY-MM-DD" (UTC); everything is computed without the local time zone so that the
// server render and the browser agree.

export const MONTHS_FR = ["janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."] as const;

export const MISSING = "[À MESURER]";

export function fmtInt(n: number): string {
  return Math.round(n).toLocaleString("fr-FR");
}

export function fmtDec(n: number, decimals = 1): string {
  return n.toLocaleString("fr-FR", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

function parseDay(day: string): { y: number; m: number; d: number } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(day);
  if (!match) return null;
  return { y: Number(match[1]), m: Number(match[2]), d: Number(match[3]) };
}

/** "2026-03-05" -> "5 mars". */
export function dayLabel(day: string): string {
  const p = parseDay(day);
  return p ? `${p.d} ${MONTHS_FR[p.m - 1] ?? ""}`.trim() : day;
}

/** "2026-03-02" .. "2026-03-31" -> "Du 2 mars au 31 mars 2026". */
export function rangeLabel(from: string | undefined, to: string | undefined): string {
  if (!from || !to) return "";
  const end = parseDay(to);
  return `Du ${dayLabel(from)} au ${dayLabel(to)}${end ? ` ${end.y}` : ""}`;
}

/** "il y a 6 min" relative to `nowIso` (the response's generated_at, so the output is deterministic). */
export function relativeFr(iso: string, nowIso: string): string {
  const t = Date.parse(iso);
  const now = Date.parse(nowIso);
  if (!Number.isFinite(t) || !Number.isFinite(now)) return "";
  const seconds = Math.max(0, Math.round((now - t) / 1000));
  if (seconds < 60) return "à l'instant";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `il y a ${minutes} min`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `il y a ${hours} h`;
  const days = Math.round(hours / 24);
  return `il y a ${days} j`;
}

/** "2026-03-31T08:00:00Z" -> "31 mars 2026, 08:00 UTC". */
export function dateTimeFr(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const d = new Date(t);
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${d.getUTCDate()} ${MONTHS_FR[d.getUTCMonth()]} ${d.getUTCFullYear()}, ${hh}:${mm} UTC`;
}

/** Short form for tables: "31 mars, 08:00". */
export function dateTimeShortFr(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const d = new Date(t);
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${d.getUTCDate()} ${MONTHS_FR[d.getUTCMonth()]}, ${hh}:${mm}`;
}

export function sum(values: ReadonlyArray<number>): number {
  return values.reduce((a, b) => a + b, 0);
}

/** Capitalised first letter ("lun" -> "Lun"). */
export function cap(s: string): string {
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : s;
}

/** Axis top rounded up to a readable number (1.08 headroom), at least 1. */
export function niceTop(max: number): number {
  if (!(max > 0)) return 1;
  const raw = max * 1.08;
  const magnitude = Math.pow(10, Math.floor(Math.log10(raw)));
  return Math.ceil(raw / magnitude) * magnitude;
}

/** tz_offset (minutes east of UTC) -> "UTC+1", "UTC+5:30", "UTC−4", "UTC". */
export function tzLabel(offsetMinutes: number): string {
  if (offsetMinutes === 0) return "UTC";
  const sign = offsetMinutes > 0 ? "+" : "−";
  const abs = Math.abs(offsetMinutes);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  return `UTC${sign}${h}${m ? `:${String(m).padStart(2, "0")}` : ""}`;
}

/** Score of the "points" delta: value - previous, or null when there is no usable previous value. */
export function pointsDelta(value: number, previous: number): number | null {
  return previous > 0 ? value - previous : null;
}

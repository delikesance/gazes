import type { ScheduleEntry } from "@/types/api";

const DAY_MS = 24 * 60 * 60 * 1000;

/** Local midnight of the given date. */
export function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

export function addDays(date: Date, days: number): Date {
  // Calendar arithmetic (not +24h) so daylight-saving changes keep local midnights.
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + days);
}

/** Monday 00:00 (local) of the week containing `date`. */
export function startOfWeek(date: Date): Date {
  const day = startOfDay(date);
  return addDays(day, -((day.getDay() + 6) % 7));
}

export function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** `YYYY-MM-DD` in local time: the key used to group airings by day. */
export function dayKey(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export interface Range { start: Date; end: Date }

/** The seven days of the week containing `anchor`; `end` is exclusive. */
export function weekRange(anchor: Date): Range {
  const start = startOfWeek(anchor);
  return { start, end: addDays(start, 7) };
}

/** Whole weeks covering the month of `anchor` (what a month grid shows); `end` is exclusive. */
export function monthGridRange(anchor: Date): Range {
  const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
  const last = new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0);
  const start = startOfWeek(first);
  return { start, end: addDays(startOfWeek(last), 7) };
}

export function daysIn(range: Range): Date[] {
  const days: Date[] = [];
  for (let d = range.start; d < range.end; d = addDays(d, 1)) days.push(d);
  return days;
}

/** Unix-second bounds for the API (it treats `from` and `to` as exclusive). */
export function apiBounds(range: Range): { from: number; to: number } {
  return { from: Math.floor(range.start.getTime() / 1000) - 1, to: Math.floor(range.end.getTime() / 1000) };
}

export function groupByDay(entries: ScheduleEntry[]): Map<string, ScheduleEntry[]> {
  const groups = new Map<string, ScheduleEntry[]>();
  for (const entry of entries) {
    const key = dayKey(new Date(entry.airing_at * 1000));
    const list = groups.get(key);
    if (list) list.push(entry); else groups.set(key, [entry]);
  }
  for (const list of groups.values()) list.sort((a, b) => a.airing_at - b.airing_at);
  return groups;
}

export type AiringState = "aired" | "next" | "upcoming";

/**
 * Past airings are "aired"; the first upcoming airing of the current day is
 * "next"; everything else is "upcoming".
 */
export function airingStates(entries: ScheduleEntry[], now: Date): Map<ScheduleEntry, AiringState> {
  const nowSec = Math.floor(now.getTime() / 1000);
  const states = new Map<ScheduleEntry, AiringState>();
  let nextAssigned = false;
  for (const entry of [...entries].sort((a, b) => a.airing_at - b.airing_at)) {
    if (entry.airing_at <= nowSec) states.set(entry, "aired");
    else if (!nextAssigned && isSameDay(new Date(entry.airing_at * 1000), now)) { states.set(entry, "next"); nextAssigned = true; }
    else states.set(entry, "upcoming");
  }
  return states;
}

export { DAY_MS };

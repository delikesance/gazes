// Pure logic of TimelineCard (ported from TimelineCard.dc.html renderVals).

export type TimelineSeverity = 'haute' | 'moyenne' | 'basse';

export interface TimelineIncidentInput {
  /** ISO date (YYYY-MM-DD...). */
  date: string;
  title?: string;
  severity?: TimelineSeverity;
  /** Minutes (number or digit string) or free text. */
  duration?: number | string | null;
  cause?: string;
  time?: string;
}

export interface TimelineInput {
  title?: string;
  incidents: ReadonlyArray<TimelineIncidentInput>;
  days?: number | null;
  /** ISO date of the last strip day; defaults to the latest incident, else `today`. */
  endDate?: string | null;
  /** Fallback end date when there is no endDate nor dated incident (injected for purity). */
  today?: Date;
}

export const TIMELINE_SEVERITIES: Record<TimelineSeverity, { rank: number; label: string; color: string; height: number; shape: string; pillBg: string; pillFg: string }> = {
  haute: { rank: 3, label: 'Haute', color: '#f87171', height: 44, shape: 'M5 0L10 10H0Z', pillBg: '#7f1d1d', pillFg: '#fecaca' },
  moyenne: { rank: 2, label: 'Moyenne', color: '#9b8afb', height: 30, shape: 'M5 0L10 5L5 10L0 5Z', pillBg: '#17171a', pillFg: '#9b8afb' },
  basse: { rank: 1, label: 'Basse', color: '#a1a1aa', height: 18, shape: 'M5 0A5 5 0 1 1 4.99 0Z', pillBg: '#17171a', pillFg: '#a1a1aa' },
};

export interface TimelineStripCell {
  title: string;
  height: number;
  color: string;
}

export interface TimelineIncidentRow {
  date: string;
  meta: string;
  severity: TimelineSeverity;
  title: string;
  cause: string;
}

export interface TimelineLegendItem {
  severity: TimelineSeverity;
  label: string;
}

export interface TimelineModel {
  title: string;
  days: number;
  gap: number;
  strip: TimelineStripCell[];
  firstDay: string;
  lastDay: string;
  legend: TimelineLegendItem[];
  incidents: TimelineIncidentRow[];
  ariaLabel: string;
}

const MOIS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.'];
const DAY_MS = 86400000;

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

export function parseDay(s: unknown): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(String(s || ''));
  return m ? new Date(Date.UTC(+m[1], +m[2] - 1, +m[3])) : null;
}
export const dayKey = (d: Date): string => d.toISOString().slice(0, 10);
export const dayLabel = (d: Date): string => d.getUTCDate() + ' ' + MOIS[d.getUTCMonth()];

export function formatDuration(v: unknown): string {
  if (v == null || v === '') return '';
  if (typeof v === 'number' || /^\d+$/.test(String(v))) {
    const m = Number(v);
    return m < 60 ? m + ' min' : Math.floor(m / 60) + ' h ' + String(m % 60).padStart(2, '0');
  }
  return String(v);
}

export function computeTimeline(p: TimelineInput): TimelineModel {
  const n = Number(p.days);
  const days = Math.max(7, Math.min(120, Math.round(p.days == null || !Number.isFinite(n) ? 30 : n)));
  const raw = list(p.incidents).map((x) => ({
    date: x.date == null ? '' : String(x.date),
    title: x.title || '',
    severity: (x.severity && TIMELINE_SEVERITIES[x.severity] ? x.severity : 'basse') as TimelineSeverity,
    duration: x.duration,
    cause: x.cause || '',
    time: x.time || '',
    d: parseDay(x.date),
  }));
  const dated = raw.filter((x) => x.d !== null) as Array<(typeof raw)[number] & { d: Date }>;
  const today = p.today ?? new Date();
  const endD =
    parseDay(p.endDate) ||
    (dated.length
      ? new Date(Math.max(...dated.map((x) => x.d.getTime())))
      : new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate())));
  const startD = new Date(endD.getTime() - (days - 1) * DAY_MS);

  const byDay: Record<string, typeof raw> = {};
  raw.forEach((x) => {
    if (x.d) {
      const k = dayKey(x.d);
      (byDay[k] = byDay[k] || []).push(x);
    }
  });
  const strip: TimelineStripCell[] = Array.from({ length: days }, (_, i) => {
    const d = new Date(startD.getTime() + i * DAY_MS);
    const list = byDay[dayKey(d)] || [];
    if (!list.length) return { title: dayLabel(d) + ' : aucun incident', height: 6, color: '#2a2a2e' };
    const worst = list.reduce((a, b) => (TIMELINE_SEVERITIES[b.severity].rank > TIMELINE_SEVERITIES[a.severity].rank ? b : a));
    const s = TIMELINE_SEVERITIES[worst.severity];
    return {
      title: dayLabel(d) + ' : ' + list.length + (list.length > 1 ? ' incidents' : ' incident') + ', gravité ' + s.label.toLowerCase(),
      height: s.height,
      color: s.color,
    };
  });

  const inWin = raw.filter((x) => !x.d || (x.d >= startD && x.d <= endD));
  const sorted = inWin.slice().sort((a, b) => (b.d ? b.d.getTime() : 0) - (a.d ? a.d.getTime() : 0));
  const incidents: TimelineIncidentRow[] = sorted.map((x) => ({
    date: x.d ? dayLabel(x.d) + ' ' + x.d.getUTCFullYear() : x.date,
    meta: [x.time, formatDuration(x.duration)].filter(Boolean).join(' · '),
    severity: x.severity,
    title: x.title,
    cause: x.cause,
  }));

  const counts: Record<TimelineSeverity, number> = { haute: 0, moyenne: 0, basse: 0 };
  inWin.forEach((x) => {
    counts[x.severity]++;
  });
  const order: TimelineSeverity[] = ['haute', 'moyenne', 'basse'];
  const legend = order.map((k) => ({ severity: k, label: TIMELINE_SEVERITIES[k].label + ' : ' + counts[k] }));

  const title = p.title ?? 'Chronologie';
  const auto =
    title + ' : ' + inWin.length + (inWin.length > 1 ? ' incidents' : ' incident') + ' sur ' + days + ' jours, du ' + dayLabel(startD) + ' au ' + dayLabel(endD) +
    ' (' + counts.haute + ' de gravité haute, ' + counts.moyenne + ' moyenne, ' + counts.basse + ' basse).';

  return {
    title, days, gap: days > 60 ? 1 : days > 14 ? 3 : 6, strip,
    firstDay: dayLabel(startD), lastDay: dayLabel(endD), legend, incidents, ariaLabel: auto,
  };
}

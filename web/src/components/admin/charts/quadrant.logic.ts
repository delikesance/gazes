// Pure logic of QuadrantCard (ported from QuadrantCard.dc.html renderVals).

export type QuadrantKey = 'topLeft' | 'topRight' | 'bottomLeft' | 'bottomRight';

export interface QuadrantPointInput {
  label: string;
  x: number | null;
  y: number | null;
  size?: number | null;
}

export interface QuadrantInput {
  title?: string;
  points: ReadonlyArray<QuadrantPointInput>;
  xUnit?: string;
  yUnit?: string;
  xDecimals?: number | null;
  yDecimals?: number | null;
  xMin?: number | null;
  xMax?: number | null;
  yMin?: number | null;
  yMax?: number | null;
  xThreshold?: number | null;
  yThreshold?: number | null;
  quadrantNames?: Partial<Record<QuadrantKey, string>>;
  quadrantRules?: Partial<Record<QuadrantKey, string>>;
  ariaLabel?: string;
}

export const QUADRANT_KEYS: QuadrantKey[] = ['topLeft', 'topRight', 'bottomLeft', 'bottomRight'];
export const QUADRANT_DEFAULT_NAMES: Record<QuadrantKey, string> = {
  topLeft: 'À pousser',
  topRight: 'Valeurs sûres',
  bottomLeft: 'À retirer',
  bottomRight: 'À surveiller',
};
export const QUADRANT_TEXT_COLORS: Record<QuadrantKey, string> = { topLeft: '#fafafa', topRight: '#9b8afb', bottomLeft: '#a1a1aa', bottomRight: '#f87171' };
export const QUADRANT_DOT_COLORS: Record<QuadrantKey, string> = { topLeft: '#fafafa', topRight: '#9b8afb', bottomLeft: '#71717a', bottomRight: '#f87171' };

export interface QuadrantPointModel {
  name: string;
  tip: string;
  leftPct: number;
  bottomPct: number;
  diameter: number;
  color: string;
  flip: boolean;
  quadrant: QuadrantKey;
}

export interface QuadrantSummary {
  key: QuadrantKey;
  name: string;
  n: string;
  color: string;
  rule: string;
  titles: string;
}

export interface QuadrantModel {
  title: string;
  points: QuadrantPointModel[];
  quadrants: QuadrantSummary[];
  names: Record<QuadrantKey, string>;
  thrX: number;
  thrY: number;
  xTicks: string[];
  yTicks: string[];
  ariaLabel: string;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

function fr(v: number, d: number, u: string): string {
  return v.toLocaleString('fr-FR', { minimumFractionDigits: d, maximumFractionDigits: d }) + (u ? ' ' + u : '');
}

const clampPct = (v: number): number => Math.max(0, Math.min(100, v));
const mean = (a: ReadonlyArray<number>): number => (a.length ? a.reduce((s, v) => s + v, 0) / a.length : 0);

/** Quadrant of a point: x >= threshold is "right", y >= threshold is "top". */
export function quadrantOf(x: number, y: number, xThr: number, yThr: number): QuadrantKey {
  return x >= xThr ? (y >= yThr ? 'topRight' : 'bottomRight') : y >= yThr ? 'topLeft' : 'bottomLeft';
}

/** Five evenly spaced tick labels from min to min + span. */
export function linearTicks(min: number, span: number, decimals: number, unit: string): string[] {
  return [0, 1, 2, 3, 4].map((i) => fr(min + (span * i) / 4, decimals, unit));
}

export function computeQuadrant(p: QuadrantInput): QuadrantModel {
  const xd = Math.max(0, Math.round(toNum(p.xDecimals, 0)));
  const yd = Math.max(0, Math.round(toNum(p.yDecimals, 0)));
  const xu = p.xUnit || '';
  const yu = p.yUnit || '';

  const pts = list(p.points).map((d) => ({
    label: d.label == null ? '' : String(d.label),
    x: toNum(d.x, 0),
    y: toNum(d.y, 0),
    size: d.size == null ? null : toNum(d.size, 0),
  }));
  const xs = pts.map((d) => d.x);
  const ys = pts.map((d) => d.y);
  const lo = (a: number[]) => (a.length ? Math.min(...a) : 0);
  const hi = (a: number[]) => (a.length ? Math.max(...a) : 1);
  const pad = (a: number[]) => (hi(a) - lo(a) || 1) * 0.08;
  const xMin = p.xMin == null ? lo(xs) - pad(xs) : toNum(p.xMin, 0);
  const xMax = p.xMax == null ? hi(xs) + pad(xs) : toNum(p.xMax, 1);
  const yMin = p.yMin == null ? lo(ys) - pad(ys) : toNum(p.yMin, 0);
  const yMax = p.yMax == null ? hi(ys) + pad(ys) : toNum(p.yMax, 1);
  const xSpan = xMax - xMin || 1;
  const ySpan = yMax - yMin || 1;
  const xThr = p.xThreshold == null ? mean(xs) : toNum(p.xThreshold, 0);
  const yThr = p.yThreshold == null ? mean(ys) : toNum(p.yThreshold, 0);
  const thrX = clampPct(((xThr - xMin) / xSpan) * 100);
  const thrY = clampPct(((yThr - yMin) / ySpan) * 100);

  const names: Record<QuadrantKey, string> = { ...QUADRANT_DEFAULT_NAMES, ...(p.quadrantNames ?? {}) };
  const rules = p.quadrantRules ?? {};

  const sizes = pts.map((d) => d.size).filter((s): s is number => s != null);
  const sMax = sizes.length ? Math.max(...sizes) : 0;
  const points: QuadrantPointModel[] = pts.map((d) => {
    const q = quadrantOf(d.x, d.y, xThr, yThr);
    const l = ((d.x - xMin) / xSpan) * 100;
    const b = ((d.y - yMin) / ySpan) * 100;
    return {
      name: d.label,
      tip: d.label + ' : ' + fr(d.x, xd, xu) + ' en X, ' + fr(d.y, yd, yu) + ' en Y, ' + names[q],
      leftPct: clampPct(l),
      bottomPct: clampPct(b),
      diameter: d.size != null && sMax > 0 ? Math.round(8 + (d.size / sMax) * 12) : 12,
      color: QUADRANT_DOT_COLORS[q],
      flip: l > 80,
      quadrant: q,
    };
  });

  const quadrants: QuadrantSummary[] = QUADRANT_KEYS.map((k) => {
    const list = pts.filter((d) => quadrantOf(d.x, d.y, xThr, yThr) === k).map((d) => d.label);
    return {
      key: k,
      name: names[k],
      n: list.length + (list.length > 1 ? ' éléments' : ' élément'),
      color: QUADRANT_TEXT_COLORS[k],
      rule: rules[k] || '',
      titles: list.length ? list.join(', ') : 'Aucun',
    };
  });

  const title = p.title ?? 'Nuage de points';
  return {
    title, points, quadrants, names, thrX, thrY,
    xTicks: linearTicks(xMin, xSpan, xd, xu),
    yTicks: [4, 3, 2, 1, 0].map((i) => fr(yMin + (ySpan * i) / 4, yd, yu)),
    ariaLabel: p.ariaLabel || title + ' : ' + quadrants.map((z) => z.name + ' (' + z.titles + ')').join(' ; ') + '.',
  };
}

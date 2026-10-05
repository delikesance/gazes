// Pure logic of LineChartCard (ported from LineChartCard.dc.html renderVals).
// Kept free of imports and TS-only runtime syntax so `node --experimental-strip-types` can run it.

export type LineTone = 'accent' | 'muted' | 'light';
export type Nullable<T> = T | null | undefined;

export interface LineSeriesInput {
  label?: string;
  values: ReadonlyArray<Nullable<number | string>>;
  style?: 'solid' | 'dashed';
  tone?: LineTone;
}

export interface LineMarkInput {
  index: number;
  /** End of a range mark (inclusive). */
  to?: Nullable<number>;
  label?: string;
  detail?: string;
  cause?: string;
  tone?: 'danger' | 'neutral';
}

export interface LineChartInput {
  title?: string;
  series: ReadonlyArray<LineSeriesInput>;
  xLabels?: ReadonlyArray<string | number>;
  yMin?: Nullable<number>;
  yMax?: Nullable<number>;
  yTickCount?: Nullable<number>;
  unit?: string;
  decimals?: Nullable<number>;
  area?: boolean;
  legend?: boolean;
  xTickCount?: Nullable<number>;
  height?: Nullable<number>;
  marks?: ReadonlyArray<LineMarkInput>;
  ariaLabel?: string;
}

export const LINE_W = 800;
export const LINE_TONES: Record<LineTone, string> = { accent: '#9b8afb', muted: '#71717a', light: '#fafafa' };
const DEFAULT_TONES: LineTone[] = ['accent', 'light', 'muted', 'muted'];

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

export function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

export function fr(n: number, dec: number): string {
  return n.toLocaleString('fr-FR', { minimumFractionDigits: dec, maximumFractionDigits: dec });
}

/** Smallest "nice" step (1, 2, 2.5, 5 x 10^k) that fits `span` in at most `maxIntervals` intervals. */
export function pickStep(span: number, maxIntervals: number): number {
  const e0 = Math.floor(Math.log10(span / maxIntervals));
  for (let e = e0; e <= e0 + 2; e++) {
    for (const f of [1, 2, 2.5, 5]) {
      const st = f * Math.pow(10, e);
      if (Math.ceil(span / st - 1e-9) <= maxIntervals) return st;
    }
  }
  return span / maxIntervals;
}

export interface YAxis {
  yMin: number;
  top: number;
  step: number;
  intervals: number;
  decimals: number;
  /** Labels from top to bottom. */
  ticks: string[];
}

export interface YAxisInput {
  /** Every non-null data value. */
  data: ReadonlyArray<number>;
  yMin?: Nullable<number>;
  yMax?: Nullable<number>;
  yTickCount?: Nullable<number>;
  decimals?: Nullable<number>;
  unit?: string;
}

export function computeYAxis(input: YAxisInput): YAxis {
  const dMax = input.data.length ? Math.max(...input.data) : 1;
  const dMin = input.data.length ? Math.min(...input.data) : 0;
  const yMin = input.yMin == null || (input.yMin as unknown) === '' ? Math.min(0, dMin) : toNum(input.yMin, 0);
  const ticks = Math.max(2, Math.round(toNum(input.yTickCount, 4)));
  let step: number;
  let intervals: number;
  let top: number;
  const yMaxN = input.yMax == null || (input.yMax as unknown) === '' ? NaN : Number(input.yMax);
  if (Number.isFinite(yMaxN) && yMaxN > yMin) {
    top = yMaxN;
    intervals = ticks;
    step = (top - yMin) / ticks;
  } else {
    const span = dMax - yMin || 1;
    step = pickStep(span * 1.05, ticks + 1);
    intervals = Math.max(1, Math.ceil((span * 1.05) / step - 1e-9));
    top = yMin + intervals * step;
  }
  let dec = 0;
  while (dec < 3 && Math.abs(step * Math.pow(10, dec) - Math.round(step * Math.pow(10, dec))) > 1e-9) dec++;
  const decimals = input.decimals == null || (input.decimals as unknown) === '' ? dec : Math.max(0, Math.round(toNum(input.decimals, 0)));
  const unit = input.unit ? ' ' + input.unit : '';
  const labels: string[] = [];
  for (let k = intervals; k >= 0; k--) labels.push(fr(yMin + k * step, decimals) + unit);
  return { yMin, top, step, intervals, decimals, ticks: labels };
}

export function gridPath(intervals: number, height: number, width: number = LINE_W): string {
  return Array.from({ length: intervals + 1 }, (_, k) => 'M0 ' + ((k * height) / intervals).toFixed(1) + 'H' + width).join('');
}

export function xPos(i: number, n: number, width: number = LINE_W): number {
  return n > 1 ? (i / (n - 1)) * width : width / 2;
}

export function yPos(v: number, yMin: number, top: number, height: number): number {
  return height - ((v - yMin) / (top - yMin || 1)) * height;
}

/** SVG path of one series; null values break the line (gap). */
export function buildLinePath(
  values: ReadonlyArray<number | null>,
  n: number,
  yMin: number,
  top: number,
  height: number,
  width: number = LINE_W,
): string {
  let d = '';
  let pen = false;
  values.forEach((v, i) => {
    if (v == null) {
      pen = false;
      return;
    }
    d += (pen ? 'L' : 'M') + xPos(i, n, width).toFixed(1) + ' ' + yPos(v, yMin, top, height).toFixed(1) + ' ';
    pen = true;
  });
  return d.trim();
}

/** Closed area under the first series; '' when fewer than two points. */
export function buildAreaPath(
  values: ReadonlyArray<number | null>,
  n: number,
  yMin: number,
  top: number,
  height: number,
  width: number = LINE_W,
): string {
  const pts: Array<[number, number]> = [];
  values.forEach((v, i) => {
    if (v != null) pts.push([xPos(i, n, width), yPos(v, yMin, top, height)]);
  });
  if (pts.length < 2) return '';
  return (
    pts.map((q, i) => (i ? 'L' : 'M') + q[0].toFixed(1) + ' ' + q[1].toFixed(1)).join(' ') +
    ' L' + pts[pts.length - 1][0].toFixed(1) + ' ' + height +
    ' L' + pts[0][0].toFixed(1) + ' ' + height + ' Z'
  );
}

/** Evenly spread subset of the labels (first and last always kept). */
export function pickXTicks(labels: ReadonlyArray<string>, count: number): string[] {
  const xc = Math.max(2, Math.round(count));
  if (labels.length <= xc) return labels.slice();
  return Array.from({ length: xc }, (_, i) => labels[Math.round((i * (labels.length - 1)) / (xc - 1))]);
}

export interface LineSeriesModel {
  label: string;
  values: Array<number | null>;
  dashed: boolean;
  stroke: string;
  strokeWidth: number;
  dash: string;
  path: string;
}

export interface LineBand {
  leftPct: number;
  widthPct: number;
  danger: boolean;
}

export interface LineMarkModel {
  n: number;
  leftPct: number;
  topPct: number;
  label: string;
  range: string;
  detail: string;
  cause: string;
}

export interface LineChartModel {
  title: string;
  height: number;
  viewBox: string;
  yTicks: string[];
  xTicks: string[];
  gridPath: string;
  areaPath: string;
  areaFill: string;
  series: LineSeriesModel[];
  legend: boolean;
  bands: LineBand[];
  marks: LineMarkModel[];
  ariaLabel: string;
}

export function computeLineChart(p: LineChartInput): LineChartModel {
  const H = Math.max(80, toNum(p.height, 240));
  const raw = list(p.series).slice(0, 4);
  const series = raw.map((s, i) => {
    const values = list(s.values).map((v) =>
      v == null || v === '' || !Number.isFinite(Number(v)) ? null : Number(v),
    );
    const tone = s.tone && LINE_TONES[s.tone] ? s.tone : DEFAULT_TONES[i];
    return { label: s.label || 'Série ' + (i + 1), values, dashed: s.style === 'dashed', stroke: LINE_TONES[tone] };
  });
  const n = series.reduce((m, s) => Math.max(m, s.values.length), 0);
  const data: number[] = [];
  series.forEach((s) => s.values.forEach((v) => v != null && data.push(v)));
  const axis = computeYAxis({ data, yMin: p.yMin, yMax: p.yMax, yTickCount: p.yTickCount, decimals: p.decimals, unit: p.unit });
  const { yMin, top, decimals } = axis;

  const seriesModels: LineSeriesModel[] = series.map((s, i) => ({
    ...s,
    strokeWidth: i === 0 ? 2.4 : 2,
    dash: s.dashed ? '6 5' : 'none',
    path: buildLinePath(s.values, n, yMin, top, H),
  }));

  const areaPath = p.area && series[0] ? buildAreaPath(series[0].values, n, yMin, top, H) : '';
  const labels = p.xLabels ? list(p.xLabels).map(String) : [];
  const xTicks = pickXTicks(labels, toNum(p.xTickCount, 5));
  const legend = (p.legend === undefined ? series.length > 1 : p.legend) && series.length > 0;

  const bands: LineBand[] = [];
  const rawMarks = list(p.marks);
  const marks: LineMarkModel[] = series[0]
    ? rawMarks.map((m, i) => {
        const idx = Math.max(0, Math.min(n - 1, Math.round(toNum(m.index, 0))));
        let v = series[0].values[idx];
        if (v == null) v = yMin;
        const hasTo = m.to != null && (m.to as unknown) !== '' && Number.isFinite(Number(m.to));
        const to = hasTo ? Math.max(0, Math.min(n - 1, Math.round(Number(m.to)))) : idx;
        const a0 = Math.min(idx, to);
        const a1 = Math.max(idx, to);
        const isRange = hasTo && a1 > a0 && n > 1;
        if (isRange) {
          bands.push({ leftPct: (a0 / (n - 1)) * 100, widthPct: ((a1 - a0) / (n - 1)) * 100, danger: m.tone === 'danger' });
        }
        const range = isRange
          ? labels[a0] && labels[a1]
            ? 'Du ' + labels[a0] + ' au ' + labels[a1]
            : 'Points ' + (a0 + 1) + ' à ' + (a1 + 1)
          : '';
        return {
          n: i + 1,
          leftPct: n > 1 ? (idx / (n - 1)) * 100 : 50,
          topPct: (yPos(v, yMin, top, H) / H) * 100,
          label: m.label || '',
          range,
          detail: m.detail ? String(m.detail) : '',
          cause: m.cause ? 'Cause : ' + String(m.cause) : '',
        };
      })
    : [];

  const title = p.title ?? 'Courbe';
  const unit = p.unit ? ' ' + p.unit : '';
  const f = (v: number) => fr(v, decimals) + unit;
  const auto =
    title +
    ' : ' +
    series
      .map((s) => {
        const vs = s.values.filter((v): v is number => v != null);
        return vs.length
          ? s.label + ' de ' + f(vs[0]) + ' à ' + f(vs[vs.length - 1]) + ' (min ' + f(Math.min(...vs)) + ', max ' + f(Math.max(...vs)) + ')'
          : s.label + ' sans données';
      })
      .join(' ; ') +
    (labels.length ? ', du ' + labels[0] + ' au ' + labels[labels.length - 1] : '') +
    '.';

  return {
    title,
    height: H,
    viewBox: '0 0 ' + LINE_W + ' ' + H,
    yTicks: axis.ticks,
    xTicks,
    gridPath: gridPath(axis.intervals, H),
    areaPath,
    areaFill: areaPath ? series[0].stroke : 'none',
    series: seriesModels,
    legend,
    bands,
    marks,
    ariaLabel: p.ariaLabel || auto,
  };
}

// Pure logic of BarChartCard (ported from BarChartCard.dc.html renderVals).

export type BarLabelMode = 'auto' | 'ends' | 'each' | 'none';
export type BarShowValues = 'none' | 'max' | 'all';
export type BarHighlight = 'none' | 'max';

export interface BarChartInput {
  title?: string;
  /** null / non-numeric values are drawn as 0 (a 2px stub), like the design. */
  values: ReadonlyArray<number | string | null | undefined>;
  labels?: ReadonlyArray<string | number>;
  subLabels?: ReadonlyArray<string | number>;
  startLabel?: string;
  endLabel?: string;
  labelMode?: BarLabelMode;
  labelEvery?: number | null;
  gap?: number | null;
  height?: number | null;
  max?: number | null;
  highlight?: BarHighlight;
  highlightIndexes?: ReadonlyArray<number>;
  highlightLabel?: string;
  otherLabel?: string;
  showValues?: BarShowValues;
  unit?: string;
  decimals?: number | null;
  ariaLabel?: string;
}

export interface BarModel {
  value: number;
  /** Formatted value shown above the bar ('' when hidden). */
  val: string;
  height: number;
  highlighted: boolean;
  dimmed: boolean;
  label: string;
  every: string;
  sub: string;
  tip: string;
}

export interface BarChartModel {
  title: string;
  bars: BarModel[];
  height: number;
  gap: number;
  radius: number;
  each: boolean;
  everyRow: boolean;
  ends: boolean;
  startLabel: string;
  endLabel: string;
  legend: boolean;
  highlightLabel: string;
  otherLabel: string;
  ariaLabel: string;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

export function computeBarChart(p: BarChartInput): BarChartModel {
  const dec = Math.max(0, Math.round(toNum(p.decimals, 0)));
  const unit = p.unit ? ' ' + p.unit : '';
  const fmt = (v: number) => v.toLocaleString('fr-FR', { minimumFractionDigits: dec, maximumFractionDigits: dec }) + unit;

  const values = list(p.values).map((v) => toNum(v, 0));
  const n = values.length;
  const labels = p.labels ? list(p.labels).map(String) : [];
  const subs = p.subLabels ? list(p.subLabels).map(String) : [];
  const H = Math.max(60, toNum(p.height, 200));
  const vmax = Math.max(1e-9, toNum(p.max, n ? Math.max(...values) : 1));
  const maxVal = n ? Math.max(...values) : 0;
  const maxIdx = n ? values.indexOf(maxVal) : -1;

  const hiList = p.highlightIndexes ? list(p.highlightIndexes).map((x) => Number(x)) : [];
  const hlMax = p.highlight === 'max';
  const hlOn = hlMax || hiList.length > 0;
  const isHi = (i: number) => (hlMax && i === maxIdx) || hiList.indexOf(i) >= 0;

  const sv = p.showValues ?? 'none';
  const gap = p.gap == null ? (n > 40 ? 1 : n > 24 ? 2 : n > 12 ? 4 : n > 7 ? 6 : 10) : Math.max(0, toNum(p.gap, 4));
  const radius = n <= 8 ? 6 : 3;
  const room = sv !== 'none' || hlMax ? 20 : 0;
  const barArea = H - room;

  const mode = p.labelMode ?? 'auto';
  const every = mode === 'ends' ? Math.max(0, Math.round(toNum(p.labelEvery, 0))) : 0;
  const bars: BarModel[] = values.map((v, i) => {
    const showVal = sv === 'all' || ((sv === 'max' || hlMax) && i === maxIdx);
    return {
      value: v,
      val: showVal ? fmt(v) : '',
      height: Math.max(2, Math.round((Math.max(0, v) / vmax) * barArea)),
      highlighted: hlOn && isHi(i),
      dimmed: hlOn && !isHi(i),
      label: labels[i] || '',
      every: every > 0 && i % every === 0 ? labels[i] || '' : '',
      sub: subs[i] || '',
      tip: (labels[i] ? labels[i] + ' : ' : '') + fmt(v),
    };
  });

  const each = mode === 'each' || (mode === 'auto' && n <= 8 && labels.length > 0);
  const ends = !each && !(every > 0) && mode !== 'none' && (labels.length > 0 || !!p.startLabel || !!p.endLabel);
  const startLabel = p.startLabel || labels[0] || '';
  const endLabel = p.endLabel || (labels.length ? labels[labels.length - 1] : '');

  const title = p.title ?? 'Barres';
  const auto =
    title + ' : ' + n + ' barres' +
    (n ? ', de ' + fmt(Math.min(...values)) + ' à ' + fmt(maxVal) + (labels[maxIdx] ? ', maximum le ' + labels[maxIdx] : '') : '') +
    (startLabel && endLabel ? ', de ' + startLabel + ' à ' + endLabel : '') + '.';

  return {
    title, bars, height: H, gap, radius, each,
    everyRow: every > 0 && labels.length > 0,
    ends, startLabel, endLabel, legend: hlOn,
    highlightLabel: p.highlightLabel ?? 'Maximum',
    otherLabel: p.otherLabel ?? 'Autres',
    ariaLabel: p.ariaLabel || auto,
  };
}

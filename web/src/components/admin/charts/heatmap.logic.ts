// Pure logic of HeatmapCard (ported from HeatmapCard.dc.html renderVals).

export type HeatmapTone = 'accent' | 'light' | 'danger';
export const HEATMAP_TONES: Record<HeatmapTone, string> = { accent: '#9b8afb', light: '#fafafa', danger: '#f87171' };

export interface HeatmapInput {
  title?: string;
  rowLabels?: ReadonlyArray<string | number>;
  colLabels?: ReadonlyArray<string | number>;
  /** values[row][col]; null / non-numeric = not measurable. */
  values: ReadonlyArray<ReadonlyArray<number | string | null | undefined>>;
  texts?: ReadonlyArray<ReadonlyArray<string | number | null | undefined>> | null;
  rowMeta?: ReadonlyArray<string | number> | null;
  tone?: HeatmapTone;
  max?: number | null;
  showValues?: boolean;
  cellHeight?: number | null;
  colLabelCount?: number | null;
  colLabelEvery?: number | null;
  rowLabelWidth?: number | null;
  missingNote?: string;
  ariaLabel?: string;
}

export interface HeatmapCell {
  missing: boolean;
  tip: string;
  /** Fill opacity (0 when missing). */
  opacity: number;
  text: string;
  darkText: boolean;
}

export interface HeatmapRow {
  label: string;
  meta: string;
  cells: HeatmapCell[];
}

export interface HeatmapColHead {
  label: string;
  /** Label is absolutely positioned (centred over the cell) instead of inline. */
  floating: boolean;
}

export interface HeatmapModel {
  title: string;
  color: string;
  rows: HeatmapRow[];
  colHeads: HeatmapColHead[];
  legend: number[];
  hasText: boolean;
  hasMeta: boolean;
  cellHeight: number;
  radius: number;
  gap: number;
  rowLabelWidth: number;
  minWidth: number;
  missingNote: string;
  missingCount: number;
  ariaLabel: string;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

/** Opacity of a cell for a normalised value in [0, 1], rounded to 0.05 steps. */
export function cellOpacity(v: number): number {
  return Math.round((0.08 + Math.max(0, Math.min(1, v)) * 0.92) * 20) / 20;
}

/** Indices of the column labels that are displayed. */
export function shownColumns(nc: number, wanted: number, every: number): Set<number> {
  const shown = new Set<number>();
  const w = Math.max(2, Math.round(wanted));
  const e = Math.max(0, Math.round(every));
  if (e > 0) {
    for (let i = 0; i < nc; i += e) shown.add(i);
  } else if (nc <= w) {
    for (let i = 0; i < nc; i++) shown.add(i);
  } else {
    for (let k = 0; k < w; k++) shown.add(Math.round((k * (nc - 1)) / (w - 1)));
  }
  return shown;
}

export function computeHeatmap(p: HeatmapInput): HeatmapModel {
  const color = HEATMAP_TONES[p.tone ?? 'accent'] || HEATMAP_TONES.accent;
  const rowLabels = list(p.rowLabels).map(String);
  const colLabels = list(p.colLabels).map(String);
  const values = list(p.values);
  const texts = p.texts ? list(p.texts) : null;
  const meta = p.rowMeta ? list(p.rowMeta).map(String) : null;
  const nr = Math.max(rowLabels.length, values.length);
  const nc = Math.max(colLabels.length, ...values.map((r) => (r ? r.length : 0)), 0);
  const showValues = p.showValues ?? false;
  const hasText = !!texts || showValues;
  const ch = Math.max(14, toNum(p.cellHeight, hasText ? 40 : 22));
  const radius = ch >= 30 ? 8 : 4;
  const rlw = Math.max(24, toNum(p.rowLabelWidth, 40));
  const vmax = toNum(p.max, 1) || 1;

  let best: { v: number; r: string; c: string } | null = null;
  let missing = 0;
  const rows: HeatmapRow[] = Array.from({ length: nr }, (_, ri) => {
    const rv = list(values[ri]);
    const cells: HeatmapCell[] = Array.from({ length: nc }, (_, ci) => {
      const raw = rv[ci];
      const isNull = raw == null || raw === '' || !Number.isFinite(Number(raw));
      const tip0 = (rowLabels[ri] || '') + (colLabels[ci] ? ' · ' + colLabels[ci] : '');
      if (isNull) {
        missing++;
        return { missing: true, tip: tip0 + ' : non mesurable', opacity: 0, text: hasText ? '—' : '', darkText: false };
      }
      const v = Number(raw) / vmax;
      const o = cellOpacity(v);
      if (!best || v > best.v) best = { v, r: rowLabels[ri] || '', c: colLabels[ci] || '' };
      let t = '';
      const tr = texts && texts[ri];
      if (tr && tr[ci] != null) t = String(tr[ci]);
      else if (showValues) t = Math.round(v * 100) + ' %';
      return { missing: false, tip: tip0 + ' : ' + Math.round(v * 100) + ' %', opacity: o, text: t, darkText: o >= 0.85 };
    });
    return { label: rowLabels[ri] || '', meta: meta ? meta[ri] || '' : '', cells };
  });

  const wanted = Math.max(2, Math.round(toNum(p.colLabelCount, 6)));
  const every = Math.max(0, Math.round(toNum(p.colLabelEvery, 0)));
  const shown = shownColumns(nc, wanted, every);
  const floating = every > 0 ? every > 1 : nc > wanted;
  const colHeads: HeatmapColHead[] = Array.from({ length: nc }, (_, i) => ({
    label: shown.has(i) ? colLabels[i] || '' : '',
    floating,
  }));

  const title = p.title ?? 'Carte de chaleur';
  const b = best as { v: number; r: string; c: string } | null;
  const auto =
    title + ' : ' + nr + ' lignes par ' + nc + ' colonnes' +
    (b ? ', valeur la plus élevée : ' + b.r + (b.c ? ' à ' + b.c : '') + ' (' + Math.round(b.v * 100) + ' %)' : '') + '.';

  return {
    title, color, rows, colHeads,
    legend: [0, 0.2, 0.4, 0.6, 0.8, 1].map(cellOpacity),
    hasText, hasMeta: !!meta, cellHeight: ch, radius, gap: 3, rowLabelWidth: rlw,
    minWidth: hasText ? Math.max(360, rlw + nc * 56) : 0,
    missingNote: p.missingNote || (missing && hasText ? '« — » : valeur non mesurable.' : ''),
    missingCount: missing,
    ariaLabel: p.ariaLabel || auto,
  };
}

// Pure logic of HBarListCard (ported from HBarListCard.dc.html renderVals).

export type HBarTone = 'accent' | 'light' | 'muted' | 'danger';
export type HBarFormat = 'number' | 'percent' | 'decimal';
export type HBarSort = 'none' | 'desc' | 'asc';

export interface HBarItem {
  label: string;
  value: number | null;
  hint?: string;
  tone?: HBarTone;
  /** Number: signed delta; string: shown as is (e.g. "[À MESURER]"). */
  delta?: number | string | null;
}

export interface HBarListInput {
  items: ReadonlyArray<HBarItem>;
  format?: HBarFormat;
  unit?: string;
  deltaUnit?: string;
  deltaUnitPlural?: string;
  decimals?: number | null;
  max?: number | null;
  sort?: HBarSort;
  limit?: number | null;
  showRank?: boolean;
  showValue?: boolean;
  barHeight?: number | null;
}

export const HBAR_TONES: Record<HBarTone, string> = { accent: '#9b8afb', light: '#fafafa', muted: '#52525b', danger: '#f87171' };

export interface HBarRow {
  rank: string;
  label: string;
  hint: string;
  value: string;
  /** '' when no delta. */
  delta: string;
  /** null for a textual delta (no arrow). */
  deltaDir: 'up' | 'down' | null;
  color: string;
  widthPct: number;
  rawValue: number;
}

export interface HBarListModel {
  rows: HBarRow[];
  barHeight: number;
  showRank: boolean;
  showValue: boolean;
  rowGap: number;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

export function formatDelta(delta: number, deltaUnit: string, deltaUnitPlural?: string): string {
  const unit = deltaUnit === '' ? '%' : deltaUnit;
  const dd = Math.abs(delta).toLocaleString('fr-FR', { maximumFractionDigits: 1 });
  let du = unit;
  if (unit !== '%' && Math.abs(delta) >= 2) {
    du = deltaUnitPlural ? deltaUnitPlural : /^[A-Za-zÀ-ÿ]{4,}$/.test(unit) && !/[sxz]$/i.test(unit) ? unit + 's' : unit;
  }
  return (delta >= 0 ? '+' : '−') + dd + ' ' + du;
}

export function computeHBarList(p: HBarListInput): HBarListModel {
  const format = p.format ?? 'number';
  const dec = p.decimals == null ? (format === 'decimal' ? 1 : 0) : Math.max(0, Math.round(toNum(p.decimals, 0)));
  const unit = p.unit ? ' ' + p.unit : format === 'percent' ? ' %' : '';
  const fmt = (v: number) => v.toLocaleString('fr-FR', { minimumFractionDigits: dec, maximumFractionDigits: dec }) + unit;
  const dUnit = p.deltaUnit == null || p.deltaUnit === '' ? '%' : String(p.deltaUnit);

  let items = list(p.items).map((it, i) => ({
    label: it.label == null ? '' : String(it.label),
    value: toNum(it.value, 0),
    hint: it.hint || '',
    tone: (it.tone && HBAR_TONES[it.tone] ? it.tone : 'accent') as HBarTone,
    delta: it.delta,
    i,
  }));
  if (p.sort === 'desc') items.sort((a, b) => b.value - a.value || a.i - b.i);
  else if (p.sort === 'asc') items.sort((a, b) => a.value - b.value || a.i - b.i);
  const lim = toNum(p.limit, 0);
  if (lim > 0) items = items.slice(0, lim);

  const vmax = Math.max(1e-9, toNum(p.max, items.length ? Math.max(...items.map((x) => x.value)) : 1));
  const showRank = p.showRank ?? false;
  const showValue = p.showValue ?? true;

  const rows: HBarRow[] = items.map((it, i) => {
    let delta = '';
    let deltaDir: 'up' | 'down' | null = null;
    if (it.delta != null && it.delta !== '') {
      const dn = Number(it.delta);
      if (Number.isFinite(dn)) {
        delta = formatDelta(dn, dUnit, p.deltaUnitPlural);
        deltaDir = dn >= 0 ? 'up' : 'down';
      } else {
        delta = String(it.delta);
      }
    }
    return {
      rank: String(i + 1),
      label: it.label,
      hint: it.hint,
      value: showValue ? fmt(it.value) : '',
      delta,
      deltaDir,
      color: HBAR_TONES[it.tone],
      widthPct: Math.min(100, (Math.max(0, it.value) / vmax) * 100),
      rawValue: it.value,
    };
  });

  return { rows, barHeight: Math.max(4, toNum(p.barHeight, 6)), showRank, showValue, rowGap: showRank ? 12 : 14 };
}

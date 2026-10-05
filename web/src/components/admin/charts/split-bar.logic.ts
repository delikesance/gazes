// Pure logic of SplitBarCard (ported from SplitBarCard.dc.html renderVals).

export type SplitTone = 'accent' | 'light' | 'muted' | 'soft' | 'dim' | 'danger';
export type SplitShowValue = 'percent' | 'raw' | 'none';

export interface SplitSegmentInput {
  label: string;
  value: number | null;
  tone?: SplitTone;
}

export interface SplitGroupInput {
  label?: string;
  segments: ReadonlyArray<SplitSegmentInput>;
}

export interface SplitBarCardInput {
  title?: string;
  segments?: ReadonlyArray<SplitSegmentInput>;
  groups?: ReadonlyArray<SplitGroupInput>;
  showValue?: SplitShowValue;
  unit?: string;
  barHeight?: number | null;
}

export const SPLIT_TONES: Record<SplitTone, string> = {
  accent: '#9b8afb',
  light: '#fafafa',
  muted: '#52525b',
  soft: '#c4b8fc',
  dim: '#3f3f46',
  danger: '#f87171',
};
export const SPLIT_CYCLE: SplitTone[] = ['accent', 'light', 'muted', 'soft', 'dim', 'danger'];

export interface SplitSegmentModel {
  tip: string;
  widthPct: number;
  hasValue: boolean;
  color: string;
  text: string;
}

export interface SplitGroupModel {
  label: string;
  total: string;
  ariaLabel: string;
  segs: SplitSegmentModel[];
}

export interface SplitBarCardModel {
  groups: SplitGroupModel[];
  barHeight: number;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

export const fmtSplitPct = (v: number): string => (Math.round(v * 10) / 10).toLocaleString('fr-FR', { maximumFractionDigits: 1 }) + ' %';

export function computeSplitBarCard(p: SplitBarCardInput): SplitBarCardModel {
  const mode = p.showValue ?? 'percent';
  const unit = p.unit ? ' ' + p.unit : '';
  const raw = (v: number) => v.toLocaleString('fr-FR', { maximumFractionDigits: 2 }) + unit;

  const src: ReadonlyArray<SplitGroupInput> =
    p.groups && p.groups.length ? p.groups : [{ label: '', segments: list(p.segments) }];

  const groups = src.map((g) => {
    const segs = list(g.segments).map((s, i) => ({
      label: s.label == null ? '' : String(s.label),
      value: Math.max(0, toNum(s.value, 0)),
      tone: (s.tone && SPLIT_TONES[s.tone] ? s.tone : SPLIT_CYCLE[i % SPLIT_CYCLE.length]) as SplitTone,
    }));
    const total = segs.reduce((a, s) => a + s.value, 0);
    const sum = total || 1;
    const out: SplitSegmentModel[] = segs.map((s) => {
      const share = (s.value / sum) * 100;
      const val = mode === 'percent' ? fmtSplitPct(share) : mode === 'raw' ? raw(s.value) : '';
      return {
        tip: s.label + ' : ' + fmtSplitPct(share),
        widthPct: share,
        hasValue: s.value > 0,
        color: SPLIT_TONES[s.tone],
        text: s.label + (val ? ' ' + val : ''),
      };
    });
    const lbl = g.label == null ? '' : String(g.label);
    return {
      label: lbl,
      total: lbl && mode === 'raw' ? raw(total) : '',
      ariaLabel: (lbl || p.title || 'Répartition') + ' : ' + segs.map((s) => s.label + ' ' + fmtSplitPct((s.value / sum) * 100)).join(', ') + '.',
      segs: out,
    };
  });

  return { groups, barHeight: Math.max(8, toNum(p.barHeight, 14)) };
}

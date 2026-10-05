// Pure logic of FunnelCard (ported from FunnelCard.dc.html renderVals).

export interface FunnelStepInput {
  label: string;
  value: number | null;
  source?: string;
}

export interface FunnelInput {
  title?: string;
  steps: ReadonlyArray<FunnelStepInput>;
  unit?: string;
  highlightLoss?: boolean;
}

export interface FunnelRow {
  label: string;
  source: string;
  count: string;
  pctStart: string;
  isWorst: boolean;
  widthPct: number;
  hasValue: boolean;
  /** Null for the first step. */
  lossText: string | null;
  convText: string;
}

export interface FunnelModel {
  title: string;
  rows: FunnelRow[];
  /** Index of the step with the largest relative loss, -1 if none. */
  worst: number;
  worstLoss: number;
  ariaLabel: string;
}

function list<T>(v: ReadonlyArray<T> | null | undefined): ReadonlyArray<T> {
  return Array.isArray(v) ? v : [];
}

function toNum(v: unknown, d: number): number {
  const n = Number(v);
  return v == null || v === '' || !Number.isFinite(n) ? d : n;
}

export const fmtInt = (v: number): string => Math.round(v).toLocaleString('fr-FR');
export const fmtPct = (v: number): string =>
  (Math.round(v * 10) / 10).toLocaleString('fr-FR', { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + ' %';

/** Relative loss (0..1) from each step to the next; index 0 is 0. Returns the worst step. */
export function computeLosses(values: ReadonlyArray<number>): { losses: number[]; worst: number; worstLoss: number } {
  const losses = values.map(() => 0);
  let worst = -1;
  let worstLoss = 0;
  values.forEach((v, i) => {
    if (!i) return;
    const prev = values[i - 1];
    const loss = prev > 0 ? (prev - v) / prev : 0;
    losses[i] = loss;
    if (loss > worstLoss) {
      worstLoss = loss;
      worst = i;
    }
  });
  return { losses, worst, worstLoss };
}

export function computeFunnel(p: FunnelInput): FunnelModel {
  const unit = p.unit ? ' ' + p.unit : '';
  const steps = list(p.steps).map((s) => ({
    label: s.label == null ? '' : String(s.label),
    value: Math.max(0, toNum(s.value, 0)),
    source: s.source || '',
  }));
  const start = steps.length ? steps[0].value || 1 : 1;
  const hl = p.highlightLoss ?? true;
  const { losses, worst, worstLoss } = computeLosses(steps.map((s) => s.value));

  const rows: FunnelRow[] = steps.map((s, i) => {
    const isWorst = hl && i === worst;
    const prev = i ? steps[i - 1].value : 0;
    return {
      label: s.label,
      source: s.source,
      count: fmtInt(s.value) + unit,
      pctStart: fmtPct((s.value / start) * 100),
      isWorst,
      widthPct: Math.min(100, (s.value / start) * 100),
      hasValue: s.value > 0,
      lossText: i ? '−' + fmtInt(prev - s.value) + ' (−' + fmtPct(losses[i] * 100) + ')' : null,
      convText: i ? 'Conversion ' + fmtPct(prev > 0 ? (s.value / prev) * 100 : 0) + ' de l’étape précédente' : 'Étape de départ',
    };
  });

  const title = p.title ?? 'Entonnoir';
  const auto =
    title + ' : ' +
    steps
      .map((s, i) => s.label + ' ' + fmtInt(s.value) + (i ? ' (' + fmtPct(steps[i - 1].value > 0 ? (s.value / steps[i - 1].value) * 100 : 0) + ' de l’étape précédente)' : ''))
      .join(', ') +
    (hl && worst > 0 ? '. Plus forte perte : de ' + steps[worst - 1].label + ' à ' + steps[worst].label + ' (−' + fmtPct(worstLoss * 100) + ').' : '.');

  return { title, rows, worst, worstLoss, ariaLabel: auto };
}

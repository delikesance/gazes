// Pure logic of FunnelCard (ported from FunnelCard.dc.html renderVals).

export interface FunnelStepInput {
  label: string;
  value: number | null;
  source?: string;
  /**
   * Accounts that could be measured at this step. When given, the conversion and the loss use it
   * instead of the previous step (nested funnels where too recent accounts are left out).
   */
  eligible?: number | null;
  /** The step cannot be measured yet: no figure, no bar, and it is never flagged as the worst loss. */
  unmeasured?: boolean;
  /** Extra line (e.g. "4 comptes trop récents exclus"); replaces the conversion text when unmeasured. */
  detail?: string;
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

/** Losses of a funnel with eligible bases and unmeasured steps (unmeasured steps never count as a loss). */
export function computeCustomLosses(
  steps: ReadonlyArray<{ value: number; eligible: number | null; unmeasured: boolean }>,
): { losses: number[]; worst: number; worstLoss: number } {
  const losses = steps.map(() => 0);
  let worst = -1;
  let worstLoss = 0;
  steps.forEach((s, i) => {
    if (!i || s.unmeasured) return;
    const base = s.eligible !== null ? s.eligible : steps[i - 1].value;
    const loss = base > 0 ? (base - s.value) / base : 0;
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
    eligible: s.eligible == null ? null : Math.max(0, toNum(s.eligible, 0)),
    unmeasured: !!s.unmeasured,
    detail: s.detail || '',
  }));
  const start = steps.length ? steps[0].value || 1 : 1;
  const hl = p.highlightLoss ?? true;
  const custom = steps.some((s) => s.unmeasured || s.eligible !== null);
  const { losses, worst, worstLoss } = custom ? computeCustomLosses(steps) : computeLosses(steps.map((s) => s.value));

  const rows: FunnelRow[] = steps.map((s, i) => {
    const isWorst = hl && i === worst;
    if (s.unmeasured) {
      return {
        label: s.label,
        source: s.source,
        count: '—',
        pctStart: '',
        isWorst: false,
        widthPct: 0,
        hasValue: false,
        lossText: null,
        convText: s.detail || 'Non mesurable pour le moment',
      };
    }
    const hasBase = s.eligible !== null;
    const prev = hasBase ? (s.eligible as number) : i ? steps[i - 1].value : 0;
    const conv = i
      ? hasBase
        ? 'Conversion ' + fmtPct(prev > 0 ? (s.value / prev) * 100 : 0) + ' de ' + fmtInt(prev) + (prev > 1 ? ' comptes éligibles' : ' compte éligible')
        : 'Conversion ' + fmtPct(prev > 0 ? (s.value / prev) * 100 : 0) + ' de l’étape précédente'
      : 'Étape de départ';
    return {
      label: s.label,
      source: s.source,
      count: fmtInt(s.value) + unit,
      pctStart: fmtPct((s.value / start) * 100),
      isWorst,
      widthPct: Math.min(100, (s.value / start) * 100),
      hasValue: s.value > 0,
      lossText: i ? '−' + fmtInt(prev - s.value) + ' (−' + fmtPct(losses[i] * 100) + ')' : null,
      convText: conv + (s.detail ? ' · ' + s.detail : ''),
    };
  });

  const title = p.title ?? 'Entonnoir';
  const auto =
    title + ' : ' +
    steps
      .map((s, i) => {
        if (s.unmeasured) return s.label + ' non mesurable' + (s.detail ? ' (' + s.detail + ')' : '');
        const base = s.eligible !== null ? s.eligible : i ? steps[i - 1].value : 0;
        return s.label + ' ' + fmtInt(s.value) + (i ? ' (' + fmtPct(base > 0 ? (s.value / base) * 100 : 0) + (s.eligible !== null ? ' des comptes éligibles)' : ' de l’étape précédente)') : '');
      })
      .join(', ') +
    (hl && worst > 0 ? '. Plus forte perte : de ' + steps[worst - 1].label + ' à ' + steps[worst].label + ' (−' + fmtPct(worstLoss * 100) + ').' : '.');

  return { title, rows, worst, worstLoss, ariaLabel: auto };
}

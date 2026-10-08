"use client";

import { useCallback, useMemo, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import type { AdminCosts, AdminOverview } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef, Row } from "@/components/admin/cards";
import { BarChartCard, LineChartCard } from "@/components/admin/charts";
import { Button, Chip, Gauge, PillGroup, ProgressBar, SeverityBadge, StatTile } from "@/components/admin/ui";
import { rangeLabel } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import { REVENUE_IDEAS, RISKS } from "./business.content";
import {
  ASSUMPTION_DEFS,
  HOURS_NEEDS,
  MISSING_FILL,
  MISSING_MEASURE,
  SCENARIOS,
  STORAGE_KEY,
  USERS_NEEDS,
  assumptionEditText,
  buildBaseline,
  clampAssumption,
  growthClampNotice,
  scenarioGrowth,
  costBreakdown,
  decOr,
  findings,
  fmtDec,
  fmtInt,
  intOr,
  mergeAssumptions,
  milestones,
  missingFor,
  monthLabel,
  monthLabels,
  parseAssumptionInput,
  parseStoredState,
  projectAll,
  saturation,
  saturationText,
  scenarioDef,
  serializeState,
  seriesOf,
  stepAssumption,
} from "./business.logic";
import type { AssumptionDef, AssumptionKey, Assumptions, MeasuredFacts, MonthRow, ScenarioId } from "./business.logic";

export interface BusinessViewProps {
  costs: AdminCosts;
  overview: AdminOverview;
  periodDays: number;
  from?: string;
  to?: string;
  /** Response timestamp: month 0 of the projection (deterministic, same on server and browser). */
  generatedAt: string;
}

const MONEY_NOTE = "Montants dans l'unité des prix configurés : aucune devise n'est supposée.";

function readStorage(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeStorage(value: string | null): void {
  try {
    if (value === null) window.localStorage.removeItem(STORAGE_KEY);
    else window.localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // Storage can be blocked (private window, site data cleared): the page works without it.
  }
}

// Local-only store for the assumptions. The in-memory copy keeps the page working when storage is blocked;
// the server render and the first client render see "nothing stored" (no hydration mismatch).
let memory: string | null | undefined;
const listeners = new Set<() => void>();

function snapshot(): string | null {
  return memory !== undefined ? memory : readStorage();
}

function persist(value: string | null): void {
  memory = value;
  writeStorage(value);
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  const onStorage = (e: StorageEvent) => {
    if (e.key === STORAGE_KEY || e.key === null) {
      memory = undefined;
      listener();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
}

const NOTHING_STORED = (): string | null => null;

/** Hint under an assumption: what the initial value is based on. */
function hintFor(def: AssumptionDef, facts: MeasuredFacts, periodDays: number): string {
  if (def.key === "growth" && facts.measuredGrowth === null && facts.growthNote !== null) {
    return `${facts.growthNote} Bornée à −50 / +100 % par mois. Prudent ×0,5, ambitieux ×1,5.`;
  }
  if (def.key === "growth" && facts.measuredGrowth !== null && facts.newUsers !== null && facts.totalUsers !== null) {
    return `Mesurée : ${fmtInt(facts.newUsers)} nouveau${facts.newUsers > 1 ? "x" : ""} inscrit${facts.newUsers > 1 ? "s" : ""} pour ${fmtInt(facts.totalUsers - facts.newUsers)} avant la période de ${periodDays} jours, ramenés à 30 jours. Bornée à −50 / +100 % par mois. Prudent ×0,5, ambitieux ×1,5.`;
  }
  return def.hint;
}

type FieldOrigin = "mesuré" | "saisi" | "vide";

function originOf(key: AssumptionKey, value: number | null, defaults: Assumptions, edited: Partial<Assumptions>): FieldOrigin {
  if (key in edited) return value === null ? "vide" : "saisi";
  return value === null ? "vide" : "mesuré";
}

interface AssumptionFieldProps {
  def: AssumptionDef;
  hint: string;
  value: number | null;
  origin: FieldOrigin;
  onChange: (value: number | null) => void;
}

function AssumptionField({ def, hint, value, origin, onChange }: AssumptionFieldProps) {
  const [draft, setDraft] = useState<string | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const inputId = `business-${def.key}`;
  const hintId = `${inputId}-hint`;
  const errorId = `${inputId}-error`;
  const shown = draft ?? assumptionEditText(def, value);

  const commit = (next: number | null) => {
    setDraft(null);
    setInvalid(false);
    setNotice(null);
    onChange(next);
  };

  return (
    <div
      style={{
        flex: "1 1 280px",
        minWidth: 0,
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        gap: 12,
        padding: 16,
        borderRadius: 20,
        background: "#17171a",
      }}
    >
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", justifyContent: "space-between", gap: 8 }}>
        <label htmlFor={inputId} style={{ fontWeight: 500, flex: "1 1 160px", minWidth: 0 }}>
          {def.label}
        </label>
        <Chip label={origin} tone={origin === "saisi" ? "accent" : "neutral"} />
      </div>
      <span id={hintId} style={{ fontSize: 12, color: "#a1a1aa" }}>
        {hint}
      </span>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
        <Button
          variant="secondary"
          icon="minus"
          iconOnly
          aria-label={`Diminuer : ${def.label}`}
          style={{ width: 44, paddingLeft: 0, paddingRight: 0, flex: "none" }}
          onClick={() => commit(stepAssumption(def, value, -1))}
        />
        <div style={{ flex: 1, minWidth: 0, display: "flex", alignItems: "baseline", justifyContent: "center", gap: 8 }}>
          <input
            id={inputId}
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={shown}
            placeholder={def.placeholder}
            aria-describedby={invalid ? `${hintId} ${errorId}` : hintId}
            aria-invalid={invalid ? "true" : "false"}
            onFocus={() => setDraft(assumptionEditText(def, value))}
            onChange={(e) => {
              const text = e.target.value;
              setDraft(text);
              const parsed = parseAssumptionInput(text);
              if (parsed === undefined) {
                setInvalid(true);
                return;
              }
              setInvalid(false);
              setNotice(def.key === "growth" ? growthClampNotice(parsed) : null);
              onChange(parsed === null ? null : clampAssumption(def, parsed));
            }}
            onBlur={() => {
              setDraft(null);
              setInvalid(false);
            }}
            style={{
              width: "100%",
              minWidth: 0,
              minHeight: 44,
              boxSizing: "border-box",
              textAlign: "center",
              fontFamily: "'Geist Mono', monospace",
              fontSize: 16,
              fontVariantNumeric: "tabular-nums",
              color: "#fafafa",
              background: "#111113",
              border: 0,
              borderRadius: 14,
              boxShadow: invalid ? "inset 0 0 0 1px #f87171" : "inset 0 0 0 1px rgba(255,255,255,0.07)",
              outlineOffset: 2,
            }}
          />
          <span style={{ fontSize: 12, color: "#a1a1aa", whiteSpace: "nowrap" }}>{def.unit}</span>
        </div>
        <Button
          variant="secondary"
          icon="plus"
          iconOnly
          aria-label={`Augmenter : ${def.label}`}
          style={{ width: 44, paddingLeft: 0, paddingRight: 0, flex: "none" }}
          onClick={() => commit(stepAssumption(def, value, 1))}
        />
      </div>
      {notice !== null ? (
        <span role="status" style={{ fontSize: 12, color: "#fbbf24" }}>
          {notice}
        </span>
      ) : null}
      {invalid ? (
        <span id={errorId} role="alert" style={{ fontSize: 12, color: "#f87171" }}>
          Valeur invalide : saisir un nombre positif (la virgule est acceptée), ou laisser vide.
        </span>
      ) : null}
    </div>
  );
}

function NeedsBlock({ title, missing }: { title: string; missing: string[] }) {
  return (
    <div role="status" style={{ ...SURFACE, flex: "1 1 320px", gap: 8 }}>
      <h3 style={{ margin: 0, fontSize: 18, fontWeight: 500, letterSpacing: "-0.01em" }}>{title}</h3>
      <span style={{ fontFamily: "'Geist Mono', monospace", fontSize: 16, color: "#a1a1aa" }}>{MISSING_MEASURE}</span>
      <p style={MUTED_TEXT}>
        {missing.length > 0
          ? `Hypothèses à renseigner dans le modèle ci-dessous : ${missing.join(", ")}.`
          : "Les mesures nécessaires ne sont pas disponibles."}
      </p>
    </div>
  );
}

function Block({ label, title, subtitle, children }: { label: string; title: string; subtitle?: string; children: ReactNode }) {
  return (
    <section aria-label={label} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <SectionCard bare title={title} subtitle={subtitle} />
      {children}
    </section>
  );
}

const MILESTONE_COLUMNS: ColumnDef[] = [
  { key: "month", label: "Mois", type: "text", sortable: false },
  { key: "users", label: "Inscrits", type: "text", align: "right", sortable: false },
  { key: "actives", label: "Actifs", type: "text", align: "right", sortable: false },
  { key: "hours", label: "Heures", type: "text", align: "right", sortable: false },
  { key: "peak", label: "Pic de flux", type: "text", align: "right", sortable: false },
  { key: "servers", label: "Serveurs", type: "text", align: "right", sortable: false },
  { key: "cost", label: "Coût", type: "text", align: "right", sortable: false },
  { key: "cph", label: "Coût / h", type: "text", align: "right", sortable: false },
  { key: "revenue", label: "Revenu potentiel", type: "text", align: "right", muted: true, sortable: false },
];

function milestoneRow(r: MonthRow, isoDate: string): Row {
  return {
    month: monthLabel(isoDate, r.month) + (r.month === 0 ? " (aujourd'hui)" : ""),
    users: intOr(r.users),
    actives: intOr(r.actives),
    hours: r.hours === null ? MISSING_MEASURE : `${fmtInt(r.hours)} h`,
    peak: intOr(r.peak),
    servers: intOr(r.servers),
    cost: r.cost === null ? MISSING_FILL : `${r.costPartial ? "≥ " : ""}${fmtDec(r.cost, 2)}`,
    cph: decOr(r.costPerHour, 3),
    revenue: decOr(r.revenue, 2),
  };
}

function fmtGB(bytes: number): string {
  return fmtDec(bytes / 1e9, bytes >= 1e11 ? 0 : 1);
}

export function BusinessView({ costs, overview, periodDays, from, to, generatedAt }: BusinessViewProps) {
  const facts = useMemo(() => buildBaseline(costs, overview, periodDays), [costs, overview, periodDays]);
  const { baseline, defaults } = facts;

  const raw = useSyncExternalStore(subscribe, snapshot, NOTHING_STORED);
  const stored = useMemo(() => parseStoredState(raw) ?? { scenario: "tendance" as ScenarioId, edited: {} as Partial<Assumptions> }, [raw]);
  const scenario = stored.scenario;
  const edited = stored.edited;
  const setScenario = useCallback((next: ScenarioId) => persist(serializeState({ ...stored, scenario: next })), [stored]);
  const setValue = useCallback(
    (key: AssumptionKey, value: number | null) => persist(serializeState({ ...stored, edited: { ...stored.edited, [key]: value } })),
    [stored],
  );
  const resetAll = useCallback(() => persist(null), []);

  const assumptions = useMemo(() => mergeAssumptions(defaults, edited), [defaults, edited]);
  const all = useMemo(() => projectAll(baseline, assumptions), [baseline, assumptions]);
  const sel = scenarioDef(scenario);
  const rows = all[scenario];
  const first = rows[0];
  const last = rows[rows.length - 1];
  const limit = assumptions.streamLimit;
  const load = costs.load;

  const breakdown = useMemo(() => costBreakdown(costs, baseline), [costs, baseline]);
  const sats = useMemo(
    () => Object.fromEntries(SCENARIOS.map((s) => [s.id, saturation(all[s.id], limit)])) as Record<ScenarioId, ReturnType<typeof saturation>>,
    [all, limit],
  );
  const satSel = sats[scenario];
  const satText = saturationText(satSel, generatedAt);
  const watch = useMemo(
    () => findings({ breakdown, assumptions, scenario, rows, saturation: satSel, isoDate: generatedAt, peak: baseline.peak }),
    [breakdown, assumptions, scenario, rows, satSel, generatedAt, baseline.peak],
  );

  const labels = useMemo(() => monthLabels(generatedAt), [generatedAt]);
  const usersByScenario = SCENARIOS.map((s) => ({ s, values: seriesOf(all[s.id], "users") }));
  const chartUsers = usersByScenario.every((x) => x.values !== null);
  const hoursSeries = seriesOf(rows, "hours");
  const costSeries = hoursSeries === null ? null : seriesOf(rows, "cost");
  const tableRows = useMemo(() => milestones(rows).map((r) => milestoneRow(r, generatedAt)), [rows, generatedAt]);

  const c = costs.costs;
  const perHour = c.per_watch_hour.measured ? c.per_watch_hour.value : null;
  const perActive = c.per_active_user.measured ? c.per_active_user.value : null;
  const peak = baseline.peak;
  const hasGauge = limit !== null && peak !== null;
  const orderedScenarios = [sel, ...SCENARIOS.filter((s) => s.id !== sel.id)];
  const growthPct = assumptions.growth;

  return (
    <>
      <AdminPageHeader
        eyebrow="Pilotage"
        title="Business"
        subtitle={`${rangeLabel(from, to)} · projection sur 12 mois`}
        badges={<span className="admin-chip">Hypothèses locales, rien n&apos;est enregistré</span>}
      />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        <KpiCard
          label="Coût d'infrastructure"
          value={breakdown.total}
          unit="unités / mois"
          missingLabel={MISSING_FILL}
          tag={breakdown.total === null ? undefined : breakdown.partial ? "partiel : lignes à renseigner" : "mesuré"}
          showSpark={false}
        />
        <KpiCard label="Coût par heure regardée" value={perHour} unit="unités" missingLabel={MISSING_FILL} tag={perHour === null ? undefined : "mesuré"} showSpark={false} />
        <KpiCard label="Coût par utilisateur actif" value={perActive} unit="unités" missingLabel={MISSING_FILL} tag={perActive === null ? undefined : "mesuré"} showSpark={false} />
        <KpiCard
          label="Pic de flux simultanés"
          value={peak}
          unit={limit !== null ? `/ ${fmtInt(limit)}` : "flux"}
          tag={peak === null ? undefined : `${baseline.peakEstimated ? "estimé" : "mesuré"}${baseline.peakTruncated ? " · tronqué" : ""}${limit === null ? " · limite à mesurer" : ""}`}
          showSpark={false}
        />
        <KpiCard
          label="Saturation projetée"
          value={satSel.status === "unknown" ? null : satText.date}
          tag={`scénario ${sel.label.toLowerCase()}`}
          showSpark={false}
        />
      </section>

      <section aria-label="À regarder" style={SURFACE}>
        <SectionCard bare title="À regarder" subtitle="Constats chiffrés et actionnables" />
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
          {watch.map((w) => (
            <InsightCard key={w.id} surface="tile" level={w.level} title={w.title} text={w.text} basis={280} />
          ))}
        </div>
      </section>

      <div style={WRAP_ROW}>
        <section aria-label="Coûts d'infrastructure" style={{ ...SURFACE, flex: "3 1 480px" }}>
          <SectionCard
            bare
            title="Coûts d'infrastructure par mois"
            subtitle={breakdown.total === null ? `Aucune ligne chiffrée. ${MONEY_NOTE}` : `Total des lignes chiffrées : ${fmtDec(breakdown.total, 2)}. ${MONEY_NOTE}`}
          />
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column" }}>
            {breakdown.lines.map((l) => (
              <li key={l.id} style={{ display: "flex", flexDirection: "column", gap: 8, padding: "14px 0", borderTop: "1px solid rgba(255,255,255,0.07)" }}>
                <span style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: "6px 12px" }}>
                  <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                    <span style={{ fontWeight: 500 }}>{l.label}</span>
                    <span style={{ fontSize: 12, color: "#a1a1aa", overflowWrap: "anywhere" }}>{l.detail}</span>
                  </span>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
                    <Chip tone={l.value === null ? "code" : "neutral"} label={l.tag} />
                    <span style={{ minWidth: 96, textAlign: "right", fontSize: 16, fontVariantNumeric: "tabular-nums" }}>
                      {l.value === null ? MISSING_FILL : fmtDec(l.value, 2)}
                    </span>
                  </span>
                </span>
                <ProgressBar layout="bar" value={l.bar} label={l.label} showValue={false} height={6} />
              </li>
            ))}
          </ul>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Coût par heure regardée" value={perHour === null ? "" : fmtDec(perHour, 2)} missingLabel={MISSING_FILL} size="sm" basis={160} />
            <StatTile label="Coût par utilisateur actif" value={perActive === null ? "" : fmtDec(perActive, 2)} missingLabel={MISSING_FILL} size="sm" basis={160} />
          </div>
        </section>

        <section aria-label="Capacité" style={{ ...SURFACE, flex: "2 1 340px" }}>
          <SectionCard bare title="Capacité" />
          {hasGauge ? (
            <Gauge
              label="Pic de flux simultanés"
              value={peak}
              max={limit}
              threshold={limit * 0.8}
              thresholdLabel="80 % de la limite"
              alertFrom={100}
              warnFrom={80}
              unit="flux"
              size="lg"
              height={16}
            />
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={MONO_LABEL}>Pic de flux simultanés</span>
              <span style={{ fontSize: 28, letterSpacing: "-0.02em", fontVariantNumeric: "tabular-nums" }}>{peak === null ? MISSING_MEASURE : `${fmtInt(peak)} flux`}</span>
              <span style={{ fontFamily: "'Geist Mono', monospace", fontSize: 14, color: "#a1a1aa" }}>Limite de flux : {MISSING_MEASURE}</span>
            </div>
          )}
          <p style={MUTED_TEXT}>
            {peak === null ? "Pic de flux non mesuré. " : `Pic de ${fmtInt(peak)} flux (${baseline.peakEstimated ? "estimé à partir des séances" : "mesuré"}${baseline.peakTruncated ? ", tronqué" : ""}). `}
            {limit === null ? (
              <>
                La limite n&apos;est pas mesurée : <a href="#hypotheses" style={{ color: "#fafafa" }}>la renseigner dans le modèle d&apos;hypothèses</a> après un test de charge. La jauge apparaît dès qu&apos;elle est connue.
              </>
            ) : (
              "Limite saisie dans le modèle d'hypothèses, à valider par un test de charge."
            )}
          </p>
          <span style={MONO_LABEL}>Date de saturation projetée</span>
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 8 }}>
            {SCENARIOS.map((s) => {
              const t = saturationText(sats[s.id], generatedAt);
              const g = scenarioGrowth(growthPct, s.id);
              return (
                <li
                  key={s.id}
                  aria-current={s.id === scenario ? "true" : undefined}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 12,
                    padding: "12px 14px",
                    borderRadius: 20,
                    background: "#17171a",
                    boxShadow: s.id === scenario ? "inset 0 0 0 1px #9b8afb" : "inset 0 0 0 1px rgba(255,255,255,0.07)",
                  }}
                >
                  <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                    <span style={{ fontWeight: 500 }}>
                      {s.label}
                      {s.id === scenario ? " (choisi)" : ""}
                    </span>
                    <span style={{ fontSize: 12, color: "#a1a1aa" }}>{g === null ? `croissance ${MISSING_MEASURE}` : `${fmtDec(g, 1)} % par mois`}</span>
                  </span>
                  <span style={{ textAlign: "right", display: "flex", flexDirection: "column", gap: 2 }}>
                    <span style={{ fontWeight: 500 }}>{t.date}</span>
                    <span style={{ fontSize: 12, color: "#a1a1aa" }}>{t.detail}</span>
                  </span>
                </li>
              );
            })}
          </ul>
        </section>
      </div>

      <section aria-label="Charge mesurée" style={SURFACE}>
        <SectionCard
          bare
          title="Charge mesurée du serveur"
          subtitle={
            load && load.measured_days > 0
              ? `Compteurs enregistrés sur ${load.measured_days} jour${load.measured_days > 1 ? "s" : ""} de la période (depuis le ${load.since}).`
              : "Aucune donnée de charge enregistrée sur cette période : les compteurs démarrent avec cette version."
          }
        />
        <div style={WRAP_ROW}>
          <StatTile label="Volume servi" value={load && load.measured_days > 0 ? fmtGB(load.bytes_out) : ""} unit="Go" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="CPU ffmpeg (conversion H.264)" value={load && load.measured_days > 0 ? fmtDec(load.cpu_seconds_transcode / 3600, 2) : ""} unit="h CPU" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="CPU ffmpeg (copie)" value={load && load.measured_days > 0 ? fmtDec(load.cpu_seconds_copy / 3600, 2) : ""} unit="h CPU" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="Séances iPhone / iPad" value={load?.apple_share == null ? "" : fmtDec(load.apple_share * 100, 1)} unit="%" hint="iPad en mode bureau non détecté" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="Clients sans AV1" value={load?.no_av1_share == null ? "" : fmtDec(load.no_av1_share * 100, 1)} unit="%" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="Séances à convertir" value={load?.transcode_share == null ? "" : fmtDec(load.transcode_share * 100, 1)} unit="%" missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="Pic de remuxes" value={load && load.measured_days > 0 ? fmtInt(load.peak_remuxes) : ""} unit={load && load.remux_limit > 0 ? `/ ${fmtInt(load.remux_limit)}` : undefined} hint={load && load.remux_rejected > 0 ? `${fmtInt(load.remux_rejected)} refusé${load.remux_rejected > 1 ? "s" : ""}` : undefined} missingLabel={MISSING_MEASURE} size="sm" basis={160} />
          <StatTile label="Pic de processus ffmpeg" value={load && load.measured_days > 0 ? fmtInt(load.peak_ffmpeg) : ""} missingLabel={MISSING_MEASURE} size="sm" basis={160} />
        </div>
      </section>

      <section aria-label="Projection à 12 mois" style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <div style={{ flex: "1 1 320px", minWidth: 0 }}>
            <SectionCard
              bare
              title="Projection à 12 mois"
              subtitle={`Scénario « ${sel.label} » : ${sel.note}, soit ${scenarioGrowth(growthPct, sel.id) === null ? MISSING_MEASURE : `${fmtDec(scenarioGrowth(growthPct, sel.id) ?? 0, 1)} % par mois`}.`}
              subtitlePlacement="below"
            />
          </div>
          <PillGroup
            ariaLabel="Scénario"
            options={SCENARIOS.map((s) => ({ id: s.id, label: s.label }))}
            value={scenario}
            onChange={(id) => setScenario(String(id) as ScenarioId)}
          />
        </div>

        <div style={WRAP_ROW}>
          {chartUsers ? (
            <LineChartCard
              title="Inscrits"
              subtitle="Violet : scénario choisi · gris : les deux autres"
              series={orderedScenarios.map((s) => ({
                label: s.label + (s.id === scenario ? " (choisi)" : ""),
                values: seriesOf(all[s.id], "users") ?? [],
                tone: s.id === scenario ? ("accent" as const) : ("muted" as const),
              }))}
              xLabels={labels}
              yMin={0}
              yTickCount={4}
              decimals={0}
              height={240}
              ariaLabel={`Courbes d'inscrits sur 12 mois. Scénario ${sel.label.toLowerCase()} : de ${intOr(first.users)} à ${intOr(last.users)} inscrits.`}
              grow={1}
              basis={640}
            />
          ) : (
            <NeedsBlock title="Inscrits" missing={missingFor(assumptions, USERS_NEEDS)} />
          )}
        </div>

        <div style={WRAP_ROW}>
          {hoursSeries !== null ? (
            <BarChartCard
              title="Heures regardées par mois"
              subtitle={`${intOr(first.hours)} h aujourd'hui, ${intOr(last.hours)} h dans 12 mois`}
              values={hoursSeries}
              labelMode="ends"
              startLabel={labels[0]}
              endLabel={labels[labels.length - 1]}
              unit="h"
              height={200}
              ariaLabel={`Heures regardées par mois, de ${intOr(first.hours)} à ${intOr(last.hours)} sur 12 mois.`}
              grow={1}
              basis={320}
            />
          ) : (
            <NeedsBlock title="Heures regardées par mois" missing={missingFor(assumptions, HOURS_NEEDS)} />
          )}
          {costSeries !== null ? (
            <LineChartCard
              title="Coût mensuel"
              subtitle={`${decOr(first.cost, 2)} aujourd'hui, ${decOr(last.cost, 2)} dans 12 mois${last.costPartial ? " (lignes chiffrées seulement)" : ""}`}
              series={[{ label: "Coût mensuel", values: costSeries, tone: "accent" }]}
              xLabels={labels}
              yMin={0}
              yTickCount={2}
              decimals={0}
              area
              height={200}
              ariaLabel={`Coût mensuel sur 12 mois, de ${decOr(first.cost, 2)} à ${decOr(last.cost, 2)} unités.`}
              grow={1}
              basis={320}
            />
          ) : (
            <NeedsBlock title="Coût mensuel" missing={missingFor(assumptions, HOURS_NEEDS)} />
          )}
        </div>

        <div style={WRAP_ROW}>
          <DataTable
            id="jalons"
            title={`Jalons du scénario « ${sel.label.toLowerCase()} »`}
            caption={`Jalons du scénario ${sel.label.toLowerCase()} à 0, 3, 6, 9 et 12 mois`}
            columns={MILESTONE_COLUMNS}
            rows={tableRows}
            rowKey="month"
            minWidth={760}
            footnote={`« ≥ » : coût partiel, somme des lignes chiffrées seulement. Serveurs : ${MISSING_MEASURE} tant que la limite de flux n'est pas renseignée (le coût compte alors une machine). ${MONEY_NOTE}`}
            grow={1}
            basis={640}
          />
        </div>
      </section>

      <section id="hypotheses" aria-label="Modèle d'hypothèses" style={SURFACE}>
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
          <div style={{ flex: "1 1 320px", minWidth: 0 }}>
            <SectionCard
              bare
              title="Modèle d'hypothèses"
              subtitle="Modifiez une valeur : la projection se recalcule. Une valeur vide reste vide : rien n'est inventé. Les valeurs sont gardées dans ce navigateur seulement."
              subtitlePlacement="below"
            />
          </div>
          <Button variant="secondary" onClick={resetAll}>
            Réinitialiser
          </Button>
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
          {ASSUMPTION_DEFS.map((def) => (
            <AssumptionField
              key={def.key}
              def={def}
              hint={hintFor(def, facts, periodDays)}
              value={assumptions[def.key]}
              origin={originOf(def.key, assumptions[def.key], defaults, edited)}
              onChange={(v) => setValue(def.key, v)}
            />
          ))}
        </div>
      </section>

      <Block label="Pistes de revenus à valider" title="Pistes de revenus à valider" subtitle="Hypothèses, aucun chiffre promis">
        <div style={WRAP_ROW}>
          {REVENUE_IDEAS.map((v) => (
            <article key={v.title} style={{ ...SURFACE, flex: "1 1 320px", alignItems: "flex-start", gap: 12 }}>
              <Chip tone="neutral" label="Hypothèse" />
              <h3 style={{ margin: 0, fontSize: 18, fontWeight: 500, letterSpacing: "-0.01em" }}>{v.title}</h3>
              <span style={{ fontSize: 14 }}>{v.idea}</span>
              <span style={{ ...MONO_LABEL, color: "#9b8afb" }}>À mesurer pour la valider</span>
              <ul style={{ margin: 0, padding: "0 0 0 18px", display: "flex", flexDirection: "column", gap: 6, fontSize: 13, color: "#a1a1aa" }}>
                {v.measures.map((m) => (
                  <li key={m}>{m}</li>
                ))}
              </ul>
              <span style={{ fontSize: 12, color: "#a1a1aa" }}>{v.caveat}</span>
            </article>
          ))}
        </div>
      </Block>

      <Block label="Risques" title="Risques">
        <div style={WRAP_ROW}>
          {RISKS.map((r) => (
            <article key={r.title} style={{ ...SURFACE, flex: "1 1 320px", alignItems: "flex-start", gap: 12 }}>
              <SeverityBadge level={r.level} />
              <h3 style={{ margin: 0, fontSize: 18, fontWeight: 500, letterSpacing: "-0.01em" }}>{r.title}</h3>
              <span style={{ fontSize: 14 }}>{r.text}</span>
              <span style={MONO_LABEL}>Signal à surveiller</span>
              <span style={{ fontSize: 13, color: "#a1a1aa" }}>{r.signal}</span>
              <span style={{ ...MONO_LABEL, color: "#9b8afb" }}>Piste d&apos;atténuation</span>
              <span style={{ fontSize: 13 }}>{r.mitigation}</span>
            </article>
          ))}
        </div>
      </Block>
    </>
  );
}

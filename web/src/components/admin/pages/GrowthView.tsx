import type { AdminGrowth } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import { BarChartCard, FunnelCard, HeatmapCard, LineChartCard } from "@/components/admin/charts";
import { Gauge, ProgressBar, StatTile } from "@/components/admin/ui";
import { fmtInt, rangeLabel } from "./fr-date";
import { LIST_RESET, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  churnModel,
  cumulativeModel,
  delayModel,
  funnelNote,
  funnelSteps,
  growthInsights,
  growthKpis,
  isNotMeasured,
  cohortModel,
  weeksModel,
} from "./growth.logic";

export interface GrowthViewProps {
  growth: AdminGrowth;
  periodDays: number;
  from?: string;
  to?: string;
}

const SOURCES = ["Direct", "Moteurs de recherche", "Réseaux sociaux", "Recommandation (lien partagé)", "Autres sites"];

const NOTE: React.CSSProperties = { margin: 0, fontSize: 12, color: "#a1a1aa" };

export function GrowthView({ growth, periodDays, from, to }: GrowthViewProps) {
  const kpis = growthKpis(growth, periodDays);
  const insights = growthInsights(growth);
  const steps = funnelSteps(growth);
  const cohorts = cohortModel(growth);
  const cumul = cumulativeModel(growth);
  const weeks = weeksModel(growth);
  const delay = delayModel(growth);
  const churn = churnModel(growth);
  const total = growth.cumulative_users.length ? growth.cumulative_users[growth.cumulative_users.length - 1].users : null;
  const visitorsMissing = isNotMeasured(growth, "visitor_to_signup");
  const sourcesMissing = isNotMeasured(growth, "acquisition_sources");

  return (
    <>
      <AdminPageHeader eyebrow="Acquisition et rétention" title="Croissance" subtitle={rangeLabel(from, to)} />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard
            key={k.label}
            label={k.label}
            value={k.value}
            note={k.note}
            delta={k.delta}
            deltaUnit={k.deltaUnit}
            vs={k.vs}
            series={k.series}
            showSpark={k.series.length > 1}
          />
        ))}
      </section>

      <section aria-label="À regarder" style={SURFACE}>
        <SectionCard title="À regarder" subtitle="Constats chiffrés tirés de cette page" bare />
        {insights.length > 0 ? (
          <ul style={{ ...LIST_RESET, display: "flex", flexWrap: "wrap", gap: 12 }}>
            {insights.map((n) => (
              <li key={n.title} style={{ flex: "1 1 260px", minWidth: 0, display: "flex" }}>
                <InsightCard level={n.level} title={n.title} text={n.text} surface="tile" />
              </li>
            ))}
          </ul>
        ) : (
          <p style={MUTED_TEXT}>Rien à signaler : pas assez de comptes ni de séances sur la période pour établir un constat.</p>
        )}
      </section>

      <div style={{ display: "flex" }}>
        {growth.funnel.cohort_size > 0 ? (
          <FunnelCard
            title="Entonnoir d'activation"
            subtitle={funnelNote(growth, periodDays)}
            steps={steps}
            note="Les barres sont proportionnelles à la cohorte de départ. La conversion est mesurée d'une étape à la suivante sur les comptes assez anciens pour être mesurés ; la perte la plus forte est entourée."
          />
        ) : (
          <section aria-label="Entonnoir d'activation" style={{ ...SURFACE, flex: 1 }}>
            <SectionCard title="Entonnoir d'activation" bare />
            <p style={MUTED_TEXT}>Aucun compte créé sur la période : l&apos;entonnoir ne peut pas être calculé.</p>
          </section>
        )}
      </div>

      <div style={{ display: "flex" }}>
        <HeatmapCard
          title="Rétention par cohorte hebdomadaire"
          subtitle="Part des inscrits de la semaine revus à J1, J7, J14 et J30"
          rowHeader="Cohorte (7 jours)"
          metaLabel="Inscrits"
          rowLabels={cohorts.rowLabels}
          rowMeta={cohorts.rowMeta}
          colLabels={cohorts.colLabels}
          values={cohorts.values}
          texts={cohorts.texts}
          max={cohorts.max}
          colLabelCount={4}
          rowLabelWidth={150}
          legendHigh={`Plus (de 0 % à ${cohorts.max} %)`}
          missingNote="« — » : cohorte trop récente ou sans inscrit, pas encore mesurable."
          ariaLabel={
            cohorts.hasData
              ? "Rétention des cohortes hebdomadaires, carte de chaleur avec les pourcentages dans chaque case."
              : "Rétention des cohortes hebdomadaires : aucune cohorte n'est encore mesurable."
          }
        />
      </div>

      <div style={WRAP_ROW}>
        <div style={{ flex: "2 1 520px", minWidth: 0, display: "flex", flexDirection: "column", gap: 16 }}>
          <LineChartCard
            title="Inscrits cumulés"
            subtitle={cumul.summary}
            series={[{ label: "Inscrits cumulés", values: cumul.values, tone: "accent" }]}
            xLabels={cumul.labels}
            yMin={cumul.yMin}
            area
            height={240}
            ariaLabel={cumul.ariaLabel}
          />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Inscrits sur la période" value={fmtInt(growth.funnel.cohort_size)} size="sm" basis={140} />
            <StatTile label="Total à la fin de la période" value={total === null ? "" : fmtInt(total)} size="sm" basis={140} />
          </div>
        </div>

        <div style={{ flex: "1 1 340px", minWidth: 0, display: "flex", flexDirection: "column", gap: 16 }}>
          <BarChartCard
            title="Inscrits par semaine"
            subtitle={weeks.subtitle}
            values={weeks.values}
            labels={weeks.labels}
            labelMode="ends"
            showValues="all"
            height={210}
            ariaLabel={weeks.ariaLabel}
            footnote="La dernière semaine est en cours."
          />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Meilleure semaine" value={fmtInt(weeks.best)} size="sm" basis={120} />
            <StatTile label="Semaine en cours" value={fmtInt(weeks.current)} size="sm" basis={120} />
          </div>
        </div>
      </div>

      <div style={WRAP_ROW}>
        <section aria-label="Inscription vers première séance" style={{ ...SURFACE, flex: "2 1 360px" }}>
          <SectionCard title="Inscription vers première séance" subtitle="Comptes créés sur la période" bare />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            {delay.tiles.map((t) => (
              <StatTile key={t.label} label={t.label} value={t.value} size="sm" basis={120} />
            ))}
          </div>
          {delay.hasData ? (
            <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 14 }}>
              {delay.bars.map((b) => (
                <li key={b.key}>
                  <ProgressBar label={b.name} value={b.value} valueText={b.text} tone={b.tone} />
                </li>
              ))}
            </ul>
          ) : (
            <p style={MUTED_TEXT}>Aucun compte créé sur la période.</p>
          )}
          <p style={NOTE}>
            Écart entre la création du compte et sa première séance. Le délai médian n&apos;est pas exposé par l&apos;API : [À MESURER].
          </p>
        </section>

        <section aria-label="Visiteurs vers inscrits" style={{ ...SURFACE, flex: "1 1 280px" }}>
          <SectionCard title="Visiteurs vers inscrits" badge="À instrumenter" bare />
          <StatTile
            label="Taux d'inscription parmi les visiteurs"
            value={visitorsMissing ? "" : "—"}
            size="sm"
            basis={240}
          />
          <p style={NOTE}>
            Les visiteurs uniques ne sont pas suivis aujourd&apos;hui : seul le nombre d&apos;inscrits existe. Il faut un compteur de visites côté web pour calculer ce taux.
          </p>
        </section>

        <section aria-label="Sources d'acquisition" style={{ ...SURFACE, flex: "1 1 320px" }}>
          <SectionCard title="Sources d'acquisition" badge="À instrumenter" bare />
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 14 }}>
            {SOURCES.map((name) => (
              <li key={name}>
                <ProgressBar label={name} value={0} valueText={sourcesMissing ? "[À MESURER]" : "—"} />
              </li>
            ))}
          </ul>
          <p style={NOTE}>Prévu : enregistrer la source (paramètre UTM ou référent) à l&apos;inscription, dans la table des comptes.</p>
        </section>
      </div>

      <section aria-label="Churn" style={{ ...SURFACE, gap: 20 }}>
        <SectionCard title="Churn" subtitle="Comptes actifs il y a 30 à 60 jours et absents depuis" bare />
        <div style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
          <div style={{ flex: "2 1 420px", minWidth: 0 }}>
            {churn.measured ? (
              <Gauge label="Churn mensuel" value={churn.value} max={100} unit="%" size="lg" height={18} />
            ) : (
              <StatTile label="Churn mensuel" value="" size="lg" basis={240} />
            )}
          </div>
          <div style={{ flex: "3 1 420px", minWidth: 0, display: "flex", flexWrap: "wrap", gap: 12, alignContent: "flex-start" }}>
            {churn.tiles.map((c) => (
              <div key={c.label} style={{ flex: "1 1 150px", minWidth: 0, display: "flex" }}>
                <StatTile
                  label={c.label}
                  value={c.value}
                  hint={c.label === "Objectif de churn" ? "Aucun objectif défini" : undefined}
                  size="sm"
                  basis={150}
                />
              </div>
            ))}
          </div>
        </div>
        <p style={NOTE}>{churn.note}</p>
      </section>
    </>
  );
}

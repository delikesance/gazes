import type { AdminViews } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef } from "@/components/admin/cards";
import { BarChartCard, HBarListCard, LineChartCard } from "@/components/admin/charts";
import { SplitBar, StatTile } from "@/components/admin/ui";
import { rangeLabel } from "./fr-date";
import { LIST_RESET, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  durationBars,
  hourBars,
  leaverRows,
  resumeBlock,
  retention,
  sessionsChart,
  timezoneItems,
  viewsInsights,
  viewsKpis,
  weekdayRows,
} from "./views.logic";

export interface ViewsViewProps {
  views: AdminViews;
  periodDays: number;
  from?: string;
  to?: string;
}

const LEAVER_COLUMNS: ColumnDef[] = [
  { key: "title", label: "Série", type: "text" },
  { key: "ep", label: "Épisode", type: "text", muted: true },
  { key: "pct", label: "Abandon avant 25 %", type: "bar", max: 100, tone: "danger", decimals: 0, unit: "%" },
  { key: "min", label: "Minute médiane d'abandon", type: "number", decimals: 1, unit: "min" },
  { key: "n", label: "Séances", type: "number" },
];

export function ViewsView({ views, periodDays, from, to }: ViewsViewProps) {
  const vs = `vs ${periodDays} j préc.`;
  const kpis = viewsKpis(views);
  const insights = viewsInsights(views);
  const sessions = sessionsChart(views);
  const duration = durationBars(views);
  const ret = retention(views);
  const week = weekdayRows(views);
  const hours = hourBars(views);
  const zones = timezoneItems(views);
  const resume = resumeBlock(views);
  const leavers = leaverRows(views);

  return (
    <>
      <AdminPageHeader eyebrow="Séances de lecture" title="Visionnages" subtitle={rangeLabel(from, to)} />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard
            key={k.label}
            label={k.label}
            value={k.value}
            unit={k.unit}
            delta={k.delta}
            deltaUnit={k.deltaUnit}
            vs={vs}
            series={k.series}
            showSpark={k.series.length > 1}
          />
        ))}
      </section>

      <section aria-label="À regarder" style={SURFACE}>
        <SectionCard title="À regarder" subtitle="Constats chiffrés et pistes d'action" bare />
        {insights.length > 0 ? (
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 10 }}>
            {insights.map((n) => (
              <li key={n.title} style={{ display: "flex" }}>
                <InsightCard level={n.level} title={n.title} text={n.text} surface="tile" />
              </li>
            ))}
          </ul>
        ) : (
          <p style={MUTED_TEXT}>Rien à signaler : pas assez de séances sur la période pour établir un constat.</p>
        )}
      </section>

      <div style={{ display: "flex" }}>
        <LineChartCard
          title="Séances par jour"
          subtitle={sessions.summary}
          series={[
            { label: "Période actuelle", values: sessions.current, tone: "accent" },
            { label: "Période précédente", values: sessions.previous, tone: "muted", style: "dashed" },
          ]}
          xLabels={sessions.labels}
          yMin={0}
          yMax={sessions.top}
          yTickCount={4}
          area
          ariaLabel={`Courbe des séances par jour sur ${periodDays} jours, avec la période précédente en pointillés gris.`}
        />
      </div>

      <div style={WRAP_ROW}>
        <div style={{ flex: "2 1 380px", minWidth: 0, display: "flex", flexWrap: "wrap", alignContent: "flex-start", gap: 16 }}>
          <BarChartCard
            title="Durée de séance"
            subtitle={duration.summary}
            values={duration.values}
            labels={duration.labels}
            subLabels={duration.counts}
            showValues="all"
            unit="%"
            decimals={1}
            height={200}
            ariaLabel={duration.ariaLabel}
            basis={380}
          />
          <div style={{ flex: "1 1 100%", minWidth: 0, display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Durée moyenne" value={kpis[1].value === null ? "" : `${kpis[1].value} min`} basis={120} />
            <StatTile label="Durée médiane" value="" basis={120} />
          </div>
        </div>

        {ret.hasData ? (
          <LineChartCard
            title="Rétention dans l'épisode"
            subtitle="% encore présent, par décile de l'épisode"
            series={[{ label: "Rétention", values: ret.values, tone: "accent" }]}
            xLabels={ret.xLabels}
            yMin={ret.yMin}
            yMax={100}
            yTickCount={4}
            xTickCount={4}
            unit="%"
            decimals={0}
            area
            marks={ret.marks}
            footnote="Les pastilles numérotées marquent les points de décrochage."
            ariaLabel={ret.ariaLabel}
            grow={3}
            basis={460}
          />
        ) : (
          <section aria-label="Rétention dans l'épisode" style={{ ...SURFACE, flex: "3 1 460px" }}>
            <SectionCard title="Rétention dans l'épisode" bare />
            <p style={MUTED_TEXT}>Pas assez de séances sur la période pour tracer la courbe.</p>
          </section>
        )}
      </div>

      <div style={WRAP_ROW}>
        <HBarListCard
          title="Complétion par jour"
          subtitle={week.summary}
          items={week.rows}
          format="decimal"
          unit="%"
          deltaUnit="pt"
          max={100}
          barHeight={8}
          footnote="Écart en points par rapport à la moyenne de la période."
          grow={2}
          basis={340}
        />

        <BarChartCard
          title="Séances par heure"
          subtitle={hours.summary}
          values={hours.values}
          labels={hours.labels}
          labelMode="ends"
          labelEvery={6}
          highlightIndexes={hours.peak}
          highlightLabel="Heures les plus chargées"
          otherLabel="Autres heures"
          unit="%"
          decimals={1}
          height={180}
          footnote="Heure UTC (le fuseau du spectateur n'est pas appliqué)."
          ariaLabel={hours.ariaLabel}
          grow={3}
          basis={460}
        />
      </div>

      <div style={WRAP_ROW}>
        <HBarListCard
          title="Fuseaux horaires"
          subtitle="Décalage tz_offset"
          items={zones}
          format="decimal"
          unit="%"
          emptyText="Aucune séance sur la période."
          footnote="Les pays ne sont pas mesurés aujourd'hui : [À MESURER] via GeoIP."
          grow={2}
          basis={340}
        />

        <div style={{ flex: "2 1 340px", minWidth: 0, display: "flex", flexWrap: "wrap", alignContent: "flex-start", gap: 16 }}>
          <section aria-label="Épisodes par actif" style={{ ...SURFACE, flex: "1 1 340px" }}>
            <SectionCard title="Épisodes par actif" subtitle="Épisodes distincts vus" bare />
            <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
              <StatTile label="Moyenne" value={kpis[3].value === null ? "" : `${kpis[3].value} épisodes`} basis={100} />
              <StatTile label="Médiane" value="" basis={100} />
            </div>
            <p style={MUTED_TEXT}>La répartition par nombre d&apos;épisodes vus et la médiane ne sont pas exposées par l&apos;API : [À MESURER].</p>
          </section>
        </div>

        <section aria-label="Reprises et premières lectures" style={{ ...SURFACE, flex: "2 1 340px" }}>
          <SectionCard title="Reprises et premières lectures" bare />
          {resume.hasData ? (
            <>
              <SplitBar segments={resume.segments} legend="none" height={18} />
              <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
                <StatTile label="Reprises" value={resume.resumePct} hint={resume.resumeHint} swatch="accent" basis={130} />
                <StatTile label="Premières lectures" value={resume.firstPct} hint={resume.firstHint} swatch="light" basis={130} />
              </div>
            </>
          ) : (
            <p style={MUTED_TEXT}>Aucune séance sur la période.</p>
          )}
          <span style={{ fontSize: 12, color: "#a1a1aa" }}>Reprise : séance sur un épisode déjà commencé (table progress).</span>
        </section>
      </div>

      <div style={{ display: "flex" }}>
        <DataTable
          title="Épisodes qui font décrocher"
          subtitle="Abandon avant 25 % de l'épisode, trié par taux"
          columns={LEAVER_COLUMNS}
          rows={leavers}
          rowKey="id"
          sortKey="pct"
          sortDir="desc"
          minWidth={560}
          caption="Épisodes qui font décrocher"
          emptyText="Aucun épisode ne fait décrocher sur la période."
        />
      </div>

      <p style={MUTED_TEXT}>
        Actifs par jour et temps regardé : voir Vue d&apos;ensemble. Dernières séances : voir le détail d&apos;un utilisateur dans Utilisateurs.
      </p>
    </>
  );
}

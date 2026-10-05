import Link from "next/link";
import type { AdminOverview, AdminPlaybackErrors, AdminPlaybackHealth } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { Alert, InsightCard, KpiCard, MediaRow, SectionCard } from "@/components/admin/cards";
import { BarChartCard, HeatmapCard, LineChartCard } from "@/components/admin/charts";
import { StatTile } from "@/components/admin/ui";
import { fmtInt, MISSING, rangeLabel } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  heatmapBlock,
  latestErrors,
  overviewInsights,
  overviewKpis,
  playbackTiles,
  sessionsChart,
  signupsBlock,
  topAnimeRows,
} from "./overview.logic";

export interface OverviewViewProps {
  overview: AdminOverview;
  /** Player block; null when the playback endpoints did not answer. */
  health: AdminPlaybackHealth | null;
  errors: AdminPlaybackErrors | null;
  /** Readable reason shown in the "Lecteur et flux" block when health/errors are missing. */
  playbackNotice?: string;
  periodDays: number;
  from?: string;
  to?: string;
  /** Response timestamp: relative times ("il y a 6 min") are computed against it. */
  generatedAt: string;
}

export function OverviewView({ overview, health, errors, playbackNotice, periodDays, from, to, generatedAt }: OverviewViewProps) {
  const vs = `vs ${periodDays} j préc.`;
  const kpis = overviewKpis(overview);
  const insights = overviewInsights(overview, health);
  const sessions = sessionsChart(overview);
  const signups = signupsBlock(overview);
  const top = topAnimeRows(overview);
  const heat = heatmapBlock(overview);
  const tiles = playbackTiles(health);
  const latest = latestErrors(errors, generatedAt);
  const live = health?.active_sessions;
  const liveValue = live && live.measured && live.value !== null ? live.value : null;

  return (
    <>
      <AdminPageHeader
        eyebrow="Tableau de bord"
        title="Vue d'ensemble"
        subtitle={rangeLabel(from, to)}
        badges={liveValue !== null ? <span className="admin-chip">{fmtInt(liveValue)} séance{liveValue > 1 ? "s" : ""} en cours</span> : null}
      />

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

      {insights.length > 0 ? (
        <section aria-label="À regarder" style={WRAP_ROW}>
          {insights.map((n) => (
            <InsightCard key={n.title} level={n.level} title={n.title} text={n.text} linkLabel={n.linkLabel} href={n.href} />
          ))}
        </section>
      ) : null}

      <div id="visionnages" style={WRAP_ROW}>
        <LineChartCard
          title="Visionnages par jour"
          subtitle={sessions.summary}
          series={[{ label: "Visionnages", values: sessions.values, tone: "accent" }]}
          xLabels={sessions.labels}
          yMin={0}
          yMax={sessions.top}
          yTickCount={4}
          area
          ariaLabel={`Courbe des visionnages par jour sur ${periodDays} jours.`}
          grow={2}
          basis={520}
        />

        <section aria-label="En ce moment" style={{ ...SURFACE, flex: "1 1 320px" }}>
          <SectionCard title="En ce moment" badge="Direct" bare />
          <p style={MUTED_TEXT}>
            {liveValue !== null
              ? `${fmtInt(liveValue)} séance${liveValue > 1 ? "s" : ""} en cours de lecture.`
              : `Séances en cours : ${MISSING}.`}
          </p>
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 4 }}>
            <li>
              <MediaRow variant="live" title="Détail par anime" meta="Aucun endpoint de séances en cours par anime" value={MISSING} />
            </li>
          </ul>
        </section>
      </div>

      <div style={WRAP_ROW}>
        <section id="animes" aria-label="Animes les plus regardés" style={{ ...SURFACE, flex: "3 1 460px" }}>
          <SectionCard title="Animes les plus regardés" subtitle="Visionnages sur la période" bare />
          {top.length > 0 ? (
            <ol style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 6 }}>
              {top.map((a, i) => (
                <li key={`${a.title}-${a.rank}`}>
                  <MediaRow variant="ranked" rank={a.rank} title={a.title} meta={a.meta} value={a.value} valueLabel="visionnages" bar={a.bar} trend={a.trend} tint={i} />
                </li>
              ))}
            </ol>
          ) : (
            <p style={MUTED_TEXT}>Aucune séance sur la période.</p>
          )}
        </section>

        <div id="inscrits" style={{ flex: "2 1 340px", minWidth: 0, display: "flex", flexWrap: "wrap", alignContent: "flex-start", gap: 16 }}>
          <BarChartCard
            title="Inscriptions"
            subtitle={`${fmtInt(signups.total)} sur la période`}
            values={signups.values}
            startLabel={signups.firstDay}
            endLabel={signups.lastDay}
            labelMode="ends"
            height={200}
            ariaLabel={`Inscriptions par jour sur ${periodDays} jours, ${fmtInt(signups.total)} au total.`}
            basis={340}
          />
          <div style={{ flex: "1 1 100%", minWidth: 0, display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Meilleur jour" value={signups.best} size="sm" basis={120} />
            <StatTile label="Inscrits ayant déjà regardé" value="" size="sm" basis={120} />
          </div>
        </div>
      </div>

      <div id="audience" style={WRAP_ROW}>
        <HeatmapCard
          title="Heures de pointe"
          subtitle="Heure UTC"
          rowLabels={heat.rowLabels}
          colLabels={heat.colLabels}
          values={heat.values}
          colLabelEvery={6}
          ariaLabel={heat.ariaLabel}
          footnote="Genres, langues et derniers inscrits : voir Catalogue et Utilisateurs."
          grow={2}
          basis={420}
        />
      </div>

      <div id="infra" style={WRAP_ROW}>
        <section aria-label="Lecteur et flux" style={{ ...SURFACE, flex: "2 1 340px" }}>
          <SectionCard title="Lecteur et flux" bare />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Séances actives" value={tiles.activeSessions} basis={100} />
            <StatTile label="Démarrage médian" value={tiles.startup} basis={100} />
            <StatTile label="Erreurs de lecture" value={tiles.errorRate} basis={100} />
          </div>
          {playbackNotice ? <p style={MUTED_TEXT} role="status">{playbackNotice}</p> : null}
          <span style={MONO_LABEL}>Dernières erreurs</span>
          {latest.length > 0 ? (
            <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 10 }}>
              {latest.map((e) => (
                <li key={e.key}>
                  <Alert code={e.code} text={e.text} time={e.when} />
                </li>
              ))}
            </ul>
          ) : (
            <p style={MUTED_TEXT}>{errors ? "Aucune erreur de lecture enregistrée." : `Dernières erreurs : ${MISSING}.`}</p>
          )}
          <Link
            href="/admin/playback"
            className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#fafafa]"
            style={{ display: "inline-flex", alignItems: "center", minHeight: 44, fontSize: 13, fontWeight: 500, color: "#fafafa" }}
          >
            Ouvrir Lecteur et flux
          </Link>
        </section>
      </div>
    </>
  );
}

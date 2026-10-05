"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  AdminCosts,
  AdminErrorsSummary,
  AdminIssuesList,
  AdminPlaybackErrors,
  AdminPlaybackHealth,
  AdminSources,
} from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef, Row } from "@/components/admin/cards";
import { ALL_TAB } from "@/components/admin/cards/cards.logic";
import { HBarListCard, LineChartCard } from "@/components/admin/charts";
import { Chip, StatTile } from "@/components/admin/ui";
import { fmtInt, MISSING, rangeLabel } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  allFindings,
  cacheBlock,
  COPY_COLUMN,
  copyText,
  dailyTrend,
  errorsByCode,
  journalRows,
  journalTabs,
  notMeasured,
  playbackKpis,
  probableCauses,
  sourceRows,
  type CopyState,
} from "./playback.logic";

export interface PlaybackViewProps {
  health: AdminPlaybackHealth;
  errors: AdminPlaybackErrors;
  summary: AdminErrorsSummary;
  sources: AdminSources;
  /** Optional: needs the metrics:read scope. */
  costs: AdminCosts | null;
  /** Optional: constats recorded through the issues endpoint. */
  issues: AdminIssuesList | null;
  /** Readable messages for optional endpoints that failed. */
  notices?: string[];
  periodDays: number;
  from?: string;
  to?: string;
}

const JOURNAL_COLUMNS: ColumnDef[] = [
  { key: "time", label: "Dernière occurrence", type: "mono", sortable: false },
  { key: "code", label: "Code", type: "chip", sortable: false },
  { key: "show", label: "Anime / épisode", type: "text", sortable: false },
  { key: "src", label: "Source", type: "mono", sortable: false },
  { key: "n", label: "Occurrences", type: "number", decimals: 0, sortable: false },
  { key: COPY_COLUMN, label: "Contexte", type: "button", align: "right", sortable: false },
];

const SOURCE_COLUMNS: ColumnDef[] = [
  { key: "source", label: "Source", type: "mono", sortable: false },
  { key: "failures", label: "Échecs", type: "number", sortable: false },
  { key: "share", label: "Part des échecs", type: "bar", max: 100, decimals: 1, unit: "%", sortable: false },
  { key: "delta", label: "Évolution", type: "delta", decimals: 0, goodWhen: "down", sortable: false },
  { key: "last", label: "Dernière occurrence", type: "mono", sortable: false },
];

const COPIED_RESET_MS = 2200;

export function PlaybackView({ health, errors, summary, sources, costs, issues, notices = [], periodDays, from, to }: PlaybackViewProps) {
  const vs = `vs ${periodDays} j préc.`;
  const kpis = playbackKpis(health, summary, costs);
  const findings = allFindings(health, summary, sources, issues, periodDays);
  const byCode = errorsByCode(summary);
  const trend = dailyTrend(summary);
  const cache = cacheBlock(health);
  const causes = probableCauses(summary);
  const gaps = notMeasured(health, sources, costs);
  const sourceData = useMemo(() => sourceRows(sources), [sources]);
  const live = health.active_sessions;

  // "Copier le contexte": clipboard access can be refused (permissions, insecure context): always report the outcome.
  const [copy, setCopy] = useState<CopyState>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  const journal = useMemo(() => journalRows(errors.items, copy), [errors.items, copy]);
  const tabs = useMemo(() => journalTabs(errors.items, ALL_TAB), [errors.items]);

  const settle = useCallback((id: string, status: "copied" | "failed") => {
    setCopy({ id, status });
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopy(null), COPIED_RESET_MS);
  }, []);

  const onCellClick = useCallback(
    (row: Row, key: string) => {
      if (key !== COPY_COLUMN) return;
      const id = String(row.id);
      const index = journal.findIndex((r) => r.id === id);
      const item = index >= 0 ? errors.items[index] : undefined;
      if (!item) return;
      const text = copyText(item, summary);
      try {
        navigator.clipboard.writeText(text).then(() => settle(id, "copied"), () => settle(id, "failed"));
      } catch {
        settle(id, "failed");
      }
    },
    [errors.items, journal, settle, summary],
  );

  const journalSubtitle = errors.page.total > errors.items.length
    ? `${fmtInt(errors.items.length)} plus récents sur ${fmtInt(errors.page.total)} groupes d'erreurs`
    : `${fmtInt(errors.page.total)} groupe${errors.page.total > 1 ? "s" : ""} d'erreurs`;

  return (
    <>
      <AdminPageHeader
        eyebrow="Dépannage"
        title="Lecteur et flux"
        subtitle={rangeLabel(from, to)}
        badges={live.measured && live.value !== null ? <span className="admin-chip">{fmtInt(live.value)} séance{live.value > 1 ? "s" : ""} en cours</span> : null}
      />

      {notices.length > 0 ? (
        <section role="status" aria-label="Mesures indisponibles" style={{ ...SURFACE, gap: 6 }}>
          {notices.map((n) => (
            <p key={n} style={MUTED_TEXT}>{n}</p>
          ))}
        </section>
      ) : null}

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard
            key={k.label}
            label={k.label}
            value={k.value}
            unit={k.unit}
            delta={k.delta}
            goodWhen={k.goodDown ? "down" : "up"}
            vs={vs}
            note={k.note}
            tag={k.tag}
            series={k.series}
            showSpark={(k.series?.length ?? 0) > 1}
            basis={200}
          />
        ))}
      </section>

      <section aria-label="À regarder" style={SURFACE}>
        <SectionCard title="À regarder" subtitle="Constats chiffrés et actionnables" bare />
        {findings.length > 0 ? (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            {findings.map((f) => (
              <InsightCard key={f.title} surface="tile" level={f.level} title={f.title} text={f.text} action={f.action} basis={280} />
            ))}
          </div>
        ) : (
          <p style={MUTED_TEXT}>Rien à signaler sur la période.</p>
        )}
      </section>

      <div style={WRAP_ROW}>
        <HBarListCard
          title="Erreurs par code"
          subtitle={`${fmtInt(summary.total.value)} erreur${summary.total.value > 1 ? "s" : ""} sur ${periodDays} jours`}
          items={byCode}
          sort="desc"
          barHeight={8}
          emptyText="Aucune erreur de lecture sur la période."
          grow={3}
          basis={420}
        />
        <LineChartCard
          title="Tendance quotidienne"
          subtitle={trend.maxText}
          series={[{ label: "Erreurs par jour", values: trend.values, tone: "accent" }]}
          xLabels={trend.labels}
          yMin={0}
          xTickCount={3}
          area
          height={160}
          ariaLabel={trend.ariaLabel}
          grow={2}
          basis={300}
        />
      </div>

      <div style={WRAP_ROW}>
        <DataTable
          id="journal"
          title="Journal d'erreurs récentes"
          subtitle={journalSubtitle}
          columns={JOURNAL_COLUMNS}
          rows={journal}
          rowKey="id"
          searchable
          searchLabel="Rechercher dans le journal"
          searchPlaceholder="Anime, source ou code"
          searchKeys={["code", "show", "src"]}
          tabs={tabs}
          tabKey="code"
          tabAll={false}
          tabsLabel="Filtrer par code"
          onCellClick={onCellClick}
          emptyText="Aucune erreur récente pour ce filtre."
          minWidth={760}
          caption="Journal d'erreurs récentes"
          footnote={`Erreurs groupées par code, anime, épisode et source depuis les ${periodDays} derniers jours.`}
        />
        <p role="status" aria-live="polite" style={{ ...MUTED_TEXT, flex: "1 1 100%", minHeight: 18 }}>
          {copy ? (copy.status === "copied" ? "Contexte copié dans le presse-papiers." : "La copie a échoué : le navigateur a refusé l'accès au presse-papiers.") : ""}
        </p>
      </div>

      <div style={WRAP_ROW}>
        <DataTable
          id="sources"
          title="Pires sources et trackers"
          subtitle={`${fmtInt(sources.total_failures)} échec${sources.total_failures > 1 ? "s" : ""} · sources actives : ${MISSING}`}
          columns={SOURCE_COLUMNS}
          rows={sourceData}
          rowKey="id"
          minWidth={520}
          caption="Pires sources et trackers"
          emptyText="Aucune source en échec sur la période."
          footnote="Taux d'échec par source : [À MESURER] (les tentatives ne sont pas comptées)."
          grow={1}
          basis={440}
        />

        <section aria-label="Cache de diagnostic" style={{ ...SURFACE, flex: "1 1 440px" }}>
          <SectionCard title="Cache de diagnostic" subtitle={cache.subtitle} bare />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            {cache.tiles.map((c) => (
              <StatTile key={c.label} label={c.label} value={c.value} size="sm" basis={130} />
            ))}
          </div>
          <p style={MUTED_TEXT}>Les évictions et le taux de succès du cache ne sont pas exposés aujourd&apos;hui.</p>
        </section>
      </div>

      <section aria-label="Pas encore mesuré" style={SURFACE}>
        <SectionCard title="Pas encore mesuré" subtitle="Aucun chiffre inventé : ces mesures n'existent pas dans l'API" bare />
        <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 0 }}>
          {gaps.map((g) => (
            <li
              key={g.label}
              style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: "4px 16px", padding: "12px 0", borderTop: "1px solid rgba(255,255,255,0.07)" }}
            >
              <span style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0, flex: "1 1 260px" }}>
                <span style={{ fontWeight: 500, fontSize: 14 }}>{g.label}</span>
                <span style={{ fontSize: 12, color: "#a1a1aa" }}>{g.reason}</span>
              </span>
              <span style={{ fontFamily: "'Geist Mono', monospace", fontSize: 12, color: "#a1a1aa" }}>{MISSING}</span>
            </li>
          ))}
        </ul>
      </section>

      <section aria-label="Causes probables" style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <SectionCard title="Causes probables" subtitle="Hypothèses à vérifier dans le dépôt" bare />
        {causes.length > 0 ? (
          <div style={WRAP_ROW}>
            {causes.map((c) => (
              <article key={c.code} style={{ ...SURFACE, flex: "1 1 320px", alignItems: "flex-start", gap: 12 }}>
                <Chip tone="code" label={c.code} />
                <span style={MONO_LABEL}>Cause racine probable</span>
                <span style={{ fontSize: 14 }}>{c.probable_cause}</span>
                <span style={{ fontSize: 13, color: "#a1a1aa" }}>{fmtInt(c.count)} occurrence{c.count > 1 ? "s" : ""} sur la période</span>
                {c.files.length > 0 ? (
                  <span style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                    {c.files.map((f) => (
                      <Chip key={f} tone="code" label={f} />
                    ))}
                  </span>
                ) : null}
              </article>
            ))}
          </div>
        ) : (
          <p style={MUTED_TEXT}>Aucune erreur de lecture sur la période : pas de cause à examiner.</p>
        )}
      </section>
    </>
  );
}

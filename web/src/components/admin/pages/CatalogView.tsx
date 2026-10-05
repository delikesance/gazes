"use client";

import { useTransition } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { AdminCatalog } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef } from "@/components/admin/cards";
import { HBarListCard, QuadrantCard } from "@/components/admin/charts";
import { PillGroup, ProgressBar, SplitBar, StatTile } from "@/components/admin/ui";
import { rangeLabel } from "./fr-date";
import { LIST_RESET, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  CATALOG_FORMATS,
  catalogInsights,
  catalogKpis,
  formatName,
  genreRows,
  langName,
  leavers,
  matrix,
  seasonCards,
  shareItems,
  topNote,
  topRows,
  topSummary,
  totalSessions,
} from "./catalog.logic";
import type { CatalogFormat } from "./catalog.logic";
import { fmtDec } from "./fr-date";

export interface CatalogViewProps {
  /** Answer for every format: KPIs, matrix, genres, languages and new series come from it. */
  all: AdminCatalog;
  /** Answer for the selected format tab (the same object as `all` for "Tous"). */
  tab: AdminCatalog;
  format: CatalogFormat;
  from?: string;
  to?: string;
}

const TOP_COLUMNS: ColumnDef[] = [
  { key: "rank", label: "Rang", type: "mono", width: "64px" },
  { key: "title", label: "Anime", type: "thumb" },
  { key: "format", label: "Format", type: "text", muted: true },
  { key: "share", label: "Part des séances", type: "bar", decimals: 1, unit: "%" },
  { key: "sessions", label: "Séances", type: "number" },
  { key: "hours", label: "Heures", type: "number", decimals: 1, unit: "h" },
  { key: "completion", label: "Complétion", type: "number", decimals: 1, unit: "%" },
  { key: "fresh", label: "Nouveaux spectateurs", type: "number" },
  { key: "delta", label: "Variation des séances", type: "delta", decimals: 1, unit: "%" },
];

const LEAVER_COLUMNS: ColumnDef[] = [
  { key: "title", label: "Série", type: "text" },
  { key: "format", label: "Format", type: "text", muted: true },
  { key: "abandon", label: "Abandon", type: "bar", max: 100, tone: "danger", decimals: 1, unit: "%" },
  { key: "sessions", label: "Séances", type: "number" },
];

export function CatalogView({ all, tab, format, from, to }: CatalogViewProps) {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const [pending, startTransition] = useTransition();

  const chooseFormat = (id: string | number) => {
    const next = new URLSearchParams(search.toString());
    if (id === "all") next.delete("format");
    else next.set("format", String(id));
    const qs = next.toString();
    startTransition(() => router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false }));
  };

  const total = totalSessions(all);
  const kpis = catalogKpis(all);
  const insights = catalogInsights(all);
  const rows = topRows(tab, total).map((r) => ({ ...r }));
  const topMax = Math.max(1, ...rows.map((r) => r.share));
  const m = matrix(all);
  const lv = leavers(all);
  const genres = genreRows(all);
  const season = seasonCards(all);

  return (
    <>
      <AdminPageHeader eyebrow="Animes et bibliothèque" title="Catalogue" subtitle={rangeLabel(from, to)} />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard key={k.label} label={k.label} value={k.value} note={k.note} tag={k.tag} showSpark={false} />
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

      <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
        <div style={{ flex: "1 1 100%", minWidth: 0, display: "flex", justifyContent: "flex-end" }}>
          <PillGroup
            ariaLabel="Format"
            options={CATALOG_FORMATS.map((f) => ({ id: f.id, label: f.label }))}
            value={format}
            onChange={chooseFormat}
          />
        </div>
        <div aria-busy={pending || undefined} style={{ flex: "1 1 100%", minWidth: 0, display: "flex", opacity: pending ? 0.6 : 1 }}>
          <DataTable
            id="top-animes"
            title="Top animes"
            subtitle={topSummary(tab, all)}
            columns={TOP_COLUMNS.map((c) => (c.key === "share" ? { ...c, max: topMax } : c))}
            rows={rows}
            rowKey="id"
            sortKey="sessions"
            sortDir="desc"
            minWidth={820}
            caption={`Top animes, format : ${CATALOG_FORMATS.find((f) => f.id === format)?.label ?? format}`}
            emptyText="Aucun anime regardé sur la période pour ce format."
            footnote={topNote(tab)}
          />
        </div>
      </div>

      <div style={{ display: "flex" }}>
        {m.points.length > 0 ? (
          <QuadrantCard
            title="Popularité et complétion"
            subtitle={m.summary}
            points={m.points}
            xLabel="part des séances de la période"
            yLabel="taux de complétion"
            xUnit="%"
            yUnit="%"
            xDecimals={0}
            yDecimals={0}
            xMin={0}
            xMax={m.xMax}
            yMin={m.yMin}
            yMax={m.yMax}
            xThreshold={m.xThreshold}
            yThreshold={m.yThreshold}
            quadrantRules={m.rules}
            height={440}
          />
        ) : (
          <section aria-label="Popularité et complétion" style={{ ...SURFACE, flex: 1 }}>
            <SectionCard title="Popularité et complétion" bare />
            <p style={MUTED_TEXT}>Aucune séance sur la période : la matrice ne peut pas être tracée.</p>
          </section>
        )}
      </div>

      <div style={WRAP_ROW}>
        <DataTable
          id="fort-abandon"
          title="Séries à fort abandon"
          subtitle="Au moins 35 % de séances non terminées"
          columns={LEAVER_COLUMNS}
          rows={lv.map((l) => ({ ...l }))}
          rowKey="id"
          sortKey="abandon"
          sortDir="desc"
          minWidth={420}
          grow={3}
          basis={460}
          emptyText="Aucune série ne dépasse le seuil d'abandon."
          footnote="Abandon = séances non terminées. Seules les séries avec au moins 5 séances et 0,5 % des séances de la période comptent."
        />

        <section aria-label="Genres" style={{ ...SURFACE, flex: "2 1 340px" }}>
          <SectionCard title="Genres" bare />
          {genres.length > 0 ? (
            <>
              <div style={{ display: "flex", flexWrap: "wrap", gap: "6px 16px", fontSize: 12, color: "#a1a1aa" }}>
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <span aria-hidden="true" style={{ width: 10, height: 10, borderRadius: 3, background: "#9b8afb" }} />
                  Part des visionnages
                </span>
                <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                  <span aria-hidden="true" style={{ width: 10, height: 10, borderRadius: 3, background: "#fafafa" }} />
                  Complétion
                </span>
              </div>
              <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 14 }}>
                {genres.map((g) => (
                  <li key={g.name} style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                    <span style={{ display: "flex", justifyContent: "space-between", fontSize: 13 }}>
                      <span>{g.name}</span>
                      <span style={{ fontVariantNumeric: "tabular-nums", color: "#a1a1aa" }}>
                        {fmtDec(g.share, 1)} % · {fmtDec(g.completion, 1)} %
                      </span>
                    </span>
                    <ProgressBar layout="bar" tone="accent" value={g.shareBar} label={`${g.name}, part des visionnages`} />
                    <ProgressBar layout="bar" tone="light" value={g.completionBar} label={`${g.name}, complétion`} />
                  </li>
                ))}
              </ul>
              <p style={MUTED_TEXT}>Un anime compte dans chacun de ses genres : les parts peuvent dépasser 100 % au total.</p>
            </>
          ) : (
            <p style={MUTED_TEXT}>Aucun genre renseigné sur la période.</p>
          )}
        </section>
      </div>

      <div style={WRAP_ROW}>
        <HBarListCard
          title="Formats"
          subtitle="Part des séances"
          items={shareItems(all.formats, formatName)}
          format="decimal"
          unit="%"
          emptyText="Aucune séance sur la période."
          basis={280}
        />
        <HBarListCard
          title="Langues audio"
          subtitle="Part des séances"
          items={shareItems(all.audio_langs, langName)}
          format="decimal"
          unit="%"
          emptyText="Aucune séance sur la période."
          basis={280}
        />
        <HBarListCard
          title="Langues des sous-titres"
          subtitle="Part des séances"
          items={shareItems(all.sub_langs, langName)}
          format="decimal"
          unit="%"
          emptyText="Aucune séance sur la période."
          basis={280}
        />
      </div>

      <section aria-label="Nouveautés et fonds de catalogue" style={{ ...SURFACE, gap: 20 }}>
        <SectionCard title="Nouveautés et fonds de catalogue" subtitle="Part des séances de la période" bare />
        {season.hasData ? (
          <>
            <SplitBar segments={season.segments} legend="none" height={18} />
            <div style={WRAP_ROW}>
              {season.cards.map((c) => (
                <div key={c.name} style={{ flex: "1 1 280px", minWidth: 0, display: "flex", flexDirection: "column", gap: 12 }}>
                  <h3 style={{ margin: 0, display: "inline-flex", alignItems: "center", gap: 8, fontWeight: 500, fontSize: 15 }}>
                    <span aria-hidden="true" style={{ width: 10, height: 10, borderRadius: 3, background: c.swatch === "accent" ? "#9b8afb" : "#fafafa" }} />
                    {c.name}
                  </h3>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
                    <StatTile label="Séries" value={c.series} basis={80} />
                    <StatTile label="Part des séances" value={c.share} basis={80} />
                    <StatTile label="Séances" value={c.sessions} basis={80} />
                    <StatTile label="Séances par série" value={c.perSeries} basis={80} />
                  </div>
                </div>
              ))}
            </div>
          </>
        ) : (
          <p style={MUTED_TEXT}>Aucune séance sur la période.</p>
        )}
        <span style={{ fontSize: 13, color: "#a1a1aa" }}>{season.note} La complétion par groupe n&apos;est pas exposée : [À MESURER].</span>
      </section>

      <div style={WRAP_ROW}>
        <section aria-label="Demandes sans source disponible" style={{ ...SURFACE, flex: "3 1 460px" }}>
          <SectionCard title="Demandes sans source disponible" badge="À instrumenter" badgeTone="neutral" bare />
          <p style={MUTED_TEXT}>
            Aucune table n&apos;enregistre les séries demandées sans source. Les échecs de lecture par source sont dans Lecteur et flux.
          </p>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Séries sans source" value="" basis={140} />
            <StatTile label="Tentatives" value="" basis={140} />
            <StatTile label="Dernier échec" value="" basis={140} />
          </div>
        </section>

        <section aria-label="Copies AV1" style={{ ...SURFACE, flex: "2 1 340px" }}>
          <SectionCard title="Copies AV1" badge="À instrumenter" badgeTone="neutral" bare />
          <p style={MUTED_TEXT}>Le statut des copies AV1 (prêtes, en file, en échec) n&apos;est pas exposé par l&apos;API admin.</p>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Prêtes" value="" basis={100} />
            <StatTile label="En file" value="" basis={100} />
            <StatTile label="En échec" value="" basis={100} />
          </div>
          <span style={{ fontSize: 12, color: "#a1a1aa" }}>Économie de stockage : [À MESURER].</span>
        </section>
      </div>

      <section aria-label="Recherches sans résultat" style={SURFACE}>
        <SectionCard title="Recherches sans résultat" badge="À instrumenter" badgeTone="neutral" bare />
        <p style={MUTED_TEXT}>
          Aucune table ne journalise les recherches aujourd&apos;hui. Pour mesurer ce besoin, enregistrer la requête, le nombre de résultats et l&apos;heure côté API.
        </p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
          <StatTile label="Recherches par jour" value="" basis={160} />
          <StatTile label="Part sans résultat" value="" basis={160} />
          <StatTile label="Requête la plus fréquente" value="" basis={160} />
        </div>
      </section>
    </>
  );
}

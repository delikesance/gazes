"use client";

import { useEffect, useRef, useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { AdminApiError, adminWrite } from "@/lib/admin/api";
import type { AdminUserDetail, AdminUsersList, AdminUsersSummary } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, EmptyState, InsightCard, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef, Row } from "@/components/admin/cards";
import { HBarListCard } from "@/components/admin/charts";
import { Button, Chip, ProgressBar, StatTile } from "@/components/admin/ui";
import { dateTimeFr, fmtDec, fmtInt } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  USERS_PAGE_SIZE,
  detailTiles,
  directoryRow,
  directoryStatus,
  historyItems,
  pageCount,
  progressItems,
  segmentRows,
  seniorityItems,
  usersInsights,
  usersKpis,
  usersQueryString,
} from "./users.logic";
import type { UsersQuery } from "./users.logic";

export interface UsersViewProps {
  summary: AdminUsersSummary;
  list: AdminUsersList;
  /** Detail of the selected account (null when nothing is selected or the call failed). */
  detail: AdminUserDetail | null;
  /** Readable failure of the detail call, shown in the panel. */
  detailError?: { message: string; code: string } | null;
  query: UsersQuery;
  generatedAt: string;
}

const SEGMENT_COLUMNS: ColumnDef[] = [
  { key: "name", label: "Segment", type: "status", kind: "chip", tones: {} },
  { key: "rule", label: "Règle", type: "text", muted: true },
  { key: "users", label: "Effectif", type: "number" },
  { key: "share", label: "Part des comptes", type: "bar", max: 100, unit: "%", decimals: 1, width: "280px" },
  { key: "hours", label: "Heures regardées par compte", type: "number", unit: "h", decimals: 1 },
];

const DIRECTORY_COLUMNS: ColumnDef[] = [
  { key: "pseudo", label: "Pseudo", type: "text" },
  { key: "user_id", label: "Identifiant", type: "mono" },
  { key: "created_at", label: "Inscrit le", type: "text", muted: true },
  { key: "last_activity", label: "Dernière activité", type: "text", muted: true },
  { key: "sessions", label: "Séances", type: "number" },
  { key: "watch_hours", label: "Heures", type: "number", unit: "h", decimals: 1 },
  { key: "top", label: "Anime le plus vu", type: "text", sortable: false },
  { key: "segment", label: "Segment", type: "status", kind: "chip", sortable: false },
];

const ACTIONS = ["Désactiver le compte", "Réinitialiser la session"];

export function UsersView({ summary, list, detail, detailError, query, generatedAt }: UsersViewProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [pending, startTransition] = useTransition();
  const detailRef = useRef<HTMLElement>(null);
  const [grant, setGrant] = useState<{ userId: number; tone: "ok" | "error"; text: string } | null>(null);
  const [granting, setGranting] = useState(false);

  const grantAdmin = async (userId: number, pseudo?: string) => {
    if (!window.confirm(`Donner les droits d'administrateur à ${pseudo ?? `#${userId}`} ? Il aura accès à tout ce panel.`)) return;
    setGranting(true);
    try {
      await adminWrite("POST", `/users/${userId}/role`, { role: "admin" });
      setGrant({ userId, tone: "ok", text: "Ce compte est maintenant administrateur." });
    } catch (e) {
      setGrant({ userId, tone: "error", text: e instanceof AdminApiError ? `${e.message} (code ${e.code})` : "Erreur inattendue" });
    } finally {
      setGranting(false);
    }
  };
  const reveal = useRef(false);

  const go = (next: UsersQuery) => {
    const qs = usersQueryString(next);
    startTransition(() => router.push(qs ? `${pathname}?${qs}` : pathname, { scroll: false }));
  };

  // After a click on a row, bring the detail panel into view once its content has arrived.
  useEffect(() => {
    if (reveal.current && (detail || detailError)) {
      reveal.current = false;
      detailRef.current?.scrollIntoView({ block: "start", behavior: "smooth" });
      detailRef.current?.focus({ preventScroll: true });
    }
  }, [detail, detailError]);

  const kpis = usersKpis(summary);
  const insights = usersInsights(summary);
  const segments = segmentRows(summary);
  const segmentTable: Row[] = segments.map((s) => ({
    key: s.key,
    name: s.name,
    nameTone: s.nameTone,
    rule: s.rule,
    users: s.users,
    share: s.share,
    hours: s.hours,
  }));
  const rows: Row[] = list.users.map((u) => directoryRow(u, generatedAt));
  const pages = pageCount(list.total, list.limit || USERS_PAGE_SIZE);
  const selectedRow = detail ? list.users.find((u) => u.user_id === detail.user_id) : undefined;
  const tiles = detail ? detailTiles(detail, selectedRow, generatedAt) : [];
  const history = detail ? historyItems(detail, generatedAt) : [];
  const progress = detail ? progressItems(detail) : [];
  const { valid, expiring_7d: expiring } = summary.active_sessions;
  const total = summary.kpis.total.value;
  const detailTitle = detail ? (detail.pseudo ?? `Compte #${detail.user_id}`) : query.user !== null ? `Compte #${query.user}` : "Aucun compte sélectionné";

  const pager = (
    <nav aria-label="Pagination de l'annuaire" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
      <span style={{ fontSize: 13, color: "#a1a1aa" }}>
        Page {fmtInt(query.page)} sur {fmtInt(pages)}
      </span>
      <span style={{ display: "inline-flex", gap: 8 }}>
        <Button variant="secondary" disabled={query.page <= 1 || pending} onClick={() => go({ ...query, page: query.page - 1 })}>
          Page précédente
        </Button>
        <Button variant="secondary" disabled={query.page >= pages || pending} onClick={() => go({ ...query, page: query.page + 1 })}>
          Page suivante
        </Button>
      </span>
    </nav>
  );

  return (
    <>
      <AdminPageHeader
        eyebrow="Comptes"
        title="Utilisateurs"
        subtitle={`Instantané du ${dateTimeFr(generatedAt)} · ${fmtInt(total)} ${total > 1 ? "comptes" : "compte"}`}
        periods={false}
        badges={<Chip label="Aucun e-mail exposé" tone="accent" />}
      />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard key={k.label} label={k.label} value={k.value} note={k.note} delta={k.delta} vs={k.vs} showSpark={false} basis={180} />
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
          <p style={MUTED_TEXT}>Rien à signaler : aucun compte n&apos;est encore inscrit.</p>
        )}
      </section>

      <div style={{ display: "flex" }}>
        <DataTable
          id="segments"
          title="Segments"
          subtitle={`Chaque compte appartient à un seul segment · ${fmtInt(total)} ${total > 1 ? "comptes" : "compte"}`}
          columns={SEGMENT_COLUMNS.map((c) => (c.key === "name" ? { ...c, tones: Object.fromEntries(segments.map((s) => [s.name, s.nameTone])) } : c))}
          rows={segmentTable}
          rowKey="key"
          minWidth={640}
          caption="Segments de comptes : effectif, part et heures regardées par compte"
          footnote="Heures regardées par compte : moyenne du temps total regardé (watch_sessions.watched_seconds) par compte du segment. Les visionnages sans compte ne sont pas comptés ici."
          emptyText="Aucun segment."
        />
      </div>

      <div style={{ display: "flex" }}>
        <DataTable
          id="annuaire"
          title="Annuaire"
          subtitle="Comptes triés selon la colonne choisie, 25 par page"
          columns={DIRECTORY_COLUMNS}
          rows={rows}
          rowKey="rowId"
          manual
          sortKey={query.sort}
          sortDir={query.dir}
          onSortChange={(key, dir) => go({ ...query, sort: key as UsersQuery["sort"], dir, page: 1 })}
          searchable
          searchLabel="Rechercher un pseudo"
          searchPlaceholder="ex. akira"
          searchValue={query.q}
          onSearch={(q) => go({ ...query, q: q.trim(), page: 1 })}
          statusText={directoryStatus(list, query.q !== "")}
          selectable
          selectedKey={query.user === null ? "" : String(query.user)}
          onSelect={(row) => {
            reveal.current = true;
            go({ ...query, user: Number(row.rowId) });
          }}
          busy={pending}
          footer={pager}
          minWidth={960}
          caption="Annuaire des comptes, triable par colonne"
          emptyText={query.q ? "Aucun pseudo ne correspond à cette recherche." : "Aucun compte."}
          footnote="Aucun e-mail n'est exposé par l'API : le compte est identifié par son numéro. Les favoris explicites ne sont pas enregistrés : anime le plus vu à la place."
        />
      </div>

      <section ref={detailRef} tabIndex={-1} aria-label="Détail du compte" style={{ ...SURFACE, gap: 20, outlineOffset: 4 }}>
        <SectionCard title="Détail du compte" subtitle={detailTitle} bare />
        {detailError ? (
          <div role="alert" style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <EmptyState title="Le détail de ce compte n'a pas pu être chargé" text={`${detailError.message} (code : ${detailError.code})`} />
          </div>
        ) : detail ? (
          <>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
              {tiles.map((t) => (
                <div key={t.label} style={{ flex: "1 1 150px", minWidth: 0, display: "flex" }}>
                  <StatTile label={t.label} value={t.value} mono={t.mono} size="sm" basis={150} />
                </div>
              ))}
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
              <div style={{ flex: "1 1 320px", minWidth: 0, display: "flex", flexDirection: "column", gap: 12 }}>
                <h3 style={{ ...MONO_LABEL, margin: 0, fontWeight: 500 }}>Historique récent</h3>
                {history.length > 0 ? (
                  <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column" }}>
                    {history.map((h) => (
                      <li key={h.key} style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 0", borderTop: "1px solid rgba(255,255,255,0.07)" }}>
                        <span style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 2 }}>
                          <span style={{ fontWeight: 500, fontSize: 14 }}>{h.title}</span>
                          <span style={{ fontSize: 12, color: "#a1a1aa" }}>
                            {h.episode} · {h.when}
                          </span>
                        </span>
                        <span style={{ flex: "none", textAlign: "right", display: "flex", flexDirection: "column", gap: 2 }}>
                          <span style={{ fontVariantNumeric: "tabular-nums", fontSize: 13 }}>{h.minutes}</span>
                          <span style={{ fontSize: 12, color: "#a1a1aa" }}>{h.status}</span>
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <EmptyState title="Aucune séance enregistrée pour ce compte." />
                )}
              </div>

              <div style={{ flex: "1 1 320px", minWidth: 0, display: "flex", flexDirection: "column", gap: 12 }}>
                <h3 style={{ ...MONO_LABEL, margin: 0, fontWeight: 500 }}>Progression en cours</h3>
                {progress.length > 0 ? (
                  <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column" }}>
                    {progress.map((p) => (
                      <li key={p.key} style={{ display: "flex", flexDirection: "column", gap: 2, padding: "10px 0", borderTop: "1px solid rgba(255,255,255,0.07)" }}>
                        <span style={{ fontWeight: 500, fontSize: 14 }}>{p.label}</span>
                        <span style={{ fontSize: 12, color: "#a1a1aa" }}>{p.text}</span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <EmptyState title="Aucune progression à reprendre." />
                )}
              </div>

              <div style={{ flex: "1 1 260px", minWidth: 0, display: "flex", flexDirection: "column", gap: 12 }}>
                <h3 style={{ ...MONO_LABEL, margin: 0, fontWeight: 500 }}>Actions admin</h3>
                <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
                  <Button variant="secondary" fullWidth disabled={granting} onClick={() => void grantAdmin(detail.user_id, detail.pseudo)}>
                    Promouvoir administrateur
                  </Button>
                  {grant && grant.userId === detail.user_id ? (
                    <p role="status" style={{ ...MUTED_TEXT, fontSize: 12, color: grant.tone === "error" ? "#f87171" : undefined }}>{grant.text}</p>
                  ) : null}
                  {ACTIONS.map((label) => (
                    <Button key={label} variant="secondary" fullWidth disabled>
                      {label} (à venir)
                    </Button>
                  ))}
                </div>
                <p style={{ ...MUTED_TEXT, fontSize: 12 }}>Actions prévues, non disponibles pour l&apos;instant. Aucun e-mail n&apos;est exposé dans ce panneau.</p>
              </div>
            </div>
          </>
        ) : (
          <EmptyState title="Aucun compte sélectionné." text="Choisir un pseudo dans l'annuaire pour afficher son historique et sa progression." />
        )}
      </section>

      <div style={WRAP_ROW}>
        <HBarListCard
          title="Répartition par ancienneté"
          subtitle="Depuis la date d'inscription"
          items={seniorityItems(summary)}
          format="number"
          emptyText="Aucun compte."
          grow={1}
          basis={380}
        />

        <section aria-label="Sessions actives" style={{ ...SURFACE, flex: "1 1 380px" }}>
          <SectionCard title="Sessions actives" subtitle="Table des sessions" bare />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
            <StatTile label="Sessions valides" value={fmtInt(valid)} size="sm" basis={120} />
            <StatTile label="Expirent sous 7 jours" value={fmtInt(expiring)} size="sm" basis={120} />
            <StatTile label="En ligne (moins de 5 min)" value="" size="sm" basis={120} />
          </div>
          <ProgressBar
            label="Part des sessions qui expirent sous 7 jours"
            value={valid > 0 ? (expiring / valid) * 100 : 0}
            valueText={valid > 0 ? `${fmtInt(expiring)} sur ${fmtInt(valid)} · ${fmtDec((expiring / valid) * 100, 1)} %` : "Aucune session valide"}
            tone={valid > 0 && expiring / valid >= 0.3 ? "danger" : "accent"}
          />
          <p style={MUTED_TEXT}>
            L&apos;échéance fine (moins de 24 h, 24 à 48 h) et le nombre de comptes en ligne ne sont pas exposés par l&apos;API : [À MESURER].
          </p>
        </section>
      </div>
    </>
  );
}

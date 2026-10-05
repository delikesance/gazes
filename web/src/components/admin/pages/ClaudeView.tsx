"use client";

import { useCallback, useMemo, useState } from "react";
import type {
  AdminActions,
  AdminApproval,
  AdminApprovalsList,
  AdminEnvelope,
  AdminKillSwitch,
  AdminMcpAudit,
  AdminMcpTools,
} from "@/lib/admin/types";
import { AdminApiError, adminGet, adminWrite } from "@/lib/admin/api";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, EmptyState, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef, Row, TabDef } from "@/components/admin/cards";
import { Button, Chip, Toggle } from "@/components/admin/ui";
import { dateTimeShortFr, fmtInt, relativeFr } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import {
  approvalStatusLabel,
  argsPreview,
  auditRowToTable,
  connectionCommand,
  countByLevel,
  levelLabel,
  LEVEL_TONES,
  OUTCOME_TONES,
  pendingCount,
  sortApprovals,
  TOOL_LEVELS,
} from "./claude.logic";

export interface ClaudeViewProps {
  tools: AdminMcpTools;
  actions: AdminActions;
  approvals: AdminApprovalsList;
  killSwitch: AdminKillSwitch;
  audit: AdminMcpAudit;
  /** Server clock of the data (ISO), used for relative times. */
  generatedAt: string;
  /** Public base URL of the API, when known, for the connection command. */
  host?: string | null;
  /** Preview mode: nothing is written, buttons explain why. */
  demo?: boolean;
}

const toneOf = (t: string) => (t === "neutral" ? "muted" : t);

const TOOL_COLUMNS: ColumnDef[] = [
  { key: "name", label: "Outil", type: "mono", sortable: true },
  { key: "levelLabel", label: "Niveau", type: "chip", sortable: true },
  { key: "scope", label: "Étendue", type: "mono", muted: true, sortable: true },
  { key: "route", label: "Route admin", type: "mono", muted: true },
  { key: "summary", label: "Rôle", type: "text" },
];

const ACTION_COLUMNS: ColumnDef[] = [
  { key: "name", label: "Action", type: "mono", sortable: true },
  { key: "levelLabel", label: "Niveau", type: "chip", sortable: true },
  { key: "scope", label: "Étendue", type: "mono", muted: true },
  { key: "summary", label: "Effet", type: "text" },
  { key: "stateLabel", label: "État", type: "chip" },
];

const AUDIT_COLUMNS: ColumnDef[] = [
  { key: "ts", label: "Heure", type: "text", sortable: true },
  { key: "tool", label: "Outil", type: "mono" },
  { key: "args", label: "Arguments (masqués)", type: "mono", muted: true },
  { key: "outcomeLabel", label: "Résultat", type: "chip" },
  { key: "duration", label: "Durée", type: "number", unit: " ms", align: "right", sortable: true },
  { key: "token", label: "Jeton", type: "mono", muted: true },
];

const TOOL_TABS: TabDef[] = TOOL_LEVELS.map((l) => ({ label: levelLabel(l), value: l }));

function messageOf(e: unknown): string {
  if (e instanceof AdminApiError) return `${e.message} (code ${e.code})`;
  return "Erreur inattendue";
}

export function ClaudeView({ tools, actions, approvals, killSwitch, audit, generatedAt, host = null, demo = false }: ClaudeViewProps) {
  const [items, setItems] = useState<AdminApproval[]>(approvals.items);
  const [kill, setKill] = useState<AdminKillSwitch>(killSwitch);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<"idle" | "ok" | "ko">("idle");

  const counts = useMemo(() => countByLevel(tools.items), [tools.items]);
  const ordered = useMemo(() => sortApprovals(items), [items]);
  const pending = pendingCount(items);
  const command = connectionCommand(host);

  const refresh = useCallback(async () => {
    const [list, ks] = await Promise.all([
      adminGet<AdminApprovalsList>("/approvals", { params: { limit: 50 } }),
      adminGet<AdminKillSwitch>("/ops/kill-switch"),
    ]);
    setItems(list.data.items);
    setKill(ks.data);
  }, []);

  const run = useCallback(
    async (key: string, write: () => Promise<unknown>) => {
      if (demo) {
        setError("Aperçu : aucune écriture n'est envoyée.");
        return;
      }
      setBusy(key);
      setError(null);
      try {
        await write();
        await refresh();
      } catch (e) {
        setError(messageOf(e));
      } finally {
        setBusy(null);
      }
    },
    [demo, refresh],
  );

  const decide = (id: number, verb: "approve" | "reject" | "undo") =>
    run(`${verb}-${id}`, () => adminWrite<AdminEnvelope<AdminApproval>>("POST", `/approvals/${id}/${verb}`));

  const setSuspended = (suspended: boolean) =>
    run("kill", () => adminWrite<AdminEnvelope<AdminKillSwitch>>("PUT", "/ops/kill-switch", { suspended, reason: suspended ? reason.trim() : "" }));

  const copyCommand = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied("ok");
    } catch {
      setCopied("ko");
    }
  };

  const toolRows: Row[] = tools.items.map((t) => ({
    name: t.name,
    levelLabel: levelLabel(t.level),
    levelLabelTone: toneOf(LEVEL_TONES[t.level] ?? "neutral"),
    level: t.level,
    scope: t.scope,
    route: `${t.method} ${t.path}`,
    summary: t.summary || t.description,
  }));

  const actionRows: Row[] = actions.items.map((a) => ({
    name: a.name,
    levelLabel: levelLabel(a.level),
    levelLabelTone: toneOf(LEVEL_TONES[a.level] ?? "neutral"),
    scope: a.scope,
    summary: a.summary,
    stateLabel: a.implemented ? "Disponible" : "Pas encore implémentée",
    stateLabelTone: a.implemented ? "accent" : "muted",
  }));

  const auditRows: Row[] = audit.items.map((r) => {
    const x = auditRowToTable(r);
    return {
      ...x,
      ts: dateTimeShortFr(x.ts),
      outcomeLabelTone: toneOf(OUTCOME_TONES[x.outcome] ?? "neutral"),
    };
  });
  const auditTabs: TabDef[] = [
    { label: "OK", value: "ok" },
    { label: "Erreurs", value: "error" },
    { label: "Refusés", value: "denied" },
  ];

  return (
    <>
      <AdminPageHeader
        eyebrow="Pilotage automatique"
        title="Claude et MCP"
        subtitle="Ce que Claude peut lire et faire sur Gazes, et ce qu'il a fait."
        periods={false}
        badges={
          <>
            <span className="admin-chip">{tools.enabled ? "Serveur MCP activé" : "Serveur MCP non activé"}</span>
            <span className="admin-chip">{kill.suspended ? "Écritures suspendues" : "Écritures actives"}</span>
          </>
        }
      />

      {error ? (
        <section role="alert" style={{ ...SURFACE, gap: 6, boxShadow: "inset 0 0 0 1px #7f1d1d" }}>
          <p style={{ margin: 0, fontSize: 14 }}>{error}</p>
        </section>
      ) : null}

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        <KpiCard label="Outils exposés" value={tools.items.length} note={`${counts.read + counts.diagnostic} de lecture, ${counts.reversible + counts.sensitive} d'action`} showSpark={false} basis={200} />
        <KpiCard label="Approbations en attente" value={pending} note={pending > 0 ? "À décider ci-dessous" : "Rien à décider"} showSpark={false} basis={200} />
        <KpiCard label="Appels journalisés" value={audit.total} note={`${audit.items.length} derniers affichés`} showSpark={false} tool="get_me" basis={200} />
        <KpiCard label="Budget d'actions réversibles" value={actions.reversible_budget_per_hour} unit="/ h par jeton" showSpark={false} basis={200} />
      </section>

      <SectionCard title="Interrupteur d'arrêt" subtitle="Suspend toute écriture faite par jeton (actions, constats) : elle répond 423. Les lectures continuent.">
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, alignItems: "center" }}>
          <Toggle
            label="Suspendre l'écriture par jeton"
            tone="danger"
            showState
            checked={kill.suspended}
            disabled={busy === "kill"}
            onChange={(on) => void setSuspended(on)}
          />
          {!kill.suspended ? (
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa" }}>
              Motif (affiché dans l&apos;état)
              <input
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                maxLength={200}
                placeholder="Ex. appel d'outil suspect"
                style={{ minHeight: 44, padding: "0 14px", borderRadius: 14, border: 0, background: "#17171a", color: "#fafafa", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)", fontFamily: "inherit", fontSize: 14, width: 280, maxWidth: "100%" }}
              />
            </label>
          ) : (
            <p style={MUTED_TEXT}>
              {kill.reason ? `Motif : ${kill.reason}. ` : ""}
              {kill.updated_at ? `Depuis ${relativeFr(kill.updated_at, generatedAt)}${kill.updated_by ? ` par ${kill.updated_by}` : ""}.` : ""}
            </p>
          )}
        </div>
      </SectionCard>

      <SectionCard title="File d'approbations" subtitle="Les actions sensibles demandées par Claude n'ont aucun effet tant qu'un administrateur connecté n'a pas décidé.">
        {ordered.length === 0 ? (
          <EmptyState title="Aucune demande" text="Claude n'a proposé aucune action sensible pour l'instant." />
        ) : (
          <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 12 }}>
            {ordered.map((a) => {
              const isPending = a.status === "pending";
              return (
                <li key={a.id} style={{ borderRadius: 20, background: "#17171a", padding: 20, display: "flex", flexDirection: "column", gap: 10 }}>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
                    <Chip label={a.tool} tone="code" />
                    <Chip label={approvalStatusLabel(a.status)} tone={isPending ? "accent" : a.status === "failed" ? "danger" : "neutral"} />
                    <span style={{ ...MONO_LABEL, marginLeft: "auto" }}>n°{a.id} · {relativeFr(a.created_at, generatedAt)}</span>
                  </div>
                  {a.plan ? <p style={{ margin: 0, fontSize: 15, fontWeight: 500 }}>{a.plan.summary}</p> : null}
                  {a.justification ? <p style={MUTED_TEXT}>Justification : {a.justification}</p> : null}
                  {a.expected_effect ? <p style={MUTED_TEXT}>Effet attendu : {a.expected_effect}</p> : null}
                  <code style={{ fontFamily: "'Geist Mono', monospace", fontSize: 12, color: "#a1a1aa", overflowWrap: "anywhere" }}>{argsPreview(a.args)}</code>
                  {a.decided_by ? <p style={MUTED_TEXT}>Décidée par {a.decided_by}{a.decided_at ? ` ${relativeFr(a.decided_at, generatedAt)}` : ""}.</p> : null}
                  {isPending || a.undoable ? (
                    <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                      {isPending ? (
                        <>
                          <Button variant="primary" icon="check" disabled={busy !== null || kill.suspended} onClick={() => void decide(a.id, "approve")}>Approuver</Button>
                          <Button variant="secondary" icon="x" disabled={busy !== null} onClick={() => void decide(a.id, "reject")}>Refuser</Button>
                        </>
                      ) : (
                        <Button variant="secondary" disabled={busy !== null || kill.suspended} onClick={() => void decide(a.id, "undo")}>Annuler l&apos;action</Button>
                      )}
                      {isPending && kill.suspended ? <p style={MUTED_TEXT}>Approbation impossible tant que les écritures sont suspendues.</p> : null}
                    </div>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </SectionCard>

      <DataTable
        id="mcp-tools"
        title="Catalogue d'outils"
        subtitle={tools.enabled ? "Ce que Claude voit. Un jeton ne voit que les outils de ses étendues." : "Le serveur MCP n'est pas activé : aucun outil n'est exposé."}
        columns={TOOL_COLUMNS}
        rows={toolRows}
        rowKey="name"
        tabs={TOOL_TABS}
        tabKey="level"
        tabsLabel="Niveau d'autonomie"
        searchable
        searchKeys={["name", "summary"]}
        searchLabel="Rechercher un outil"
        minWidth={720}
        emptyText="Aucun outil."
      />

      <DataTable
        id="mcp-actions"
        title="Actions"
        subtitle={`Réversibles : exécutées sur demande, annulables, ${fmtInt(actions.reversible_budget_per_hour)} par heure et par jeton. Sensibles : jamais sans approbation. Une action simulée par défaut (dry_run).`}
        columns={ACTION_COLUMNS}
        rows={actionRows}
        rowKey="name"
        minWidth={720}
        emptyText="Aucune action."
      />

      <DataTable
        id="mcp-audit"
        title="Journal des appels"
        subtitle={`${fmtInt(audit.total)} appel${audit.total > 1 ? "s" : ""} enregistré${audit.total > 1 ? "s" : ""}. Les arguments sensibles sont masqués à l'écriture.`}
        columns={AUDIT_COLUMNS}
        rows={auditRows}
        rowKey="id"
        tabs={auditTabs}
        tabKey="outcome"
        tabsLabel="Résultat"
        minWidth={760}
        maxHeight={520}
        emptyText="Aucun appel pour l'instant."
        footnote={audit.total > audit.items.length ? `Les ${audit.items.length} appels les plus récents sur ${fmtInt(audit.total)}.` : undefined}
      />

      <SectionCard title="Connecter Claude" subtitle="Un jeton se crée uniquement avec la commande locale gazes-admin (jamais depuis le navigateur).">
        <pre style={{ margin: 0, padding: 16, borderRadius: 20, background: "#17171a", fontFamily: "'Geist Mono', monospace", fontSize: 12, lineHeight: 1.5, overflowX: "auto", userSelect: "all" }}>{command}</pre>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "center" }}>
          <Button variant="secondary" icon="copy" onClick={() => void copyCommand()}>
            {copied === "ok" ? "Copié" : copied === "ko" ? "Sélectionnez le texte" : "Copier la commande"}
          </Button>
          <p style={MUTED_TEXT}>Remplacez [HÔTE] par l&apos;adresse publique de l&apos;API et [JETON] par le jeton affiché une seule fois à sa création.</p>
        </div>
      </SectionCard>

      <SectionCard title="Surveillance automatique" badge="À venir" subtitle="Règles de veille évaluées par le serveur et exécutions planifiées de Claude.">
        <EmptyState title="Pas encore disponible" text="Les règles de veille, leur état et le planning des vérifications de Claude arrivent avec le prochain jalon (M5). Rien n'est simulé ici." />
      </SectionCard>
    </>
  );
}

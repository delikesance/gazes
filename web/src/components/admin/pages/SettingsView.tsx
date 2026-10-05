"use client";

import Link from "next/link";
import { useState } from "react";
import type { AdminEnvelope, AdminSettings, AdminThreshold, AdminTokens } from "@/lib/admin/types";
import { AdminApiError, adminWrite } from "@/lib/admin/api";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, SectionCard } from "@/components/admin/cards";
import type { ColumnDef, Row } from "@/components/admin/cards";
import { Button, Chip } from "@/components/admin/ui";
import { dateTimeShortFr, fmtDec, fmtInt, relativeFr } from "./fr-date";
import { LIST_RESET, MONO_LABEL, MUTED_TEXT } from "./styles";
import { parseThresholdInput, ruleLabel, ruleUnit, TOKEN_CLI_COMMANDS, tokenStatusLabel } from "./claude.logic";

export interface SettingsViewProps {
  settings: AdminSettings;
  /** Null when the tokens could not be read (they need an admin session). */
  tokens: AdminTokens | null;
  generatedAt: string;
  demo?: boolean;
}

const TOKEN_COLUMNS: ColumnDef[] = [
  { key: "name", label: "Nom", type: "text", sortable: true },
  { key: "scopes", label: "Étendues", type: "mono", muted: true },
  { key: "created", label: "Créé", type: "text", sortable: true },
  { key: "expires", label: "Expire", type: "text", sortable: true },
  { key: "lastUsed", label: "Dernier usage", type: "text" },
  { key: "statusLabel", label: "État", type: "chip" },
];

const PRIVACY_FACTS = [
  "Un jeton ne reçoit jamais de pseudo ni d'adresse e-mail : l'API ne les lui renvoie pas, l'annuaire des utilisateurs lui est limité à des identifiants.",
  "Aucune adresse e-mail n'est lue par le panel : elles restent chiffrées dans la base des comptes.",
  "Chaque appel de Claude est journalisé, ses arguments sensibles sont masqués.",
  "Une action sensible n'a aucun effet sans l'approbation d'un administrateur connecté ; un jeton ne peut ni approuver, ni annuler, ni lever l'interrupteur d'arrêt.",
  "Les jetons se créent et se révoquent uniquement avec la commande locale gazes-admin.",
];

function messageOf(e: unknown): string {
  return e instanceof AdminApiError ? `${e.message} (code ${e.code})` : "Erreur inattendue";
}

interface ThresholdRowProps {
  t: AdminThreshold;
  demo: boolean;
}

/** One threshold: the value in force, its bounds, and a form that PROPOSES a new value (an approval is created). */
function ThresholdRow({ t, demo }: ThresholdRowProps) {
  const [raw, setRaw] = useState("");
  const [why, setWhy] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ tone: "ok" | "error"; text: string } | null>(null);
  const unit = ruleUnit(t.rule);
  const inputId = `threshold-${t.rule}`;

  const submit = async () => {
    const parsed = parseThresholdInput(raw, t);
    if (!parsed.ok) {
      setMessage({ tone: "error", text: parsed.error });
      return;
    }
    if (why.trim().length < 3) {
      setMessage({ tone: "error", text: "Indiquez une justification." });
      return;
    }
    if (demo) {
      setMessage({ tone: "error", text: "Aperçu : aucune écriture n'est envoyée." });
      return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const res = await adminWrite<AdminEnvelope<{ status: string; approval_id?: number }>>("POST", "/ops/actions/set_alert_threshold", {
        args: { rule: t.rule, value: parsed.value },
        dry_run: false,
        justification: why.trim(),
        expected_effect: `${ruleLabel(t.rule)} : ${fmtDec(t.value, 1)} ${unit} → ${fmtDec(parsed.value, 1)} ${unit}`,
      });
      const id = res?.data?.approval_id;
      setMessage({ tone: "ok", text: id ? `Demande n°${id} créée : à approuver dans « Claude et MCP ».` : "Demande créée : à approuver dans « Claude et MCP »." });
      setRaw("");
      setWhy("");
    } catch (e) {
      setMessage({ tone: "error", text: messageOf(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <li style={{ borderRadius: 20, background: "#17171a", padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "baseline" }}>
        <h3 style={{ margin: 0, fontSize: 16, fontWeight: 500 }}>{ruleLabel(t.rule)}</h3>
        <Chip label={t.rule} tone="code" />
        <span style={{ marginLeft: "auto", fontSize: 24, fontVariantNumeric: "tabular-nums" }}>{fmtDec(t.value, 1)} {unit}</span>
      </div>
      <p style={MUTED_TEXT}>
        Par défaut {fmtDec(t.default, 1)} {unit} · bornes {fmtDec(t.min, 1)} à {fmtDec(t.max, 1)} {unit}
        {t.value === t.default ? "" : " · valeur modifiée"}
      </p>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" }}>
        <label htmlFor={inputId} style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa" }}>
          Nouvelle valeur ({unit})
          <input
            id={inputId}
            inputMode="decimal"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            style={{ minHeight: 44, width: 140, padding: "0 14px", borderRadius: 14, border: 0, background: "#111113", color: "#fafafa", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)", fontFamily: "inherit", fontSize: 14 }}
          />
        </label>
        <label htmlFor={`${inputId}-why`} style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa", flex: "1 1 260px" }}>
          Justification
          <input
            id={`${inputId}-why`}
            value={why}
            maxLength={300}
            onChange={(e) => setWhy(e.target.value)}
            style={{ minHeight: 44, padding: "0 14px", borderRadius: 14, border: 0, background: "#111113", color: "#fafafa", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)", fontFamily: "inherit", fontSize: 14 }}
          />
        </label>
        <Button variant="secondary" disabled={busy} onClick={() => void submit()}>Proposer</Button>
      </div>
      {message ? (
        <p role={message.tone === "error" ? "alert" : "status"} style={{ ...MUTED_TEXT, color: message.tone === "error" ? "#f87171" : "#fafafa" }}>
          {message.tone === "error" ? "Erreur : " : ""}{message.text}
        </p>
      ) : null}
    </li>
  );
}

export function SettingsView({ settings, tokens, generatedAt, demo = false }: SettingsViewProps) {
  const tokenRows: Row[] = (tokens?.items ?? []).map((t) => ({
    id: t.id,
    name: t.name,
    scopes: t.scopes.join(", "),
    created: dateTimeShortFr(t.created_at),
    createdSort: t.created_at,
    expires: dateTimeShortFr(t.expires_at),
    expiresSort: t.expires_at,
    lastUsed: t.last_used_at ? relativeFr(t.last_used_at, generatedAt) : "Jamais",
    statusLabel: tokenStatusLabel(t.status),
    statusLabelTone: t.status === "active" ? "accent" : "danger",
  }));
  const ks = settings.kill_switch;

  return (
    <>
      <AdminPageHeader
        eyebrow="Accès et alertes"
        title="Paramètres"
        subtitle="Seuils d'alerte, jetons d'accès et garanties de confidentialité."
        periods={false}
        badges={<span className="admin-chip">{ks.suspended ? "Écritures suspendues" : "Écritures actives"}</span>}
      />

      <SectionCard title="Seuils d'alerte" subtitle="Un seuil ne change qu'après approbation d'un administrateur : une proposition crée une demande, même venant d'un administrateur.">
        <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 12 }}>
          {settings.thresholds.map((t) => (
            <ThresholdRow key={t.rule} t={t} demo={demo} />
          ))}
        </ul>
        <p style={MUTED_TEXT}>Les demandes en attente se décident dans <Link href="/admin/claude" style={{ color: "#fafafa" }}>Claude et MCP</Link>.</p>
      </SectionCard>

      <SectionCard title="Limites de Claude" subtitle="Garde-fous appliqués à tous les jetons.">
        <dl style={{ margin: 0, display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 12 }}>
          <div style={{ borderRadius: 20, background: "#17171a", padding: 16 }}>
            <dt style={MONO_LABEL}>Actions réversibles</dt>
            <dd style={{ margin: "6px 0 0", fontSize: 22, fontVariantNumeric: "tabular-nums" }}>{fmtInt(settings.reversible_budget_per_hour)} / h par jeton</dd>
          </div>
          <div style={{ borderRadius: 20, background: "#17171a", padding: 16 }}>
            <dt style={MONO_LABEL}>Interrupteur d&apos;arrêt</dt>
            <dd style={{ margin: "6px 0 0", fontSize: 22 }}>{ks.suspended ? "Actif" : "Inactif"}</dd>
            <dd style={{ ...MUTED_TEXT, margin: "4px 0 0" }}>{ks.suspended && ks.reason ? ks.reason : "Se règle dans Claude et MCP."}</dd>
          </div>
        </dl>
      </SectionCard>

      {tokens ? (
        <DataTable
          id="admin-tokens"
          title="Jetons d'accès"
          subtitle="Métadonnées seulement : un jeton n'est affiché qu'une fois, à sa création."
          columns={TOKEN_COLUMNS}
          rows={tokenRows}
          rowKey="id"
          minWidth={720}
          emptyText="Aucun jeton. Créez-en un avec la commande ci-dessous."
        />
      ) : (
        <SectionCard title="Jetons d'accès" subtitle="La liste n'a pas pu être chargée." />
      )}

      <SectionCard title="Créer ou révoquer un jeton" subtitle="Uniquement en ligne de commande, sur la machine du serveur : aucune route HTTP ne le permet.">
        <pre style={{ margin: 0, padding: 16, borderRadius: 20, background: "#17171a", fontFamily: "'Geist Mono', monospace", fontSize: 12, lineHeight: 1.7, overflowX: "auto", userSelect: "all" }}>{TOKEN_CLI_COMMANDS.join("\n")}</pre>
        <p style={MUTED_TEXT}>Étendues : metrics:read (agrégats), diagnostics:read (lecteur, erreurs, constats), ops:write (actions réversibles), config:write (actions sensibles, soumises à approbation).</p>
      </SectionCard>

      <SectionCard title="Confidentialité" subtitle="Ce que garantit le serveur, pas un réglage.">
        <ul style={{ ...LIST_RESET, display: "flex", flexDirection: "column", gap: 8 }}>
          {PRIVACY_FACTS.map((f) => (
            <li key={f} style={{ fontSize: 14, lineHeight: 1.5 }}>{f}</li>
          ))}
        </ul>
      </SectionCard>
    </>
  );
}

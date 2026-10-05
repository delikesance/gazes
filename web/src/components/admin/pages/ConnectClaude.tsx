"use client";

import { useState, useSyncExternalStore } from "react";
import type { AdminEnvelope, AdminTokenCreated } from "@/lib/admin/types";
import { AdminApiError, adminWrite } from "@/lib/admin/api";
import { SectionCard } from "@/components/admin/cards";
import { Button } from "@/components/admin/ui";
import { MONO_LABEL, MUTED_TEXT } from "./styles";
import { dateTimeShortFr } from "./fr-date";
import { isInsecureBase, mcpAddCommand, mcpJsonConfig, normalizeBase, SCOPE_CHOICES, TTL_CHOICES } from "./claude.logic";

const noop = () => () => {};
const origin = () => (typeof window === "undefined" ? "" : window.location.origin);

const FIELD: React.CSSProperties = { minHeight: 44, padding: "0 14px", borderRadius: 14, border: 0, background: "#17171a", color: "#fafafa", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)", fontFamily: "inherit", fontSize: 14 };
const CODE: React.CSSProperties = { margin: 0, padding: 16, borderRadius: 20, background: "#17171a", fontFamily: "'Geist Mono', monospace", fontSize: 12, lineHeight: 1.5, overflowX: "auto", userSelect: "all" };

export interface ConnectClaudeProps {
  /** Preview mode: no token is created, a harmless example is shown. */
  demo?: boolean;
}

type CopyState = "idle" | "ok" | "ko";

/**
 * "Connecter Claude": creates a token (session only) and builds the ready-to-paste `claude mcp add`
 * command from the site's own address. The token lives in this component's state only: it is shown once,
 * never stored in the browser, and disappears with "Masquer le jeton" or a reload.
 */
export function ConnectClaude({ demo = false }: ConnectClaudeProps) {
  const siteOrigin = useSyncExternalStore(noop, origin, () => "");
  const [baseInput, setBaseInput] = useState<string | null>(null); // null = follow the site address
  const [name, setName] = useState("claude-code");
  const [ttl, setTtl] = useState(TTL_CHOICES[0].hours);
  const [scopes, setScopes] = useState<string[]>(SCOPE_CHOICES.filter((c) => c.defaultOn).map((c) => c.scope));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<AdminTokenCreated | null>(null);
  const [copied, setCopied] = useState<{ which: "command" | "json"; state: CopyState } | null>(null);

  const rawBase = baseInput ?? siteOrigin;
  const base = normalizeBase(rawBase);
  const token = created?.token ?? null;
  const command = mcpAddCommand(base, token);
  const json = mcpJsonConfig(base, token);

  const toggle = (scope: string) => setScopes((cur) => (cur.includes(scope) ? cur.filter((s) => s !== scope) : [...cur, scope]));

  const generate = async () => {
    setError(null);
    if (!name.trim()) return setError("Donnez un nom au jeton.");
    if (scopes.length === 0) return setError("Choisissez au moins une permission.");
    if (!base) return setError("L'adresse du site est invalide (http ou https attendu).");
    if (demo) {
      setCreated({ token: "gzs_EXEMPLE_A_REMPLACER_000000", id: 0, name: name.trim(), scopes, expires_at: "2026-11-04T10:00:00Z" });
      return;
    }
    setBusy(true);
    try {
      const res = await adminWrite<AdminEnvelope<AdminTokenCreated>>("POST", "/tokens", { name: name.trim(), scopes, ttl_hours: ttl });
      setCreated(res.data);
    } catch (e) {
      setError(e instanceof AdminApiError ? `${e.message} (code ${e.code})` : "Erreur inattendue");
    } finally {
      setBusy(false);
    }
  };

  const copy = async (which: "command" | "json", text: string | null) => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied({ which, state: "ok" });
    } catch {
      setCopied({ which, state: "ko" });
    }
  };
  const label = (which: "command" | "json", idle: string) =>
    copied?.which === which ? (copied.state === "ok" ? "Copié" : "Sélectionnez le texte") : idle;

  return (
    <SectionCard
      title="Connecter Claude"
      subtitle="Génère un jeton et la commande prête à coller dans Claude Code. Le jeton n'est affiché qu'une fois : copiez-le tout de suite."
    >
      {!created ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void generate();
          }}
          style={{ display: "flex", flexDirection: "column", gap: 16 }}
        >
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa", flex: "1 1 220px" }}>
              Nom du jeton
              <input value={name} maxLength={60} onChange={(e) => setName(e.target.value)} style={FIELD} />
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa", flex: "1 1 220px" }}>
              Adresse du site
              <input value={rawBase} inputMode="url" spellCheck={false} onChange={(e) => setBaseInput(e.target.value)} style={FIELD} aria-invalid={rawBase !== "" && !base} />
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 13, color: "#a1a1aa" }}>
              Validité
              <select value={ttl} onChange={(e) => setTtl(Number(e.target.value))} style={{ ...FIELD, paddingRight: 8 }}>
                {TTL_CHOICES.map((c) => (
                  <option key={c.hours} value={c.hours}>{c.label}</option>
                ))}
              </select>
            </label>
          </div>
          <fieldset style={{ border: 0, margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
            <legend style={{ ...MONO_LABEL, padding: 0, marginBottom: 8 }}>Permissions</legend>
            {SCOPE_CHOICES.map((c) => (
              <label key={c.scope} style={{ display: "flex", gap: 12, alignItems: "flex-start", minHeight: 44, fontSize: 14 }}>
                <input type="checkbox" checked={scopes.includes(c.scope)} onChange={() => toggle(c.scope)} style={{ width: 20, height: 20, marginTop: 2 }} />
                <span>
                  {c.label} <code style={{ fontFamily: "'Geist Mono', monospace", fontSize: 11, color: "#a1a1aa" }}>{c.scope}</code>
                  <span style={{ display: "block", fontSize: 12, color: "#a1a1aa" }}>{c.hint}</span>
                </span>
              </label>
            ))}
          </fieldset>
          {base && isInsecureBase(base) ? (
            <p role="alert" style={{ ...MUTED_TEXT, color: "#f87171" }}>
              Cette adresse n&apos;est pas en HTTPS : le jeton circulerait en clair vers une autre machine. Utilisez une adresse https.
            </p>
          ) : null}
          {error ? <p role="alert" style={{ ...MUTED_TEXT, color: "#f87171" }}>Erreur : {error}</p> : null}
          <div>
            <Button type="submit" variant="primary" disabled={busy}>Générer le lien</Button>
          </div>
        </form>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <p style={{ ...MUTED_TEXT, color: "#fafafa" }}>
            Jeton « {created.name} » créé : il expire le {dateTimeShortFr(created.expires_at)}. Il ne sera plus jamais affiché.
          </p>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <span style={MONO_LABEL}>Claude Code (dans un terminal)</span>
            {command ? <pre style={CODE}>{command}</pre> : <p role="alert" style={{ ...MUTED_TEXT, color: "#f87171" }}>Adresse invalide : corrigez-la pour obtenir la commande.</p>}
            <div>
              <Button variant="secondary" icon="copy" onClick={() => void copy("command", command)}>{label("command", "Copier la commande")}</Button>
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <span style={MONO_LABEL}>Ou en configuration (.mcp.json)</span>
            {json ? <pre style={CODE}>{json}</pre> : null}
            <div>
              <Button variant="secondary" icon="copy" onClick={() => void copy("json", json)}>{label("json", "Copier la configuration")}</Button>
            </div>
          </div>
          <p style={MUTED_TEXT}>
            Redémarrez Claude Code ensuite : les outils « gazes » apparaissent. Un jeton perdu se révoque dans Paramètres, un nouveau se génère ici.
            {demo ? " (Aperçu : jeton d'exemple, rien n'a été créé.)" : ""}
          </p>
          <div>
            <Button
              variant="ghost"
              onClick={() => {
                setCreated(null);
                setCopied(null);
              }}
            >
              Masquer le jeton
            </Button>
          </div>
        </div>
      )}
    </SectionCard>
  );
}

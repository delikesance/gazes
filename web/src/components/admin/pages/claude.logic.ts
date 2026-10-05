import type { AdminApproval, AdminMcpAuditRow, AdminMcpTool, AdminThreshold, AdminToolLevel, AdminWatchRule, AdminWatchVerdict } from "../../../lib/admin/types";

/** Pure helpers of the "Claude et MCP" and "Paramètres" pages (tested with node --test). */

export const TOOL_LEVELS: ReadonlyArray<AdminToolLevel> = ["read", "diagnostic", "reversible", "sensitive"];

const LEVEL_LABELS: Record<string, string> = {
  read: "Lecture",
  diagnostic: "Diagnostic",
  reversible: "Réversible",
  sensitive: "Sensible",
};

export function levelLabel(level: string): string {
  return LEVEL_LABELS[level] ?? level;
}

/** Chip tone of an autonomy level: sensitive actions stand out, reversible ones are accented. */
export const LEVEL_TONES: Record<string, string> = { read: "neutral", diagnostic: "neutral", reversible: "accent", sensitive: "danger" };

const OUTCOME_LABELS: Record<string, string> = {
  ok: "OK",
  error: "Erreur",
  denied: "Refusé",
  executed: "Exécuté",
  pending: "En attente",
  approved: "Approuvé",
  rejected: "Rejeté",
  undone: "Annulé",
  suspended: "Suspendu",
  budget_exceeded: "Budget dépassé",
  dry_run: "Simulation",
};

export function outcomeLabel(outcome: string): string {
  return OUTCOME_LABELS[outcome] ?? outcome;
}

/** Outcomes that deserve attention are shown in the danger tone (the label carries the meaning too). */
export const OUTCOME_TONES: Record<string, string> = {
  ok: "neutral",
  executed: "accent",
  approved: "accent",
  pending: "accent",
  dry_run: "neutral",
  undone: "neutral",
  rejected: "neutral",
  error: "danger",
  denied: "danger",
  suspended: "danger",
  budget_exceeded: "danger",
};

export function countByLevel(tools: ReadonlyArray<AdminMcpTool>): Record<AdminToolLevel, number> {
  const out: Record<AdminToolLevel, number> = { read: 0, diagnostic: 0, reversible: 0, sensitive: 0 };
  for (const t of tools) if (t.level in out) out[t.level] += 1;
  return out;
}

const STATUS_LABELS: Record<string, string> = {
  pending: "En attente de décision",
  approved: "Approuvée",
  executed: "Exécutée",
  rejected: "Rejetée",
  failed: "Échec",
  undone: "Annulée",
};

export function approvalStatusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status;
}

/** Pending approvals first (oldest first: they have waited longest), then the rest newest first. */
export function sortApprovals(items: ReadonlyArray<AdminApproval>): AdminApproval[] {
  const pending = items.filter((a) => a.status === "pending").sort((a, b) => a.id - b.id);
  const rest = items.filter((a) => a.status !== "pending").sort((a, b) => b.id - a.id);
  return [...pending, ...rest];
}

export function pendingCount(items: ReadonlyArray<AdminApproval>): number {
  return items.filter((a) => a.status === "pending").length;
}

/** Compact JSON of an arguments object, cut at `max` characters. */
export function argsPreview(args: Record<string, unknown> | null | undefined, max = 240): string {
  if (!args) return "{}";
  let text: string;
  try {
    text = JSON.stringify(args);
  } catch {
    return "{…}";
  }
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/** A token as issued by the API: `gzs_` then base64url. Anything else is never put in a command. */
export const TOKEN_RE = /^gzs_[A-Za-z0-9_-]{20,128}$/;

/**
 * Turns what the operator typed (or the site origin) into the base URL of the API: http(s) only, no
 * credentials, query or fragment; a trailing slash or a trailing /mcp is dropped. Null when invalid.
 */
export function normalizeBase(raw: string | null | undefined): string | null {
  const text = (raw ?? "").trim();
  if (!text) return null;
  let u: URL;
  try {
    u = new URL(text);
  } catch {
    return null;
  }
  if ((u.protocol !== "http:" && u.protocol !== "https:") || u.username || u.password || u.search || u.hash) return null;
  const path = u.pathname.replace(/\/+$/, "").replace(/\/mcp$/, "");
  if (!/^[A-Za-z0-9._~%/-]*$/.test(path)) return null; // nothing a shell could interpret
  return `${u.origin}${path}`;
}

/** True when the base would send the token over an unencrypted connection to another machine. */
export function isInsecureBase(base: string): boolean {
  try {
    const u = new URL(base);
    return u.protocol === "http:" && !["localhost", "127.0.0.1", "[::1]"].includes(u.hostname);
  } catch {
    return false;
  }
}

/** The /mcp URL of a base, or null when the base is invalid. */
export function mcpUrl(base: string | null | undefined): string | null {
  const b = normalizeBase(base);
  return b ? `${b}/mcp` : null;
}

/** The Claude Code command. `token` null prints a [JETON] placeholder; an invalid token is refused (null). */
export function mcpAddCommand(base: string | null | undefined, token: string | null): string | null {
  const url = mcpUrl(base);
  if (!url) return null;
  if (token !== null && !TOKEN_RE.test(token)) return null;
  return `claude mcp add --scope user --transport http gazes ${url} --header "Authorization: Bearer ${token ?? "[JETON]"}"`;
}

/** The same server as a .mcp.json / claude_desktop style block. */
export function mcpJsonConfig(base: string | null | undefined, token: string | null): string | null {
  const url = mcpUrl(base);
  if (!url) return null;
  if (token !== null && !TOKEN_RE.test(token)) return null;
  return JSON.stringify({ mcpServers: { gazes: { type: "http", url, headers: { Authorization: `Bearer ${token ?? "[JETON]"}` } } } }, null, 2);
}

export interface ScopeChoice {
  scope: string;
  label: string;
  hint: string;
  defaultOn: boolean;
}

export const SCOPE_CHOICES: ReadonlyArray<ScopeChoice> = [
  { scope: "metrics:read", label: "Lire les statistiques", hint: "visionnages, utilisateurs agrégés, croissance, coûts", defaultOn: true },
  { scope: "diagnostics:read", label: "Lire le lecteur et les constats", hint: "erreurs de lecture, sources, surveillance, constats", defaultOn: true },
  { scope: "ops:write", label: "Agir (réversible)", hint: "créer, annoter et résoudre des constats : annulable, 30 par heure", defaultOn: false },
  { scope: "config:write", label: "Proposer des seuils", hint: "un changement de seuil d'alerte attend votre approbation", defaultOn: false },
];

export const TTL_CHOICES: ReadonlyArray<{ hours: number; label: string }> = [
  { hours: 720, label: "30 jours" },
  { hours: 2160, label: "90 jours" },
  { hours: 8760, label: "1 an" },
];

/** The same operations from the server machine (the panel does them too, for an administrator session). */
export const TOKEN_CLI_COMMANDS = [
  "gazes-admin token create --name claude --scopes metrics:read,diagnostics:read --ttl 720h",
  "gazes-admin token list",
  "gazes-admin token revoke <id>",
] as const;

export function auditRowToTable(row: AdminMcpAuditRow) {
  return {
    id: row.id,
    ts: row.ts,
    tool: row.tool,
    toolTone: "accent",
    args: row.args_summary,
    outcome: row.outcome,
    outcomeLabel: outcomeLabel(row.outcome),
    duration: row.duration_ms,
    token: row.token_id === null ? "—" : `#${row.token_id}`,
  };
}

export type ThresholdParse = { ok: true; value: number } | { ok: false; error: string };

/** Parses a threshold typed by the operator (comma accepted) and checks the rule's bounds. */
export function parseThresholdInput(raw: string, t: Pick<AdminThreshold, "min" | "max">): ThresholdParse {
  const text = raw.trim().replace(",", ".");
  if (text === "") return { ok: false, error: "Saisissez une valeur." };
  if (!/^-?\d+(\.\d+)?$/.test(text)) return { ok: false, error: "Valeur numérique attendue." };
  const value = Number(text);
  if (!Number.isFinite(value)) return { ok: false, error: "Valeur numérique attendue." };
  if (value < t.min || value > t.max) return { ok: false, error: `La valeur doit être entre ${t.min} et ${t.max}.` };
  return { ok: true, value };
}

const RULE_LABELS: Record<string, string> = {
  error_rate_pct: "Taux d'erreur de lecture",
  startup_p95_s: "Démarrage p95",
  stream_saturation_pct: "Saturation des flux",
  disk_pct: "Occupation du disque",
};
const RULE_UNITS: Record<string, string> = { error_rate_pct: "%", startup_p95_s: "s", stream_saturation_pct: "%", disk_pct: "%" };

export function ruleLabel(rule: string): string {
  return RULE_LABELS[rule] ?? rule;
}
export function ruleUnit(rule: string): string {
  return RULE_UNITS[rule] ?? "";
}

/** Human readable token status; the API gives active | expired | revoked. */
export function tokenStatusLabel(status: string): string {
  return ({ active: "Actif", expired: "Expiré", revoked: "Révoqué" } as Record<string, string>)[status] ?? status;
}

// ---- Surveillance automatique ----

const RULE_STATE_LABELS: Record<string, string> = { ok: "OK", near: "Proche du seuil", breached: "Dépassé", not_measured: "Non mesuré" };

export function ruleStateLabel(state: string): string {
  return RULE_STATE_LABELS[state] ?? state;
}

/** Tone of a rule state; the label always carries the meaning as well. */
export const RULE_STATE_TONES: Record<string, string> = { ok: "muted", near: "accent", breached: "danger", not_measured: "muted" };

const WATCH_RULE_LABELS: Record<string, string> = {
  error_rate_pct: "Taux d'erreur de lecture",
  startup_p95_s: "Démarrage p95",
  stream_saturation_pct: "Saturation des flux",
  disk_pct: "Occupation du disque",
  source_failures: "Sources en échec (SRC_DEAD)",
};

export function watchRuleLabel(rule: string, fallback?: string): string {
  return WATCH_RULE_LABELS[rule] ?? fallback ?? rule;
}

/** "13,3 %" / "4 échecs"; null is never turned into 0. */
export function formatWatchValue(value: number | null, unit: string): string {
  if (value === null || !Number.isFinite(value)) return "[À MESURER]";
  const text = value.toLocaleString("fr-FR", { maximumFractionDigits: 1 });
  return unit ? `${text} ${unit}` : text;
}

const VERDICT_LABELS: Record<string, string> = {
  open: "Ouvert",
  pending: "Mesure à +24 h",
  improved: "Amélioré",
  not_improved: "Pas d'amélioration",
};

export function verdictLabel(verdict: AdminWatchVerdict | string): string {
  return VERDICT_LABELS[verdict] ?? verdict;
}

export const VERDICT_TONES: Record<string, string> = { open: "muted", pending: "muted", improved: "accent", not_improved: "danger" };

export function breachedRules(rules: ReadonlyArray<AdminWatchRule>): AdminWatchRule[] {
  return rules.filter((r) => r.state === "breached");
}

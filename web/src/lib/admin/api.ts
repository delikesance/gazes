import { getApiBase } from "@/lib/api";

/** Periods accepted by the admin API (?period=). The server answers 400 for anything else. */
export const ADMIN_PERIODS = [7, 30, 90] as const;
export type AdminPeriod = (typeof ADMIN_PERIODS)[number];
export const DEFAULT_ADMIN_PERIOD: AdminPeriod = 30;

/** Reads a raw `?period=` value (string, string[] or undefined) and falls back to 30 days. */
export function parseAdminPeriod(raw: string | string[] | null | undefined): AdminPeriod {
  const value = Array.isArray(raw) ? raw[0] : raw;
  const days = Number(value);
  return (ADMIN_PERIODS as readonly number[]).includes(days) ? (days as AdminPeriod) : DEFAULT_ADMIN_PERIOD;
}

/** Window actually used by the server: current period and the one right before it. */
export interface AdminPeriodWindow { days: number; from: string; to: string; prev_from: string; prev_to: string }

/** Every admin read answers `{generated_at, period, data}`; `period` is absent on period-less endpoints. */
export interface AdminEnvelope<T> {
  generated_at: string;
  period?: AdminPeriodWindow;
  data: T;
}

/** Failure of an admin call; `code` is the API error code, or "network" / "invalid_response" client-side. */
export class AdminApiError extends Error {
  readonly status: number;
  readonly code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "AdminApiError";
    this.status = status;
    this.code = code;
  }
}

export interface AdminGetOptions {
  period?: AdminPeriod;
  params?: Record<string, string | number | boolean | null | undefined>;
  signal?: AbortSignal;
  /** Server-side only: raw Cookie header to forward (the browser sends its own cookies). */
  cookie?: string;
}

function buildUrl(path: string, period?: AdminPeriod, params?: AdminGetOptions["params"]): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined && value !== null && value !== "") query.set(key, String(value));
  }
  if (period !== undefined) query.set("period", String(period));
  const qs = query.toString();
  return `${getApiBase()}/admin${path.startsWith("/") ? path : `/${path}`}${qs ? `?${qs}` : ""}`;
}

async function parse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => null) as { error?: { code?: string; message?: string } } | null;
    throw new AdminApiError(res.status, body?.error?.code ?? "http_error", body?.error?.message ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  try {
    return (await res.json()) as T;
  } catch {
    throw new AdminApiError(res.status, "invalid_response", "Réponse illisible");
  }
}

async function send<T>(url: string, init: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { credentials: "include", cache: "no-store", ...init });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new AdminApiError(0, "network", "Le serveur est injoignable");
  }
  return parse<T>(res);
}

/** GET /api/v1/admin{path}; usable from server and client components alike. */
export function adminGet<T>(path: string, options: AdminGetOptions = {}): Promise<AdminEnvelope<T>> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.cookie) headers.Cookie = options.cookie;
  return send<AdminEnvelope<T>>(buildUrl(path, options.period, options.params), { method: "GET", headers, signal: options.signal });
}

/** State-changing call; the session cookie requires the X-Gazes-Admin header (CSRF guard of the API). */
export function adminWrite<T = void>(method: "POST" | "PUT" | "PATCH" | "DELETE", path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  return send<T>(buildUrl(path), {
    method,
    headers: { "X-Gazes-Admin": "1", "Content-Type": "application/json", Accept: "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
}

import { sealEnvelope, type KemInfo } from "./auth-crypto";
import { solveCaptcha, type CaptchaChallenge } from "./captcha";

/** `role` is only present for an administrator; it decides what to display, the admin API checks it again. */
export interface AccountUser { id: number; pseudo: string; email?: string; role?: "admin" }

/** Error codes returned by internal/auth/service.go. */
export type AuthErrorCode =
  | "invalid_request" | "invalid_email" | "invalid_pseudo" | "invalid_password"
  | "invalid_credentials" | "email_taken" | "not_found" | "captcha_failed" | "rate_limited"
  | "forbidden" | "unauthorized" | "server_error" | "network";

export class AuthError extends Error {
  constructor(public code: AuthErrorCode) { super(code); }
}

function base() { return process.env.NEXT_PUBLIC_API_BASE || "/api/v1"; }

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${base()}${path}`, { credentials: "same-origin", cache: "no-store", ...init });
  } catch {
    throw new AuthError("network");
  }
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new AuthError((body?.error as AuthErrorCode) || "server_error");
  return body as T;
}

const json = (body: unknown): RequestInit => ({ method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

export const fetchChallenge = () => request<CaptchaChallenge>("/auth/captcha");

/** Solve a fresh proof-of-work challenge; the result is single-use and expires after 5 minutes. */
export async function solveFreshCaptcha(signal?: AbortSignal): Promise<string> {
  return solveCaptcha(await fetchChallenge(), signal);
}

async function sealed<T>(route: "login" | "register" | "delete-account", payload: Record<string, string>): Promise<T> {
  const info = await request<KemInfo>("/auth/kem");
  return request<T>(`/auth/${route}`, json(sealEnvelope(info, route, payload)));
}

export async function register(input: { email: string; password: string; pseudo: string; captcha: string }): Promise<AccountUser> {
  return (await sealed<{ user: AccountUser }>("register", input)).user;
}

export async function login(input: { email: string; password: string; captcha: string }): Promise<AccountUser> {
  return (await sealed<{ user: AccountUser }>("login", input)).user;
}

/** Erases the account for good; the password is re-checked server side. */
export async function deleteAccount(password: string): Promise<void> {
  await sealed<void>("delete-account", { password });
}

export interface AccountSession { id: string; last_seen: number; expires_at: number; current: boolean }

export async function listSessions(): Promise<AccountSession[]> {
  return (await request<{ sessions: AccountSession[] }>("/me/sessions")).sessions;
}

export async function revokeSession(id: string): Promise<void> {
  await request(`/me/sessions/${encodeURIComponent(id)}`, { method: "DELETE", headers: { "Content-Type": "application/json" } });
}

export async function exportAccount(): Promise<unknown> {
  return request<unknown>("/me/export");
}

export async function logout(): Promise<void> {
  await request<void>("/auth/logout", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
}

export async function fetchMe(): Promise<AccountUser | null> {
  return (await request<{ user: AccountUser | null }>("/auth/me")).user;
}

export interface RemoteProgress { season_id: number; anime_id: number; title: string; episode: number; position: number; completed: boolean; updated_at: number }

export async function pullProgress(): Promise<RemoteProgress[]> {
  return (await request<{ progress: RemoteProgress[] }>("/me/progress")).progress;
}

export async function pullHidden(): Promise<number[]> {
  return (await request<{ ids: number[] }>("/me/hidden")).ids;
}

export async function updateHidden(add: number[], remove: number[]): Promise<number[]> {
  return (await request<{ ids: number[] }>("/me/hidden", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ add, remove }) })).ids;
}

export async function pullWatchlist(): Promise<number[]> {
  return (await request<{ ids: number[] }>("/me/watchlist")).ids;
}

export async function updateWatchlist(add: number[], remove: number[]): Promise<number[]> {
  return (await request<{ ids: number[] }>("/me/watchlist", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ add, remove }) })).ids;
}

export async function deleteWatchData(): Promise<void> {
  await request("/me/history", { method: "DELETE" });
}

export async function pullWatchSessions<T>(): Promise<T[]> {
  return (await request<{ sessions: T[] }>("/me/history")).sessions;
}

export async function pushWatchSessions<T>(sessions: T[]): Promise<void> {
  await request("/me/history", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ sessions }) });
}

export async function pushProgress(progress: RemoteProgress[]): Promise<RemoteProgress[]> {
  return (await request<{ progress: RemoteProgress[] }>("/me/progress", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ progress }) })).progress;
}

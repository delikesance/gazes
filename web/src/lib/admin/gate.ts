import { AdminApiError } from "./api";

/**
 * True when the admin API itself refused the visitor: 401 (signed out), 403 (not an admin) or 404 (admin API
 * not mounted on this backend). Anything else (API down, 5xx, unreadable answer) is an outage, not a refusal.
 */
export function isAdminRefusal(error: unknown): boolean {
  return error instanceof AdminApiError && (error.status === 401 || error.status === 403 || error.status === 404);
}

/** Short, detail-free label for server logs: the API error code, never the message or a stack. */
export function gateFailureCode(error: unknown): string {
  return error instanceof AdminApiError ? `${error.status}/${error.code}` : "unexpected";
}

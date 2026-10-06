// Public donation API (internal/api/donations.go). The public side only ever receives names that
// donors chose to show; everything else about a donation stays on the admin side.

export interface PublicDonations {
  btcpay: boolean;
  kofi_url?: string;
  goal_cents?: number;
  /** Present only when a goal is configured. */
  month_cents?: number;
  min_cents: number;
  max_cents: number;
  donors: string[];
}

export type DonationErrorCode = "invalid_amount" | "invalid_name" | "invalid_request" | "unavailable" | "provider_unavailable" | "rate_limited" | "server_error" | "network";

export class DonationError extends Error {
  code: DonationErrorCode;
  constructor(code: DonationErrorCode) { super(code); this.code = code; }
}

export const PRESET_CENTS = [300, 500, 1000, 2000] as const;
export const MAX_NAME = 24;

function base() { return process.env.NEXT_PUBLIC_API_BASE || "/api/v1"; }

export async function fetchPublicDonations(signal?: AbortSignal): Promise<PublicDonations> {
  let res: Response;
  try { res = await fetch(`${base()}/donations`, { cache: "no-store", signal }); } catch { throw new DonationError("network"); }
  if (!res.ok) throw new DonationError("server_error");
  return res.json();
}

/** Opens a BTCPay invoice and returns its checkout URL. */
export async function createInvoice(input: { amount_cents: number; visibility: "anonymous" | "named"; display_name: string }): Promise<string> {
  let res: Response;
  try {
    res = await fetch(`${base()}/donations/btcpay/invoice`, {
      method: "POST", credentials: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input.visibility === "named" ? input : { amount_cents: input.amount_cents, visibility: "anonymous" }),
    });
  } catch { throw new DonationError("network"); }
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new DonationError(res.status === 429 ? "rate_limited" : ((body?.error as DonationErrorCode) || "server_error"));
  if (typeof body.checkout_url !== "string") throw new DonationError("server_error");
  return body.checkout_url;
}

/** 1250 -> "12,50 €" with plain spaces (same text on server and browser). */
export function eur(cents: number): string {
  const whole = Math.floor(cents / 100).toLocaleString("fr-FR").replace(/[  ]/g, " ");
  return cents % 100 === 0 ? `${whole} €` : `${whole},${String(cents % 100).padStart(2, "0")} €`;
}

/** "12,5" or "12.50" -> 1250; null unless a positive amount with at most two decimals. */
export function parseAmount(raw: string): number | null {
  const m = /^(\d{1,6})(?:[.,](\d{1,2}))?$/.exec(raw.trim());
  if (!m) return null;
  const cents = Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0") || 0);
  return cents > 0 ? cents : null;
}

/** Same rule as the server (donations.SanitizeName): 2-24 letters, digits, spaces, - _ ' and no link. */
export function validName(raw: string): boolean {
  const name = raw.trim().replace(/\s+/g, " ");
  const n = [...name].length;
  if (n < 2 || n > MAX_NAME) return false;
  if (!/^[\p{L}\p{N} _'’-]+$/u.test(name)) return false;
  return !/http|www/i.test(name);
}

/** Goal progress, 0 to 100, or null when there is no goal to show. */
export function goalPercent(d: Pick<PublicDonations, "goal_cents" | "month_cents">): number | null {
  if (!d.goal_cents || d.goal_cents <= 0 || d.month_cents === undefined) return null;
  return Math.min(100, Math.round((d.month_cents / d.goal_cents) * 100));
}

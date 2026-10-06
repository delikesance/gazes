// Pure logic of the Dons page: formatting and table rows. The admin sees exactly who gave what;
// `publicText` states what visitors see for the same donation.
import type { AdminDonation, AdminDonations } from "@/lib/admin/types";

export const PROVIDER_LABEL: Record<string, string> = { btcpay: "Crypto (BTCPay)", kofi: "Ko-fi", manual: "Saisie manuelle" };
export const STATUS_LABEL: Record<string, string> = { pending: "En attente", settled: "Reçu", expired: "Expiré" };

const NBSP = /[  ]/g;

/** 1250 -> "12,50 €" (plain spaces, so server and browser render the same text). */
export function eur(cents: number, currency = "EUR"): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  const text = `${Math.floor(abs / 100).toLocaleString("fr-FR")},${String(abs % 100).padStart(2, "0")}`.replace(NBSP, " ");
  return `${sign}${text} ${currency === "EUR" ? "€" : currency}`;
}

export function donorText(d: AdminDonation): string {
  const parts: string[] = [];
  if (d.pseudo) parts.push(`${d.pseudo} (compte #${d.user_id})`);
  else if (d.user_id !== null) parts.push(`Compte #${d.user_id}`);
  if (d.donor_label && d.donor_label !== d.pseudo) parts.push(d.donor_label);
  return parts.join(" · ") || "Donateur inconnu";
}

export function publicText(d: AdminDonation): string {
  if (d.status !== "settled") return "Pas encore public";
  return d.visibility === "named" && d.display_name ? `Affiché : ${d.display_name}` : "Anonyme";
}

export function donationKpis(summary: AdminDonations["summary"]) {
  const avg = summary.count > 0 ? Math.round(summary.all_cents / summary.count) : null;
  return [
    { label: "Reçu ce mois", value: eur(summary.month_cents), note: "Dons reçus en euros depuis le 1er du mois (UTC)" },
    { label: "Reçu au total", value: eur(summary.all_cents), note: "Depuis l'ouverture des dons" },
    { label: "Nombre de dons", value: summary.count, note: `${summary.donors} ${summary.donors > 1 ? "donateurs distincts" : "donateur distinct"}` },
    { label: "Don moyen", value: avg === null ? null : eur(avg), note: "Total reçu divisé par le nombre de dons" },
  ];
}

export function donationRow(d: AdminDonation) {
  const at = d.settled_at ?? d.created_at;
  return {
    id: d.id,
    when: new Date(at * 1000).toISOString().slice(0, 16).replace("T", " "),
    whenSort: at,
    donor: donorText(d),
    provider: PROVIDER_LABEL[d.provider] ?? d.provider,
    amount: d.amount_cents / 100,
    status: STATUS_LABEL[d.status] ?? d.status,
    statusTone: d.status === "settled" ? "accent" : d.status === "expired" ? "muted" : undefined,
    shown: publicText(d),
  };
}

/** "12,5" or "12.50" -> 1250; null when it is not a positive amount with at most two decimals. */
export function parseEuroInput(raw: string): number | null {
  const m = /^(\d{1,7})(?:[.,](\d{1,2}))?$/.exec(raw.trim());
  if (!m) return null;
  const cents = Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0") || 0);
  return cents > 0 ? cents : null;
}

/** "" -> null (detach), "12" -> 12, anything else -> undefined (invalid). */
export function parseUserId(raw: string): number | null | undefined {
  const s = raw.trim().replace(/^#/, "");
  if (s === "") return null;
  return /^\d{1,12}$/.test(s) && Number(s) > 0 ? Number(s) : undefined;
}

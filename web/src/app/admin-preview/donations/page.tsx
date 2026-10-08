import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { DonationsView } from "@/components/admin/pages/DonationsView";
import type { AdminDonation, AdminDonations } from "@/lib/admin/types";
import "@/app/admin/admin.css";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

const day = 86_400;
const now = Math.floor(Date.UTC(2026, 9, 5, 11, 16) / 1000);

function donation(id: number, fields: Partial<AdminDonation>): AdminDonation {
  return {
    id: `don_${id}`, provider: "kofi", provider_ref: `ref_${id}`, user_id: null, pseudo: null, donor_label: "",
    visibility: "anonymous", display_name: "", amount_cents: 500, currency: "EUR", status: "settled", message: "",
    created_at: now - id * day, settled_at: now - id * day, ...fields,
  };
}

// Synthetic sample (no donation was recorded on the dev backend): one of each provider, visibility and status.
const sample: AdminDonations = {
  summary: { month_cents: 2_500, all_cents: 6_000, count: 5, donors: 4 },
  limit: 200,
  offset: 0,
  donations: [
    donation(1, { user_id: 1, pseudo: "alice", visibility: "named", display_name: "alice", amount_cents: 1_000, message: "Merci pour le site !" }),
    donation(2, { provider: "btcpay", amount_cents: 1_500, currency: "EUR" }),
    donation(3, { provider: "manual", donor_label: "Virement de Bob", amount_cents: 2_000, created_at: now - 40 * day, settled_at: now - 40 * day }),
    donation(4, { provider: "btcpay", status: "pending", settled_at: null, amount_cents: 500 }),
    donation(5, { status: "expired", settled_at: null, amount_cents: 1_000 }),
  ],
};

/** Dev-only preview of Dons with a synthetic sample (no network, no guard, no writes). `?empty=1` shows no donation. */
export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  if (process.env.NODE_ENV === "production") notFound();
  const empty = (await searchParams).empty === "1";
  const data: AdminDonations = empty
    ? { summary: { month_cents: 0, all_cents: 0, count: 0, donors: 0 }, limit: 200, offset: 0, donations: [] }
    : sample;
  return (
    <AdminShell>
      <DonationsView data={data} generatedAt={new Date(now * 1000).toISOString()} />
    </AdminShell>
  );
}

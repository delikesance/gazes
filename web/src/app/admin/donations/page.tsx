import type { Metadata } from "next";
import { DonationsView } from "@/components/admin/pages/DonationsView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminDonations, AdminEnvelope } from "@/lib/admin/types";

export const metadata: Metadata = { title: "Dons" };

export default async function Page() {
  let res: AdminEnvelope<AdminDonations>;
  try {
    res = await adminGetServer<AdminDonations>("/donations", { params: { limit: "200" } });
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Soutien" title="Dons" periods={false} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  return <DonationsView data={res.data} generatedAt={res.generated_at} />;
}

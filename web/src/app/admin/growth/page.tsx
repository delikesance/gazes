import type { Metadata } from "next";
import { GrowthView } from "@/components/admin/pages/GrowthView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminEnvelope, AdminGrowth } from "@/lib/admin/types";

export const metadata: Metadata = { title: "Croissance" };

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);
  let res: AdminEnvelope<AdminGrowth>;
  try {
    res = await adminGetServer<AdminGrowth>("/growth", { period });
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Acquisition et rétention" title="Croissance" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  return <GrowthView growth={res.data} periodDays={period} from={res.period?.from} to={res.period?.to} />;
}

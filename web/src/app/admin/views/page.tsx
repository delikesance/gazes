import { ViewsView } from "@/components/admin/pages/ViewsView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminEnvelope, AdminViews } from "@/lib/admin/types";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);
  let res: AdminEnvelope<AdminViews>;
  try {
    res = await adminGetServer<AdminViews>("/views", { period });
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Séances de lecture" title="Visionnages" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  return <ViewsView views={res.data} periodDays={period} from={res.period?.from} to={res.period?.to} />;
}

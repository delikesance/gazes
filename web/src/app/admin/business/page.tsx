import { BusinessView } from "@/components/admin/pages/BusinessView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminCosts, AdminEnvelope, AdminOverview } from "@/lib/admin/types";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);

  let costs: AdminEnvelope<AdminCosts>;
  let overview: AdminEnvelope<AdminOverview>;
  try {
    [costs, overview] = await Promise.all([
      adminGetServer<AdminCosts>("/costs", { period }),
      adminGetServer<AdminOverview>("/overview", { period }),
    ]);
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Pilotage" title="Business" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={error} />
      </>
    );
  }

  return (
    <BusinessView
      costs={costs.data}
      overview={overview.data}
      periodDays={period}
      from={costs.period?.from}
      to={costs.period?.to}
      generatedAt={costs.generated_at}
    />
  );
}

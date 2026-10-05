import { OverviewView } from "@/components/admin/pages/OverviewView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminOverview, AdminPlaybackErrors, AdminPlaybackHealth } from "@/lib/admin/types";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

function describe(reason: unknown): string {
  return reason instanceof AdminApiError ? `${reason.message} (code : ${reason.code})` : "erreur inattendue";
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);
  const [overview, health, errors] = await Promise.allSettled([
    adminGetServer<AdminOverview>("/overview", { period }),
    adminGetServer<AdminPlaybackHealth>("/playback/health", { period }),
    adminGetServer<AdminPlaybackErrors>("/playback/errors", { params: { limit: 3 } }),
  ]);

  if (overview.status === "rejected") {
    if (!(overview.reason instanceof AdminApiError)) throw overview.reason;
    return (
      <>
        <AdminPageHeader eyebrow="Tableau de bord" title="Vue d'ensemble" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={overview.reason} />
      </>
    );
  }

  const failures = [health, errors].filter((r): r is PromiseRejectedResult => r.status === "rejected");
  const playbackNotice = failures.length > 0 ? `Mesures du lecteur indisponibles : ${describe(failures[0].reason)}.` : undefined;

  return (
    <OverviewView
      overview={overview.value.data}
      health={health.status === "fulfilled" ? health.value.data : null}
      errors={errors.status === "fulfilled" ? errors.value.data : null}
      playbackNotice={playbackNotice}
      periodDays={period}
      from={overview.value.period?.from}
      to={overview.value.period?.to}
      generatedAt={overview.value.generated_at}
    />
  );
}

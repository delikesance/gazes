import { PlaybackView } from "@/components/admin/pages/PlaybackView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type {
  AdminCosts,
  AdminErrorsSummary,
  AdminIssuesList,
  AdminPlaybackErrors,
  AdminPlaybackHealth,
  AdminSources,
} from "@/lib/admin/types";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

const DAY_MS = 86_400_000;

/** Start of the journal window: the same number of days as the selected period. */
function sinceDays(days: number): string {
  return new Date(Date.now() - days * DAY_MS).toISOString();
}

function describe(reason: unknown): string {
  return reason instanceof AdminApiError ? `${reason.message} (code : ${reason.code})` : "erreur inattendue";
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);
  const since = sinceDays(period);
  const [health, errors, summary, sources, costs, issues] = await Promise.allSettled([
    adminGetServer<AdminPlaybackHealth>("/playback/health", { period }),
    adminGetServer<AdminPlaybackErrors>("/playback/errors", { params: { since, limit: 200 } }),
    adminGetServer<AdminErrorsSummary>("/playback/errors/summary", { period }),
    adminGetServer<AdminSources>("/playback/sources", { period }),
    adminGetServer<AdminCosts>("/costs", { period }),
    adminGetServer<AdminIssuesList>("/issues", { params: { limit: 50 } }),
  ]);

  const required = [health, errors, summary, sources].find((r): r is PromiseRejectedResult => r.status === "rejected");
  if (required) {
    if (!(required.reason instanceof AdminApiError)) throw required.reason;
    return (
      <>
        <AdminPageHeader eyebrow="Dépannage" title="Lecteur et flux" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={required.reason} />
      </>
    );
  }
  if (health.status !== "fulfilled" || errors.status !== "fulfilled" || summary.status !== "fulfilled" || sources.status !== "fulfilled") {
    throw new Error("unreachable");
  }

  const notices: string[] = [];
  if (costs.status === "rejected") notices.push(`Coûts et pic de séances indisponibles : ${describe(costs.reason)}.`);
  if (issues.status === "rejected") notices.push(`Constats signalés indisponibles : ${describe(issues.reason)}.`);

  return (
    <PlaybackView
      health={health.value.data}
      errors={errors.value.data}
      summary={summary.value.data}
      sources={sources.value.data}
      costs={costs.status === "fulfilled" ? costs.value.data : null}
      issues={issues.status === "fulfilled" ? issues.value.data : null}
      notices={notices}
      periodDays={period}
      from={health.value.period?.from}
      to={health.value.period?.to}
    />
  );
}

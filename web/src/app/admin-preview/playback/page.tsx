import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { PlaybackView } from "@/components/admin/pages/PlaybackView";
import type {
  AdminCosts,
  AdminEnvelope,
  AdminErrorsSummary,
  AdminIssuesList,
  AdminPlaybackErrors,
  AdminPlaybackHealth,
  AdminSources,
} from "@/lib/admin/types";
import healthJson from "@/lib/admin/real/playback-health.json";
import errorsJson from "@/lib/admin/real/playback-errors.json";
import summaryJson from "@/lib/admin/real/playback-errors-summary.json";
import sourcesJson from "@/lib/admin/real/playback-sources.json";
import costsJson from "@/lib/admin/real/costs.partial.json";
import issuesJson from "@/lib/admin/real/issues.json";
import "@/app/admin/admin.css";

const health = healthJson as AdminEnvelope<AdminPlaybackHealth>;
const errors = errorsJson as AdminEnvelope<AdminPlaybackErrors>;
const summary = summaryJson as AdminEnvelope<AdminErrorsSummary>;
const sources = sourcesJson as AdminEnvelope<AdminSources>;
const costs = costsJson as AdminEnvelope<AdminCosts>;
const issues = issuesJson as AdminEnvelope<AdminIssuesList>;

/** Dev-only preview of Lecteur et flux with real recorded responses (no network, no guard). */
export default function Page() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <AdminShell>
      <PlaybackView
        health={health.data}
        errors={errors.data}
        summary={summary.data}
        sources={sources.data}
        costs={costs.data}
        issues={issues.data}
        periodDays={health.period?.days ?? 30}
        from={health.period?.from}
        to={health.period?.to}
      />
    </AdminShell>
  );
}

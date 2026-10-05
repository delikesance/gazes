import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { OverviewView } from "@/components/admin/pages/OverviewView";
import type { AdminEnvelope, AdminOverview, AdminPlaybackErrors, AdminPlaybackHealth } from "@/lib/admin/types";
import overviewJson from "@/lib/admin/real/overview.json";
import healthJson from "@/lib/admin/real/playback-health.json";
import errorsJson from "@/lib/admin/real/playback-errors.json";
import "@/app/admin/admin.css";

const overview = overviewJson as AdminEnvelope<AdminOverview>;
const health = healthJson as AdminEnvelope<AdminPlaybackHealth>;
const errors = errorsJson as AdminEnvelope<AdminPlaybackErrors>;

/** Dev-only preview of the Vue d'ensemble with real recorded responses (no network, no guard). */
export default function Page() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <AdminShell>
      <OverviewView
        overview={overview.data}
        health={health.data}
        errors={errors.data}
        periodDays={overview.period?.days ?? 30}
        from={overview.period?.from}
        to={overview.period?.to}
        generatedAt={errors.generated_at}
      />
    </AdminShell>
  );
}

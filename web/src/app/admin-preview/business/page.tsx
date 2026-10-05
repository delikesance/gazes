import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { BusinessView } from "@/components/admin/pages/BusinessView";
import type { AdminCosts, AdminEnvelope, AdminOverview } from "@/lib/admin/types";
import costsJson from "@/lib/admin/real/costs.json";
import overviewJson from "@/lib/admin/real/overview.json";
import "@/app/admin/admin.css";

const costs = costsJson as AdminEnvelope<AdminCosts>;
const overview = overviewJson as AdminEnvelope<AdminOverview>;

/** Dev-only preview of Business with real recorded responses (no network, no guard). */
export default function Page() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <AdminShell>
      <BusinessView
        costs={costs.data}
        overview={overview.data}
        periodDays={costs.period?.days ?? 7}
        from={costs.period?.from}
        to={costs.period?.to}
        generatedAt={costs.generated_at}
      />
    </AdminShell>
  );
}

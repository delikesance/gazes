import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { ViewsView } from "@/components/admin/pages/ViewsView";
import type { AdminEnvelope, AdminViews } from "@/lib/admin/types";
import viewsJson from "@/lib/admin/real/views.json";
import "@/app/admin/admin.css";

const views = viewsJson as AdminEnvelope<AdminViews>;

/** Dev-only preview of Visionnages with a real recorded response (no network, no guard). */
export default function Page() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <AdminShell>
      <ViewsView views={views.data} periodDays={views.period?.days ?? 30} from={views.period?.from} to={views.period?.to} />
    </AdminShell>
  );
}

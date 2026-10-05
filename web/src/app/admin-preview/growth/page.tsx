import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { GrowthView } from "@/components/admin/pages/GrowthView";
import type { AdminEnvelope, AdminGrowth } from "@/lib/admin/types";
import growthJson from "@/lib/admin/real/growth.json";
import emptyJson from "@/lib/admin/real/growth.empty.json";
import "@/app/admin/admin.css";

const growth = growthJson as AdminEnvelope<AdminGrowth>;
const empty = emptyJson as AdminEnvelope<AdminGrowth>;

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

/** Dev-only preview of Croissance with real recorded responses. ?empty=1 shows the empty period. */
export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  if (process.env.NODE_ENV === "production") notFound();
  const res = (await searchParams).empty === "1" ? empty : growth;
  return (
    <AdminShell>
      <GrowthView growth={res.data} periodDays={res.period?.days ?? 30} from={res.period?.from} to={res.period?.to} />
    </AdminShell>
  );
}

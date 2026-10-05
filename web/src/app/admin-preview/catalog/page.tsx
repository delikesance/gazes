import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { CatalogView } from "@/components/admin/pages/CatalogView";
import { parseCatalogFormat } from "@/components/admin/pages/catalog.logic";
import type { AdminCatalog, AdminEnvelope } from "@/lib/admin/types";
import allJson from "@/lib/admin/real/catalog.json";
import movieJson from "@/lib/admin/real/catalog.movie.json";
import emptyJson from "@/lib/admin/real/catalog.empty.json";
import "@/app/admin/admin.css";

const all = allJson as AdminEnvelope<AdminCatalog>;
const movie = movieJson as AdminEnvelope<AdminCatalog>;
const empty = emptyJson as AdminEnvelope<AdminCatalog>;

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

/** Dev-only preview of Catalogue with real recorded responses. ?format=movie shows the recorded movie tab, ?empty=1 the empty period. */
export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  if (process.env.NODE_ENV === "production") notFound();
  const params = await searchParams;
  if (params.empty === "1") {
    return (
      <AdminShell>
        <CatalogView all={empty.data} tab={empty.data} format="all" from={empty.period?.from} to={empty.period?.to} />
      </AdminShell>
    );
  }
  const format = parseCatalogFormat(params.format);
  const tab = format === "movie" ? movie : all;
  return (
    <AdminShell>
      <CatalogView all={all.data} tab={tab.data} format={format} from={all.period?.from} to={all.period?.to} />
    </AdminShell>
  );
}

import type { Metadata } from "next";
import { CatalogView } from "@/components/admin/pages/CatalogView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { parseCatalogFormat } from "@/components/admin/pages/catalog.logic";
import { AdminApiError, parseAdminPeriod } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminCatalog, AdminEnvelope } from "@/lib/admin/types";

export const metadata: Metadata = { title: "Catalogue" };

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

/** Series kept for the aggregates (KPIs, matrix medians, leavers): the API maximum. */
const ALL_LIMIT = 200;

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const period = parseAdminPeriod(params.period);
  const format = parseCatalogFormat(params.format);
  let all: AdminEnvelope<AdminCatalog>;
  let tab: AdminEnvelope<AdminCatalog>;
  try {
    // The page-wide figures always cover every format; a format tab only changes the top table (second call).
    const allCall = adminGetServer<AdminCatalog>("/catalog", { period, params: { format: "all", limit: ALL_LIMIT } });
    const tabCall = format === "all" ? allCall : adminGetServer<AdminCatalog>("/catalog", { period, params: { format } });
    [all, tab] = await Promise.all([allCall, tabCall]);
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Animes et bibliothèque" title="Catalogue" subtitle={`Période : ${period} derniers jours`} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  return <CatalogView all={all.data} tab={tab.data} format={format} from={all.period?.from} to={all.period?.to} />;
}

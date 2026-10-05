import { AdminPageHeader } from "./AdminPageHeader";
import { parseAdminPeriod } from "@/lib/admin/api";

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

/** Temporary page body: header + empty state, until the page's content lands. */
export async function AdminPlaceholder({ eyebrow, title, searchParams }: { eyebrow: string; title: string; searchParams: SearchParams }) {
  const period = parseAdminPeriod((await searchParams).period);
  return (
    <>
      <AdminPageHeader eyebrow={eyebrow} title={title} subtitle={`Période : ${period} derniers jours`} />
      <section className="admin-empty" aria-live="polite">
        <h2>Page en cours de construction</h2>
        <p>Les mesures de cette section arrivent bientôt.</p>
      </section>
    </>
  );
}

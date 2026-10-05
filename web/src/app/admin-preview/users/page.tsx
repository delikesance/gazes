import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { UsersView } from "@/components/admin/pages/UsersView";
import { parseUsersQuery } from "@/components/admin/pages/users.logic";
import type { AdminEnvelope, AdminUserDetail, AdminUsersList, AdminUsersSummary } from "@/lib/admin/types";
import summaryJson from "@/lib/admin/real/users-summary.json";
import listJson from "@/lib/admin/real/users.session.json";
import detailJson from "@/lib/admin/real/user-detail.session.json";
import detailEmptyJson from "@/lib/admin/real/user-detail.empty.session.json";
import "@/app/admin/admin.css";

const summary = summaryJson as AdminEnvelope<AdminUsersSummary>;
const list = listJson as AdminEnvelope<AdminUsersList>;
const detail = detailJson as AdminEnvelope<AdminUserDetail>;
const detailEmpty = detailEmptyJson as AdminEnvelope<AdminUserDetail>;

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

/**
 * Dev-only preview of Utilisateurs with real recorded responses (no network, no guard).
 * ?user=6 shows an account without sessions, ?user=1 (default) one with history, ?user=none the empty panel,
 * ?empty=1 an empty directory.
 */
export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  if (process.env.NODE_ENV === "production") notFound();
  const params = await searchParams;
  const query = parseUsersQuery(params);
  const empty = params.empty === "1";
  const userParam = params.user;
  const selected = userParam === "none" ? null : query.user ?? 1;
  const data: AdminUsersList = empty ? { total: 0, limit: 25, offset: 0, users: [] } : list.data;
  const shown = selected === null ? null : selected === 6 ? detailEmpty.data : detail.data;
  return (
    <AdminShell>
      <UsersView
        summary={summary.data}
        list={data}
        detail={shown}
        detailError={null}
        query={{ ...query, user: selected }}
        generatedAt={list.generated_at}
      />
    </AdminShell>
  );
}

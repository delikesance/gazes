import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { UsersView } from "@/components/admin/pages/UsersView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { pageCount, parseUsersQuery, usersApiParams, usersQueryString } from "@/components/admin/pages/users.logic";
import { AdminApiError } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminEnvelope, AdminUserDetail, AdminUsersList, AdminUsersSummary } from "@/lib/admin/types";

export const metadata: Metadata = { title: "Utilisateurs" };

type SearchParams = Promise<{ [key: string]: string | string[] | undefined }>;

async function loadDetail(id: number): Promise<{ detail: AdminUserDetail | null; error: { message: string; code: string } | null }> {
  try {
    const res = await adminGetServer<AdminUserDetail>(`/users/${id}`);
    return { detail: res.data, error: null };
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    const message = error.status === 404 ? "Ce compte n'existe pas." : error.message;
    return { detail: null, error: { message, code: error.code } };
  }
}

export default async function Page({ searchParams }: { searchParams: SearchParams }) {
  const query = parseUsersQuery(await searchParams);
  let summary: AdminEnvelope<AdminUsersSummary>;
  let list: AdminEnvelope<AdminUsersList>;
  let detailResult: Awaited<ReturnType<typeof loadDetail>> = { detail: null, error: null };
  try {
    [summary, list, detailResult] = await Promise.all([
      adminGetServer<AdminUsersSummary>("/users/summary"),
      adminGetServer<AdminUsersList>("/users", { params: usersApiParams(query) }),
      query.user === null ? Promise.resolve(detailResult) : loadDetail(query.user),
    ]);
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Comptes" title="Utilisateurs" periods={false} />
        <ApiErrorBlock error={error} />
      </>
    );
  }

  // A page number past the end (stale link, search that narrowed the list): go to the last page.
  const last = pageCount(list.data.total, list.data.limit);
  if (list.data.total > 0 && list.data.users.length === 0 && query.page > last) {
    const qs = usersQueryString({ ...query, page: last });
    redirect(qs ? `/admin/users?${qs}` : "/admin/users");
  }

  return (
    <UsersView
      summary={summary.data}
      list={list.data}
      detail={detailResult.detail}
      detailError={detailResult.error}
      query={query}
      generatedAt={list.generated_at}
    />
  );
}

import type { Metadata } from "next";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { AdminApiError, adminGet } from "@/lib/admin/api";
import { getApiBase } from "@/lib/api";
import "./admin.css";

// Never prerender or share anything of this area: every request is checked against the session.
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: { default: "Admin — Gazes", template: "%s — Admin Gazes" },
  robots: { index: false, follow: false, nocache: true },
};

async function pseudoOf(cookie: string): Promise<string | undefined> {
  try {
    const res = await fetch(`${getApiBase()}/auth/me`, { headers: { Cookie: cookie }, cache: "no-store" });
    if (!res.ok) return undefined;
    return ((await res.json()) as { user?: { pseudo?: string } | null }).user?.pseudo;
  } catch {
    return undefined;
  }
}

export default async function AdminLayout({ children }: { children: ReactNode }) {
  const cookie = (await cookies()).toString();
  try {
    await adminGet("/me", { cookie });
  } catch (error) {
    // 401 (signed out) and 403 (not an admin) both answer 404: the panel must not reveal that it exists, and
    // a /login redirect would confirm it. A 404 from the API means the admin API is not mounted on this
    // backend (not configured or an older build): there is no panel either. Other failures (API down, 5xx)
    // surface through the error boundary.
    if (error instanceof AdminApiError && (error.status === 401 || error.status === 403 || error.status === 404)) notFound();
    throw error;
  }
  return <AdminShell pseudo={await pseudoOf(cookie)}>{children}</AdminShell>;
}

import type { Metadata } from "next";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { adminGet } from "@/lib/admin/api";
import { gateFailureCode, isAdminRefusal } from "@/lib/admin/gate";
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
    // Every failure answers 404, so the panel never reveals that it exists. 401 (signed out) and 403 (not an
    // admin) are refusals, and a /login redirect would confirm the panel; a 404 from the API means it is not
    // mounted on this backend. An outage (API down, 5xx) also answers 404: the visitor is not identified yet,
    // so an error page or a retry button would tell a stranger that something lives here. Admins reload.
    if (!isAdminRefusal(error)) console.error(`admin gate unavailable: ${gateFailureCode(error)}`);
    notFound();
  }
  return <AdminShell pseudo={await pseudoOf(cookie)}>{children}</AdminShell>;
}

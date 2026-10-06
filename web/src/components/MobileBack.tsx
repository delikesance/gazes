"use client";
import { useEffect } from "react";
import { BackButton } from "./BackButton";
import { rememberResults } from "@/lib/navigation-history";
import { HeaderBreadcrumb } from "./HeaderBreadcrumb";

/** Phone-only ← in the header for pages without a breadcrumb (genre, search results). */
export function MobileBack({ fallback, label, results }: { fallback: string; label: string; results?: string }) {
  useEffect(() => { if (results) rememberResults(`${window.location.pathname}${window.location.search}`, results); }, [results]);
  return <HeaderBreadcrumb><nav aria-label={label} className="detail-breadcrumb crumb-mobile"><BackButton fallback={fallback} label={label} /></nav></HeaderBreadcrumb>;
}

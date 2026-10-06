"use client";
import { BackButton } from "./BackButton";
import { HeaderBreadcrumb } from "./HeaderBreadcrumb";

/** Phone-only ← in the header for pages without a breadcrumb (genre, search results). */
export function MobileBack({ fallback, label }: { fallback: string; label: string }) {
  return <HeaderBreadcrumb><nav aria-label={label} className="detail-breadcrumb crumb-mobile"><BackButton fallback={fallback} label={label} /></nav></HeaderBreadcrumb>;
}

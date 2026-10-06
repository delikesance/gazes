"use client";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { canGoBack } from "@/lib/navigation-history";

/** Phone back control: returns to the page the viewer came from, or to the logical parent when the page was opened directly. */
export function BackButton({ fallback, label }: { fallback: string; label: string }) {
  const router = useRouter();
  return <button type="button" className="breadcrumb-back" onClick={() => (canGoBack() ? router.back() : router.push(fallback))}>
    <ArrowLeft size={18} aria-hidden="true" /><span>{label}</span>
  </button>;
}

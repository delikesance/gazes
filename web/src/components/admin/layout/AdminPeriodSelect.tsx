"use client";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { ADMIN_PERIODS, parseAdminPeriod } from "@/lib/admin/api";

/** 7/30/90 days selector kept in the URL (?period=). */
export function AdminPeriodSelect() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const active = parseAdminPeriod(search.get("period"));
  const choose = (days: number) => {
    const next = new URLSearchParams(search.toString());
    next.set("period", String(days));
    router.replace(`${pathname}?${next.toString()}`, { scroll: false });
  };
  return (
    <div role="group" aria-label="Période" className="admin-periods">
      {ADMIN_PERIODS.map((days) => (
        <button key={days} type="button" aria-pressed={days === active} onClick={() => choose(days)}>{days} jours</button>
      ))}
    </div>
  );
}

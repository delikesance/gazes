"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ADMIN_NAV } from "./nav";

export function AdminSidebar({ pseudo }: { pseudo?: string }) {
  const pathname = usePathname();
  return (
    <aside className="admin-sidebar">
      <div className="admin-brand">
        <span className="admin-wordmark">gazes<span>.</span></span>
        <span className="admin-chip">Admin</span>
      </div>
      <nav aria-label="Sections de l'administration" className="admin-nav">
        {ADMIN_NAV.map((item) => {
          const current = item.href === "/admin" ? pathname === "/admin" : pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link key={item.href} href={item.href} aria-current={current ? "page" : undefined} className="admin-nav-link">
              {item.label}
            </Link>
          );
        })}
      </nav>
      <div className="admin-account">
        <span className="admin-eyebrow">Connecté</span>
        {pseudo ? <span className="admin-account-name">{pseudo}</span> : null}
        <span className="admin-account-note">Accès réservé aux administrateurs</span>
      </div>
    </aside>
  );
}

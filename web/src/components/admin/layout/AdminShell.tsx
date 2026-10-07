import type { ReactNode } from "react";
import { AdminSidebar } from "./AdminSidebar";

/**
 * Dark admin frame: sidebar + content. On narrow screens the sidebar drops below the content, so a top bar
 * links down to the sections menu (pages run several screens long on a phone).
 */
export function AdminShell({ pseudo, children }: { pseudo?: string; children: ReactNode }) {
  return (
    <div className="admin-shell">
      <div className="admin-topbar">
        <span className="admin-wordmark">gazes<span>.</span></span>
        <a href="#admin-sections" className="admin-topbar-link">Menu</a>
      </div>
      <main id="admin-main" className="admin-main">{children}</main>
      <AdminSidebar pseudo={pseudo} />
    </div>
  );
}

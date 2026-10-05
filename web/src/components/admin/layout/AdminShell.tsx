import type { ReactNode } from "react";
import { AdminSidebar } from "./AdminSidebar";

/** Dark admin frame: sidebar + content. On narrow screens the sidebar drops below the content. */
export function AdminShell({ pseudo, children }: { pseudo?: string; children: ReactNode }) {
  return (
    <div className="admin-shell">
      <main id="admin-main" className="admin-main">{children}</main>
      <AdminSidebar pseudo={pseudo} />
    </div>
  );
}

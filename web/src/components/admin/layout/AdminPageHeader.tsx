import { Suspense, type ReactNode } from "react";
import { AdminPeriodSelect } from "./AdminPeriodSelect";

export interface AdminPageHeaderProps {
  eyebrow: string;
  title: string;
  subtitle?: string;
  /** Pills shown on the right, before the period selector. */
  badges?: ReactNode;
  /** Hide the period selector on pages that do not depend on it. */
  periods?: boolean;
}

export function AdminPageHeader({ eyebrow, title, subtitle, badges, periods = true }: AdminPageHeaderProps) {
  return (
    <header className="admin-page-header">
      <div className="admin-page-heading">
        <span className="admin-eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        {subtitle ? <p>{subtitle}</p> : null}
      </div>
      {badges || periods ? (
        <div className="admin-page-aside">
          {badges}
          {periods ? <Suspense fallback={null}><AdminPeriodSelect /></Suspense> : null}
        </div>
      ) : null}
    </header>
  );
}

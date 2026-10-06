import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Nouveautés",
  description: "Le journal de développement de Gazes : nouvelles fonctionnalités et corrections, mise à jour par mise à jour.",
  alternates: { canonical: "/changelog" },
};

export default function ChangelogLayout({ children }: { children: ReactNode }) {
  return children;
}

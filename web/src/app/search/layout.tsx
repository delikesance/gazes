import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = { title: "Rechercher", robots: { index: false, follow: true } };

export default function SearchLayout({ children }: { children: ReactNode }) {
  return children;
}

import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = { title: "État du service" };

export default function StatusLayout({ children }: { children: ReactNode }) {
  return children;
}

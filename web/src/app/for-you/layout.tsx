import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = { title: "Pour vous", robots: { index: false, follow: false } };

export default function ForYouLayout({ children }: { children: ReactNode }) {
  return children;
}

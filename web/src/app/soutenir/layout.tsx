import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Soutenir Gazes",
  description: "Gazes est gratuit et sans publicité : les dons paient le serveur et la bande passante. Don anonyme par défaut.",
  alternates: { canonical: "/soutenir" },
};

export default function SupportLayout({ children }: { children: ReactNode }) {
  return children;
}

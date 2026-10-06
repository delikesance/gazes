import type { Metadata } from "next";
import { RecoverPage } from "@/components/RecoverPage";

export const metadata: Metadata = { title: "Mot de passe oublié", robots: { index: false, follow: false } };

export default function Page() {
  return <RecoverPage />;
}

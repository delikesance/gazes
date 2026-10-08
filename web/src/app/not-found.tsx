import type { Metadata } from "next";
import { NotFoundView } from "@/components/NotFoundView";

// Site-wide 404, also answered by the admin panel to anyone it does not let in: it must stay generic, title
// included (the admin layout's own title would otherwise reveal the panel).
export const metadata: Metadata = {
  title: { absolute: "Page introuvable · Gazes" },
};

export default function NotFound() {
  return <NotFoundView />;
}

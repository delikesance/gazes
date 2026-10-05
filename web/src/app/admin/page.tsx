import { AdminPlaceholder } from "@/components/admin/layout/AdminPlaceholder";

export default function Page({ searchParams }: { searchParams: Promise<{ [key: string]: string | string[] | undefined }> }) {
  return <AdminPlaceholder eyebrow="Tableau de bord" title="Vue d'ensemble" searchParams={searchParams} />;
}

import { AdminPlaceholder } from "@/components/admin/layout/AdminPlaceholder";

export default function Page({ searchParams }: { searchParams: Promise<{ [key: string]: string | string[] | undefined }> }) {
  return <AdminPlaceholder eyebrow="Économie" title="Business" searchParams={searchParams} />;
}

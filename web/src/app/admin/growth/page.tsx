import { AdminPlaceholder } from "@/components/admin/layout/AdminPlaceholder";

export default function Page({ searchParams }: { searchParams: Promise<{ [key: string]: string | string[] | undefined }> }) {
  return <AdminPlaceholder eyebrow="Acquisition" title="Croissance" searchParams={searchParams} />;
}

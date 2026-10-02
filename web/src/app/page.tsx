import { CatalogBrowser } from "@/components/CatalogBrowser";
import { getCatalogPopular, getCatalogSeasonal, searchCatalog } from "@/lib/api";

export default async function Home({
  searchParams,
}: {
  searchParams?: Promise<{ q?: string; genre?: string; tab?: string; page?: string }>;
}) {
  const params = searchParams ? await searchParams : {};
  const q = params?.q || "";
  const genre = params?.genre || "";
  const tab = params?.tab || "trending";
  const page = Math.max(1, Number(params?.page) || 1);

  let initialData = null;
  let initialPopular = null;
  try {
    if (q || genre) {
      initialData = await searchCatalog(q, genre, page, 24);
    } else if (tab === "popular") {
      initialData = await getCatalogPopular(page, 24);
    } else {
      initialData = await getCatalogSeasonal(page, 24);
    }
  } catch (e) {
    console.error("SSR fetch error:", e);
  }

  try {
    if (!q && !genre && page === 1) {
      initialPopular = await getCatalogPopular(1, 12);
    }
  } catch (e) {
    console.error("SSR popular error:", e);
  }

  return (
    <CatalogBrowser
      initialData={initialData}
      initialPopular={initialPopular}
      initialQuery={q}
      initialGenre={genre}
      initialTab={tab}
      initialPage={page}
    />
  );
}

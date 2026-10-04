import { CatalogBrowser } from "@/components/CatalogBrowser";
import { getCatalogPopular, getCatalogSeasonal, searchCatalog } from "@/lib/api";

export default async function Home({
  searchParams,
}: {
  searchParams?: Promise<{ q?: string; genre?: string; genres?: string; exclude?: string; tab?: string; page?: string }>;
}) {
  const params = searchParams ? await searchParams : {};
  const q = params?.q || "";
  const genre = params?.genres || params?.genre || "";
  const exclude = params?.exclude || "";
  const tab = params?.tab || "trending";
  const page = Math.max(1, Number(params?.page) || 1);

  let initialData = null;
  let initialPopular = null;
  try {
    if (q || genre || exclude) {
      initialData = await searchCatalog(q, genre, page, 24, exclude);
    } else if (tab === "suggestions" || tab === "popular") {
      // Personalised from the viewer's local history: loaded in the browser.
    } else if (page > 1) {
      initialData = await getCatalogSeasonal(page, 24);
    }
  } catch (e) {
    console.error("SSR fetch error:", e);
  }

  try {
    if (!q && !genre && !exclude && page === 1) {
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
      initialExclude={exclude}
      initialTab={tab}
      initialPage={page}
    />
  );
}

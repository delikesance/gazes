import type { Metadata } from "next";
import { CatalogBrowser } from "@/components/CatalogBrowser";
import { SITE_NAME, SITE_URL, jsonLd } from "@/lib/site";
import { getCatalogPopular, getCatalogSeasonal, searchCatalog } from "@/lib/api";

type HomeSearchParams = { q?: string; genre?: string; genres?: string; exclude?: string; tab?: string; page?: string };

// Search, filter and paginated views duplicate the home content: keep them out of the index.
export async function generateMetadata({ searchParams }: { searchParams?: Promise<HomeSearchParams> }): Promise<Metadata> {
  const p = searchParams ? await searchParams : {};
  const variant = p?.q || p?.genre || p?.genres || p?.exclude || (p?.tab && p.tab !== "trending") || (Number(p?.page) || 1) > 1;
  return variant ? { robots: { index: false, follow: true } } : { alternates: { canonical: "/" } };
}

export default async function Home({
  searchParams,
}: {
  searchParams?: Promise<HomeSearchParams>;
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

  const websiteLd = { "@context": "https://schema.org", "@type": "WebSite", name: SITE_NAME, url: SITE_URL, inLanguage: "fr" };

  return (
    <>
    <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(websiteLd) }} />
    <CatalogBrowser
      initialData={initialData}
      initialPopular={initialPopular}
      initialQuery={q}
      initialGenre={genre}
      initialExclude={exclude}
      initialTab={tab}
      initialPage={page}
    />
    </>
  );
}

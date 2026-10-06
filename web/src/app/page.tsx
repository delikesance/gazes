import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { CatalogBrowser } from "@/components/CatalogBrowser";
import { MAX_SSR_PAGE, SITE_NAME, SITE_URL, jsonLd } from "@/lib/site";
import { getCatalogPopular, getCatalogSeasonal } from "@/lib/api";

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

  // Searches live on their own page; older links and bookmarks keep working.
  if (q || genre || exclude) {
    const next = new URLSearchParams();
    if (q) next.set("q", q);
    if (genre) next.set("genres", genre);
    if (exclude) next.set("exclude", exclude);
    redirect(`/search?${next.toString()}`);
  }
  if (tab === "suggestions" || tab === "popular") redirect("/for-you");

  let initialData = null;
  let initialPopular = null;
  try {
    if (page > MAX_SSR_PAGE) {
      // Deep pages are loaded in the browser, under its own rate limit.
    } else if (page > 1) {
      initialData = await getCatalogSeasonal(page, 24);
    }
  } catch (e) {
    console.error("SSR fetch error:", e);
  }

  try {
    if (page === 1) {
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

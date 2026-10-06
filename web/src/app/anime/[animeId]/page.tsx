import { cache } from "react";
import type { Metadata } from "next";
import { AnimeBrowser } from "@/components/AnimeBrowser";
import { getFranchise } from "@/lib/api";
import { SITE_URL, jsonLd, snippet } from "@/lib/site";

type Params = { params: Promise<{ animeId: string }> };

const franchiseOf = cache(async (id: number) => {
  try {
    return id ? await getFranchise(id) : null;
  } catch (e) {
    console.error("SSR getFranchise error:", e);
    return null;
  }
});

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const id = Number((await params).animeId);
  const franchise = await franchiseOf(id);
  if (!franchise) return { robots: { index: false, follow: false } };
  // Thin pages (no synopsis, no poster) would dilute the index.
  const thin = !snippet(franchise.description) && !franchise.poster_image;
  const description = snippet(franchise.description) || `Regardez ${franchise.title} en streaming sur Gazes.`;
  const images = franchise.poster_image ? [franchise.poster_image] : undefined;
  return {
    ...(thin && { robots: { index: false, follow: true } }),
    title: franchise.title,
    description,
    alternates: { canonical: `/anime/${id}` },
    openGraph: { type: "video.tv_show", title: franchise.title, description, url: `/anime/${id}`, images },
    twitter: { card: images ? "summary_large_image" : "summary", title: franchise.title, description, images },
  };
}

export default async function Page({ params }: Params) {
  const id = Number((await params).animeId);
  const initialFranchise = await franchiseOf(id);
  const mainSeasons = (initialFranchise?.seasons ?? []).filter((s) => s.group === "main");
  const ld = initialFranchise && {
    "@context": "https://schema.org",
    "@type": "TVSeries",
    name: initialFranchise.title,
    url: `${SITE_URL}/anime/${id}`,
    description: snippet(initialFranchise.description, 300),
    image: initialFranchise.poster_image,
    inLanguage: "fr",
    ...(mainSeasons.length > 0 && {
      numberOfSeasons: mainSeasons.length,
      containsSeason: mainSeasons.map((s, i) => ({
        "@type": "TVSeason",
        name: s.title,
        seasonNumber: s.season_number ?? i + 1,
        ...(s.start_date && { startDate: s.start_date }),
      })),
    }),
  };
  const breadcrumbLd = initialFranchise && {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: "Gazes", item: SITE_URL },
      { "@type": "ListItem", position: 2, name: initialFranchise.title, item: `${SITE_URL}/anime/${id}` },
    ],
  };
  return (
    <>
      {ld && <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(ld) }} />}
      {breadcrumbLd && <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(breadcrumbLd) }} />}
      <AnimeBrowser animeId={id} initialFranchise={initialFranchise} />
    </>
  );
}

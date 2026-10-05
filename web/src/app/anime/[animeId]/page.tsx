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
  const description = snippet(franchise.description) || `Regardez ${franchise.title} en streaming sur Gazes.`;
  const images = franchise.poster_image ? [franchise.poster_image] : undefined;
  return {
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
  const ld = initialFranchise && {
    "@context": "https://schema.org",
    "@type": "TVSeries",
    name: initialFranchise.title,
    url: `${SITE_URL}/anime/${id}`,
    description: snippet(initialFranchise.description, 300),
    image: initialFranchise.poster_image,
    inLanguage: "fr",
  };
  return (
    <>
      {ld && <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(ld) }} />}
      <AnimeBrowser animeId={id} initialFranchise={initialFranchise} />
    </>
  );
}

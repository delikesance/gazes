import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { AnimeCatalogCard } from "@/components/AnimeCatalogCard";
import { GenreHeader } from "@/components/GenreHeader";
import { SeasonalGrid } from "@/components/SeasonalGrid";
import { genreFromSlug, genreLabel } from "@/lib/genres";
import { searchCatalog } from "@/lib/api";
import { SITE_URL, jsonLd, ssrPage } from "@/lib/site";
import Link from "next/link";

type Props = { params: Promise<{ slug: string }>; searchParams?: Promise<{ page?: string }> };

export async function generateMetadata({ params, searchParams }: Props): Promise<Metadata> {
  const genre = genreFromSlug((await params).slug);
  if (!genre) return { robots: { index: false, follow: false } };
  const page = ssrPage((await searchParams)?.page);
  if (!page) return { robots: { index: false, follow: false } };
  const label = genreLabel(genre);
  return {
    title: `Animes ${label} en streaming`,
    description: `Découvrez les meilleurs animes ${label} et regardez-les en streaming sur Gazes, en VF ou VOSTFR.`,
    alternates: { canonical: `/genre/${(await params).slug}` },
    ...(page > 1 && { robots: { index: false, follow: true } }),
  };
}

export default async function GenrePage({ params, searchParams }: Props) {
  const { slug } = await params;
  const genre = genreFromSlug(slug);
  if (!genre) notFound();
  const page = ssrPage((await searchParams)?.page);
  if (!page) notFound();
  let items: Awaited<ReturnType<typeof searchCatalog>>["items"] = [];
  let hasNext = false;
  try {
    const res = await searchCatalog("", genre, page, 24);
    items = res.items ?? [];
    hasNext = res.has_next_page;
  } catch (e) {
    console.error("SSR genre error:", e);
  }
  const ld = {
    "@context": "https://schema.org",
    "@type": "ItemList",
    itemListElement: items.map((a, i) => ({ "@type": "ListItem", position: i + 1, url: `${SITE_URL}/anime/${a.id}`, name: a.franchise_title || a.display_title })),
  };
  return (
    <main className="catalog-page genre-page">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(ld) }} />
      <GenreHeader genre={genre} />
      <SeasonalGrid count={items.length} evenRows={false}>
        {items.map((anime) => <AnimeCatalogCard key={anime.id} anime={anime} />)}
      </SeasonalGrid>
      {(page > 1 || hasNext) && (
        <nav className="page-inset genre-pager" aria-label="Pagination">
          {page > 1 && <Link className="clay clay-secondary clay-sm" href={page === 2 ? `/genre/${slug}` : `/genre/${slug}?page=${page - 1}`} rel="prev">←</Link>}
          {hasNext && <Link className="clay clay-secondary clay-sm" href={`/genre/${slug}?page=${page + 1}`} rel="next">→</Link>}
        </nav>
      )}
    </main>
  );
}

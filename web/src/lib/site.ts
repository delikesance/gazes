export const SITE_URL = (process.env.NEXT_PUBLIC_SITE_URL || "https://gazes.delikesance.cloud").replace(/\/$/, "");
export const SITE_NAME = "Gazes";

/** Deepest catalog page a server-side render fetches: each new page number is an uncached catalog query. */
export const MAX_SSR_PAGE = 50;

/** The ?page= of a server-rendered listing, from 1; null past MAX_SSR_PAGE. */
export function ssrPage(raw: string | undefined): number | null {
  const page = Math.max(1, Math.floor(Number(raw)) || 1);
  return page <= MAX_SSR_PAGE ? page : null;
}

/** Plain-text snippet for meta descriptions: the catalog synopsis arrives as HTML. */
export function snippet(html: string | undefined, max = 160): string | undefined {
  const text = (html || "").replace(/<[^>]*>/g, " ").replace(/&[a-z#0-9]+;/gi, " ").replace(/\s+/g, " ").trim();
  if (!text) return undefined;
  return text.length <= max ? text : `${text.slice(0, max - 1).trimEnd()}…`;
}

/** Serialises structured data for an inline <script type="application/ld+json">. */
export function jsonLd(data: unknown): string {
  return JSON.stringify(data).replace(/</g, "\\u003c");
}

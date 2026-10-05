export const SITE_URL = (process.env.NEXT_PUBLIC_SITE_URL || "https://gazes.delikesance.cloud").replace(/\/$/, "");
export const SITE_NAME = "Gazes";

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

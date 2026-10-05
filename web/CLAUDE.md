@AGENTS.md
UI/UX: skill `ui-design` first (tokens, components, no gradients, mobile-first, verify 390px+1440px). Strings via `src/lib/i18n.ts` (`t()`, FR default).
Tests: `node scripts/*.test.mjs` directly (pnpm run needs real node_modules). Playwright flows: `scripts/design-flow.mjs`, `qa_playwright.mjs`.
SEO: `src/lib/site.ts` (SITE_URL from NEXT_PUBLIC_SITE_URL, `snippet`, `jsonLd`). Public pages export `metadata`/`generateMetadata` (canonical, OG; anime pages: `cache(getFranchise)` + JSON-LD). Non-content pages (auth, history, season/episode, filtered home) are `noindex`. `sitemap.ts` is force-dynamic (build has no backend) + 1h memory cache, few AniList calls. Default share image: `app/opengraph-image.png` (flat colors). Keep the Search Console meta in `layout.tsx`.

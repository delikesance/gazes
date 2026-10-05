---
name: ui-design
description: Use for ANY Gazes UX/UI work (new view, component, redesign, layout/styling change, share image). Gives the design workflow, the app's tokens/components and the verification steps so the result matches the site.
---
1. Invoke skill `artifact-design` for principles only (hierarchy, spacing, type scale, a11y, responsive); ignore its publishing/page-contract parts. The tokens below override its palette and fonts.
2. Read the closest existing component first; `rg -n "<class|--token>" web/src/app/globals.css` (76K: never read whole). Reuse `web/src/components/ui/*` (Button, Badge, Select, LazyImage, PageGrid, Scribble) before writing new markup/CSS.
3. Tokens (dark default; `.light` class on `<html>` must also look right):
   - dark: bg #09090b, fg #fafafa, muted #a1a1aa, surface #151517 (hover #1c1c1f, card #111113, raised #17171a), border rgba(255,255,255,.07), accent #9b8afb on #fff.
   - light: bg #f8f8fa, fg #18181b, muted #62626d, surface #ececf0, accent #7c6af0.
   - radii: pill 999, panel 24, card 20, poster 17. Gutter `var(--page-gutter)` = clamp(25px,3.9vw,80px).
   - fonts (next/font vars): `--font-dm-sans` body, `--font-fraunces` display, `--font-geist-mono` `.eyebrow`/mono. Wordmark: `gazes` + accent `.`.
4. Rules: NO gradients (flat colors, patterns, `Scribble`); mobile-first from 390px; visible `:focus-visible` (accent outline); touch targets ≥40px; contrast AA in both themes; images through `LazyImage`; no new deps/fonts; every string through `t()` (`src/lib/i18n.ts`).
5. Verify before reporting: Playwright screenshots at 390×844 and 1440×900, dark + light, against the dev stack (`localhost:8081`, see `web/scripts/design-flow.mjs`); fix what looks off. For a live critique of a page, `critic-layer:critic-layer`.
6. Report ≤3 lines.

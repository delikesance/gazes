# Opening/Ending Skip Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a "Passer l'opening" / "Passer l'ending" button in the player while playback is inside a detected opening or ending.

**Architecture:** A native Matroska chapter reader feeds a pure detector (`DetectSkipSegments`) from `ProbeReader`, so both playback paths receive `skip_segments` in `VideoMetadata`. Missing kinds are filled by a cached AniSkip lookup behind a catalog endpoint. The player merges both, finds the active segment and seeks past it.

**Tech Stack:** Go (chi router, internal `kv` cache), Next.js / React / TypeScript, `node --test`.

**Spec:** `docs/superpowers/specs/2026-10-04-opening-ending-skip-design.md` — every numeric threshold, keyword list, element ID and HTTP behaviour below comes from it; read the section named in each task.

## Global Constraints

- Repo root: `/home/workstation/Projects/gazes/.claude/worktrees/episode-preview-duplicates-f96c11`.
- Go commands need `GOMODCACHE=$HOME/go/pkg/mod`. Packages linking SQLite (`internal/api`, `internal/playback`, …) need `-ldflags='-extldflags=-Wl,--allow-multiple-definition'` (pre-existing link issue). `internal/indexer` has 2 pre-existing failing tests (TestRanking, TestTargetedFrenchSources) — not ours.
- `ffprobe` is not installed locally: no test may require it.
- Detection failures never block or delay playback; uncertain detection shows no button.
- Remux-over-transcode and streaming invariants of AGENTS.md stay untouched; chapter reading must never read a Cluster.
- UI: flat styling, no colour gradients.
- Code comments in English, terse, matching surrounding style. Commit subjects in English imperative, each commit message ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Never bare `git stash`; never push.

## Review Focus

1. Torrent-backed reader stalls while the chapter reader seeks to a SeekHead target at the end of the file → reading gives up after 5 s and playback metadata is still returned (Task 3 test with a blocking `ReadContext`).
2. A release whose chapters are "Part A / Part B / Preview" or similar with a ~90 s chapter mid-episode → no false opening/ending (Task 2 table rows).
3. AniSkip returns timings for another encode of the same episode → rejected by the 3 s duration tolerance (Task 4 test).
4. HLS session whose `timeline_origin` ≠ 0 → button appears at the right moment (Task 7 verification of the timeline the player compares against).
5. Ending that ends at the file's last seconds with a next episode available → "Épisode suivant" instead of seeking to the end, which would end playback (Task 6 pure helper test + Task 7 wiring).

---

### Task 1: Matroska chapter reader

**Files:**
- Create: `internal/metadata/matroska_chapters.go`
- Test: `internal/metadata/matroska_chapters_test.go` (package `metadata`; reuse `ebmlElement` and the `id*` vars from `matroska_test.go`, add the chapter IDs)

**Interfaces:**
- Produces:
  ```go
  type Chapter struct {
  	Start float64 `json:"start"`
  	End   float64 `json:"end"`
  	Title string  `json:"title"`
  }
  func readMatroskaChapters(ctx context.Context, r io.ReadSeeker, size int64) ([]Chapter, error)
  ```
  Reads with `ReadContext(ctx, p)` when `r` exposes it (same pattern as `skipMatroskaAttachments`). Budget constants: 1 MB total read, 512 KB max `Chapters` element.

- [ ] **Step 1: Write the failing tests** — build synthetic MKVs (EBML header + Segment with SeekHead/Info/Tracks/Chapters/Cluster as each case needs). Tests and assertions:
  - `TestReadChaptersFromHeader`: 3 atoms at 0 / 90 s / 1290 s with titles → 3 chapters, Start/End in seconds (ns ÷ 1e9), sorted.
  - `TestReadChaptersThroughSeekHead`: Chapters placed after a Cluster at the file end, referenced only by SeekHead → found; and the reader never consumed the Cluster payload (wrap the source to fail any read inside the Cluster byte range).
  - `TestReadChaptersNone`: no Chapters → `nil, nil`.
  - `TestReadChaptersSkipsHiddenDisabledAndLinked`: atoms with FlagHidden=1, FlagEnabled=0, ChapterSegmentUID present → excluded.
  - `TestReadChaptersEditions`: two editions, second has EditionFlagDefault=1 → second's atoms; an ordered edition (EditionFlagOrdered=1) is ignored; all-ordered → no chapters.
  - `TestReadChaptersPrefersEnglishTitle`: displays `jpn` then `eng` → eng string; no `eng` → first string.
  - `TestReadChaptersMissingEnd`: End absent → next chapter's Start; last stays 0.
  - `TestReadChaptersNestedAtomsIgnored`: an atom nested inside an atom is not returned.
  - `TestReadChaptersTruncated`: Chapters size beyond file end → error.
  - `TestReadChaptersTooLarge`: Chapters element > 512 KB → error.
  - `TestReadChaptersNotMatroska`: MP4-like bytes → `nil, nil`.
  - `TestReadChaptersCancelled`: cancelled ctx with a `ReadContext` reader → error `context.Canceled`.

- [ ] **Step 2: Run, verify they fail** — `GOMODCACHE=$HOME/go/pkg/mod go test ./internal/metadata/ -run Chapters` → FAIL (undefined: readMatroskaChapters).
- [ ] **Step 3: Implement** per spec §1 (element IDs, edition/atom rules, budget). Walk top-level Segment children until the first Cluster; follow SeekHead for 0x1043A770 when not met inline.
- [ ] **Step 4: Run, verify pass** — same command → PASS; `go vet ./internal/metadata/`.
- [ ] **Step 5: Commit** — `Read Matroska chapters natively, following the SeekHead`.

### Task 2: Opening/ending detection

**Files:**
- Create: `internal/metadata/skip_segments.go`
- Test: `internal/metadata/skip_segments_test.go`

**Interfaces:**
- Consumes: `Chapter` (Task 1).
- Produces:
  ```go
  type SkipSegment struct {
  	Kind   string  `json:"kind"`   // "opening" | "ending"
  	Start  float64 `json:"start"`
  	End    float64 `json:"end"`
  	Source string  `json:"source"` // "chapters" | "aniskip"
  }
  func DetectSkipSegments(chapters []Chapter, duration float64) []SkipSegment
  ```

- [ ] **Step 1: Write the failing table test** `TestDetectSkipSegments` — one row per case, asserting the exact `[]SkipSegment` (nil when none). Episode duration 1420 s unless stated. Rows:
  1. Titles "Prologue" 0–120, "Opening" 120–210, "Part A" 210–700, "Part B" 700–1290, "Ending" 1290–1380, "Preview" 1380–1420 → opening 120–210, ending 1290–1380.
  2. Same with "OP1"/"ED1"; with "Générique de début"/"Générique de fin"; with "オープニング"/"エンディング" → same segments.
  3. "Episode" 0–40, "Main" 40–1420 → nil (the word "Episode" must not match "ed"; "Main" covers > half the file).
  4. Generic "Chapter 01" 0–150 (cold open), "Chapter 02" 150–240 (90 s), "Chapter 03" 240–1290, "Chapter 04" 1290–1380 (90 s), "Chapter 05" 1380–1420 → opening 150–240, ending 1290–1380.
  5. "Chapter 01" 0–89, "Chapter 02" 89–1420 → opening 0–89 only.
  6. Generic, no 75–110 s chapter in the last quarter → opening only.
  7. "Part A" 0–700, "Part B" 700–1420 → nil.
  8. Generic 90 s chapter at 600–690 only → nil (neither first third nor last quarter).
  9. Movie 7200 s with titled "Opening" 0–100 and generic 90 s chapters mid-film → opening only.
  10. File 180 s → nil.
  11. One chapter "Opening" covering 0–1420 → nil.
  12. Title "OP / ED" (matches both) → ignored.
  13. Titled "Opening" lasting 15 s → ignored; lasting 200 s → ignored.
  14. Titled "Ending" 100–190 and "Opening" 1200–1290 (opening after ending) → nil.
  15. Last chapter "Ending" with End 0, duration 1420, start 1330 → ending 1330–1420.
  16. duration 0 → nil.
  17. Titled opening + generic 90 s chapter in last quarter → titled opening + duration-based ending.
- [ ] **Step 2: Run, verify fail** — `go test ./internal/metadata/ -run DetectSkipSegments` → FAIL.
- [ ] **Step 3: Implement** per spec §2 (keyword lists verbatim, whole-word case-insensitive matching — Unicode-aware for the Japanese and accented terms; thresholds 20–180 s titled, 75–110 s heuristic, first third / last quarter, < 5 min → none, > half file → never, opening must end before ending start else drop both, `Source: "chapters"`, sorted by Start).
- [ ] **Step 4: Run, verify pass**; `go vet`.
- [ ] **Step 5: Commit** — `Detect openings and endings from chapter titles, lengths and positions`.

### Task 3: Attach chapters and segments to probed metadata

**Files:**
- Modify: `internal/metadata/probe.go` (`VideoMetadata`, `ProbeReader`)
- Test: `internal/metadata/probe_chapters_test.go` (package `metadata`)

**Interfaces:**
- Consumes: `readMatroskaChapters` (Task 1), `DetectSkipSegments` (Task 2).
- Produces: `VideoMetadata.Chapters []Chapter \`json:"chapters,omitempty"\``, `VideoMetadata.SkipSegments []SkipSegment \`json:"skip_segments,omitempty"\``, and
  ```go
  func attachChapters(ctx context.Context, logger *slog.Logger, r io.Reader, meta *VideoMetadata)
  ```
  called by `ProbeReader` right before returning a successful `meta`. No-op if `r` is not an `io.ReadSeeker`; otherwise seek 0, read chapters with a 5 s timeout, seek 0 again; errors logged at Debug and ignored; sets `Chapters` and `SkipSegments = DetectSkipSegments(Chapters, meta.DurationSec)`.

- [ ] **Step 1: Failing tests**
  - `TestAttachChaptersFillsSegments`: synthetic MKV (titled Opening/Ending) in a `bytes.Reader`, meta with DurationSec 1420 → Chapters len as built, SkipSegments opening+ending; reader position afterwards is 0.
  - `TestAttachChaptersNonSeekable`: `io.Reader` only → meta unchanged.
  - `TestAttachChaptersGivesUpOnStall`: reader whose `ReadContext` blocks until ctx is done when reading past the header (SeekHead target at end) → returns within ~5 s (use a test hook: make the timeout a package var `chapterReadTimeout` and set it to 50 ms in the test), meta has no chapters, no panic.
  - `TestAttachChaptersCorrupt`: truncated Chapters → meta has no chapters.
- [ ] **Step 2: Run, verify fail** — `go test ./internal/metadata/ -run AttachChapters`.
- [ ] **Step 3: Implement** fields + `attachChapters` + call in `ProbeReader` (both early-return success paths).
- [ ] **Step 4: Run** `go test -count=1 ./internal/metadata/` and `go test -count=1 -ldflags='-extldflags=-Wl,--allow-multiple-definition' ./internal/playback/ ./internal/api/` → PASS.
- [ ] **Step 5: Commit** — `Expose chapters and skip segments in probed video metadata`.

### Task 4: AniList MAL id and AniSkip lookup

**Files:**
- Modify: `internal/metadata/catalog.go` (`animeDetailWithEpisodesQuery` adds `idMal`; `aniListMediaItem.IDMal int \`json:"idMal"\``; `AnimeCatalogItem.MalID int \`json:"mal_id,omitempty"\`` set in `formatCatalogItem`; `AnimeCatalogService` gains `aniskipC *kv.Cache[[]SkipSegment]` or equivalent wrapper type, created in `NewAnimeCatalogService` and `SetRedis` with domain `"aniskip"`, and `aniskipBaseURL string` defaulting to `https://api.aniskip.com`)
- Create: `internal/metadata/aniskip.go`
- Test: `internal/metadata/aniskip_test.go`

**Interfaces:**
- Consumes: `SkipSegment` (Task 2).
- Produces: `func (s *AnimeCatalogService) SkipTimes(ctx context.Context, malID, episode int, duration float64) ([]SkipSegment, error)`; `AnimeCatalogItem.MalID`.

- [ ] **Step 1: Failing tests** (httptest server, `s.aniskipBaseURL = srv.URL`, service from `NewAnimeCatalogService(srv.Client())`):
  - `TestSkipTimesFound`: server asserts path `/v2/skip-times/37430/3` and query `types=op&types=ed&episodeLength=1420.12` (both `types` values present); returns the spec's JSON with op 2.196–122.196 and ed 1325–1415, episodeLength 1420 → two segments, kinds opening/ending, Source "aniskip".
  - `TestSkipTimesNotFound`: 404 `{"found":false,"results":[]}` → empty, nil error.
  - `TestSkipTimesRejectsOtherEncode`: episodeLength 1440.065 for duration 1420.12 → empty.
  - `TestSkipTimesRejectsBadIntervals`: start ≥ end, end > duration+1, length 10 s, length 200 s → each rejected.
  - `TestSkipTimesFirstValidPerKind`: two valid `op` → first kept.
  - `TestSkipTimesErrors`: 500, invalid JSON, handler sleeping past the timeout (make the 4 s timeout a package var for the test) → error returned.
  - `TestSkipTimesCaching`: success then server shut down → second call served from cache; "not found" cached; an error is not cached (server counts hits: error, then success → 2 hits).
  - `TestSkipTimesInvalidArgs`: malID 0, episode 0, duration 0 → empty, nil, zero server hits.
  - `TestFormatCatalogItemMalID`: `aniListMediaItem{ID:1, IDMal:37430}` → `MalID == 37430`.
- [ ] **Step 2: Run, verify fail** — `go test ./internal/metadata/ -run 'SkipTimes|MalID'`.
- [ ] **Step 3: Implement** per spec §3 (validation tolerance 3 s, 20–180 s, cache key `{malID}:{episode}:{round(duration)}`, TTL 7 days found / 1 day empty, errors uncached). Use the service's HTTP client directly (not the AniList governor).
- [ ] **Step 4: Run** `go test -count=1 ./internal/metadata/` → PASS.
- [ ] **Step 5: Commit** — `Look up AniSkip skip times by MAL id for files without usable chapters`.

### Task 5: Skip-times endpoint

**Files:**
- Create: `internal/api/skip_handlers.go`
- Modify: `internal/api/router.go` (register `cat.Get("/seasons/{season}/episodes/{ep}/skip-times", s.HandleSkipTimes)` next to the preview routes)
- Test: `internal/api/skip_handlers_test.go` (follow `catalog_handlers_test.go`: fake `http.Transport` answering AniList by host `graphql.anilist.co` and AniSkip by host `api.aniskip.com`)

**Interfaces:**
- Consumes: `AnimeCatalogService.GetAnimeDetailsWithEpisodes`, `.SkipTimes`, `AnimeCatalogItem.MalID` (Task 4).
- Produces: `GET /api/v1/catalog/seasons/{season}/episodes/{ep}/skip-times?duration=` → `200 {"segments":[SkipSegment...]}`.

- [ ] **Step 1: Failing tests**
  - `TestSkipTimesRejectsInvalidParams`: season `x`, ep `0`, duration missing / `-1` / `NaN` / `30000` → 400 each.
  - `TestSkipTimesReturnsSegments`: AniList media with idMal 37430, AniSkip found → 200, two segments, `Cache-Control: private, max-age=3600`.
  - `TestSkipTimesWithoutMalID`: idMal null → 200 `{"segments":[]}`, no AniSkip request, `Cache-Control: no-store`.
  - `TestSkipTimesAniSkipDown`: AniSkip 500 → 200 empty, `no-store`.
  - Body always encodes `segments` as `[]`, never `null`.
- [ ] **Step 2: Run, verify fail** — `go test -ldflags='-extldflags=-Wl,--allow-multiple-definition' ./internal/api/ -run SkipTimes`.
- [ ] **Step 3: Implement** per spec §3 "Endpoint" (duration upper bound 6 h = 21600 s; AniSkip failure logged at Warn via `diagnostics.Logger`).
- [ ] **Step 4: Run** the api package → PASS.
- [ ] **Step 5: Commit** — `Serve AniSkip skip times for a season episode`.

### Task 6: Frontend skip-segment logic and API client

**Files:**
- Create: `web/src/lib/skip-segments.ts`
- Create: `web/scripts/skip-segments.test.mjs`
- Modify: `web/package.json` (script `"test:skip": "node --test scripts/skip-segments.test.mjs"`), `web/src/types/api.ts` (`VideoMetadata.chapters?: {start:number;end:number;title:string}[]`, `skip_segments?: SkipSegment[]` importing/re-exporting the type), `web/src/lib/api.ts` (`getSkipTimes`)

**Interfaces:**
- Produces:
  ```ts
  export interface SkipSegment { kind: 'opening' | 'ending'; start: number; end: number; source: 'chapters' | 'aniskip' }
  export function mergeSkipSegments(fromChapters: SkipSegment[], fromAniSkip: SkipSegment[]): SkipSegment[]
  export function activeSkipSegment(segments: SkipSegment[], position: number): SkipSegment | null
  export function needsAniSkip(fromChapters: SkipSegment[]): boolean
  export function skipAction(segment: SkipSegment, totalDuration: number, hasNextEpisode: boolean): 'seek' | 'next-episode'
  // api.ts
  export async function getSkipTimes(seasonId: number, episode: number, duration: number, signal?: AbortSignal): Promise<SkipSegment[]>
  ```
  `skipAction` returns `'next-episode'` only for an ending with `end >= totalDuration - 5` and `hasNextEpisode`. `getSkipTimes` resolves `[]` on any HTTP or network error.

- [ ] **Step 1: Failing tests** (`node:test`, import `../src/lib/skip-segments.ts` like `episode-file.test.mjs`):
  - merge: chapters opening + aniskip opening & ending → chapters opening + aniskip ending, sorted by start; empty inputs → `[]`.
  - active: segment 120–210 → active at 120, 150, 208.99; not at 119.9, 209, 210, 300.
  - needsAniSkip: `[]` → true; opening only → true; both → false.
  - skipAction: ending 1330–1420 / total 1422 / next → `'next-episode'`; same without next → `'seek'`; ending 1290–1380 / total 1420 → `'seek'`; opening near end → `'seek'`.
- [ ] **Step 2: Run, verify fail** — `cd web && node --test scripts/skip-segments.test.mjs` → FAIL (module not found).
- [ ] **Step 3: Implement** module, types, `getSkipTimes` (follow the URL/base and error style of `requestEpisodePreview` in `api.ts`).
- [ ] **Step 4: Run** `npm run test:skip`, `npx tsc --noEmit`, `npm run lint` → clean.
- [ ] **Step 5: Commit** — `Add skip-segment merging and selection for the player`.

### Task 7: Skip button in the player

**Files:**
- Create: `web/src/components/SkipSegmentButton.tsx`
- Modify: `web/src/components/VideoPlayerModal.tsx`, `web/src/lib/i18n.ts`, the player stylesheet where `player-pill` is defined (find with grep)

**Interfaces:**
- Consumes: everything from Task 6; `handleSeek`, `onNextEpisode`, `seasonId`, `episodeNumber`, `videoMeta`, `totalDuration`, `playbackOffset`, `currentTimeRef` already in `VideoPlayerModal`.
- Produces: `export function SkipSegmentButton(props: { segment: SkipSegment; action: 'seek' | 'next-episode'; onSkip: () => void }): JSX.Element`.

- [ ] **Step 1: Verify the timeline** — read how `playbackOffset`, `hls-playback.ts` (`timeline_origin`) and `handleSeek` relate in HLS and legacy modes; write the conclusion (which expression gives file-timeline seconds, and any origin correction) as a comment at the call site and in the task report.
- [ ] **Step 2: Wire state** — once `videoMeta.duration_sec > 0`: segments = `videoMeta.skip_segments ?? []`; if `needsAniSkip` and `seasonId > 0 && episodeNumber`, call `getSkipTimes` once per (season, episode, file) with an AbortController cleaned up on change/unmount; store `mergeSkipSegments(...)`. Track the active segment from the existing time-update path without adding per-frame React re-renders beyond a state change when the active segment id changes.
- [ ] **Step 3: Button** — rendered while a segment is active, independent of `showControls`, bottom-right above the dock; labels via `t()`: "Passer l'opening", "Passer l'ending", "Épisode suivant" (when `skipAction` is `'next-episode'`). Click: `handleSeek(segment.end)` or `onNextEpisode()`. Add i18n pairs `["Passer l'opening","Skip opening"]`, `["Passer l'ending","Skip ending"]`, `["Épisode suivant","Next episode"]` (check for an existing "Épisode suivant" key first). Flat style reusing `player-pill` tokens.
- [ ] **Step 4: Shortcut** — `case "KeyS"` in the existing keyboard handler: same action as the click when a segment is active; otherwise nothing.
- [ ] **Step 5: Verify** — `npx tsc --noEmit`, `npm run lint`, `npm run test:skip`, `npm run test:files` → clean. Run `npm run build` if it completes in this environment; report if it cannot.
- [ ] **Step 6: Commit** — `Show a skip button during detected openings and endings`.

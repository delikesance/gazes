# Gazes 🎬⚡

> A scalable, cost-efficient torrent video streaming platform combining sequential BitTorrent downloading with in-browser WebRTC P2P mesh delivery ("YouTube for Torrents").

---

## 🌟 Overview

**Gazes** enables instantaneous, zero-install video streaming directly from BitTorrent swarms in a modern web browser. By combining an ultra-fast sequential streaming bridge with client-side WebRTC mesh networking, Gazes achieves instant playback times and full BitTorrent swarm compatibility while drastically reducing server bandwidth and compute overhead.

```
                  ┌─────────────────────────────────────────┐
                  │       Web Browser (Video Player)        │
                  └────────────┬──────────────────▲─────────┘
                               │ (WebRTC P2P)     │ (HTTP/HLS Fallback)
                               ▼                  │
                  ┌─────────────────────────┐     │
                  │  Other Active Viewers   │     │
                  │ (Zero-Cost P2P Swarm)   │     │
                  └─────────────────────────┘     │
                                                  │
                                   ┌──────────────┴──────────────┐
                                   │  Gazes Streaming Bridge     │
                                   │  (Sequential Engine & Remux)│
                                   └──────────────▲──────────────┘
                                                  │ (BitTorrent TCP/uTP/DHT)
                                   ┌──────────────┴──────────────┐
                                   │   Standard Torrent Swarm    │
                                   └─────────────────────────────┘
```

---

## 🚀 Key Features

* **🎌 Anime-Focused Indexing (Nyaa.si Integration)**: Native integration with Nyaa.si RSS and search APIs for discovering, cataloging, and streaming anime torrents.
* **⚡ Instant 1-Click Playback**: Play torrent videos directly in the browser within seconds without waiting for the full file to download.
* **🌐 Universal Swarm Compatibility**: Connects to all standard BitTorrent networks (TCP, uTP, DHT, PEX, Trackers).
* **💰 Ultra-Low Compute Cost (Smart Remuxing)**: Zero-CPU re-wrapping of existing H.264/H.265 streams from MKV/AVI into browser-compatible formats, transcoding only incompatible audio (e.g. AC3/DTS/FLAC to AAC/Opus).
* **💬 Styled Subtitles**: ASS/SSA rendering with JASSUB, independent of player controls, with WebVTT extraction retained for other clients.
* **🤝 P2P-Assisted Bandwidth Offloading**: Active viewers sharing the same stream exchange chunks via WebRTC mesh, offloading 70–90% of origin egress bandwidth for viral content.
* **🧠 Ephemeral Sliding-Window Buffering**: Streams are cached in a sliding memory/disk buffer ahead of playback position rather than saving massive video files permanently to server disks.
* **🔍 Torrent & Metadata Indexer**: Rich metadata scraping (titles, posters, video duration, audio/subtitle tracks, peer health).

---

## 🛠️ Architecture & Tech Stack

| Layer | Technology | Role |
| :--- | :--- | :--- |
| **Streaming Core** | **Go** (`anacrolix/torrent`) or **Rust** | Sequential piece scheduling, BitTorrent protocol, sliding window buffer |
| **Media Pipeline** | **FFmpeg** / Custom Remuxer | Container remuxing (MKV/MP4/fMP4/HLS) & audio transcoding |
| **P2P Mesh Layer**| **WebRTC** (`p2p-media-loader`) | Peer-to-peer segment sharing among concurrent browser viewers |
| **Frontend UI**   | **Next.js / React / TypeScript** | Responsive web app, catalog browsing, modern HTML5 player (Video.js / HLS.js) |
| **Metadata & DB** | **PostgreSQL / SQLite** + **Redis** | Magnet link indexing, search, peer health stats, caching |

---

## 📂 Project Structure

```text
gazes/
├── cmd/
│   ├── server/           # Main backend API & streaming server entrypoint
│   └── indexer/          # Torrent metadata scraper & tracker worker
├── internal/
│   ├── torrent/          # Torrent client, sequential piece scheduler & DHT
│   ├── stream/           # HTTP range serving, HLS segmenter & remuxer
│   ├── metadata/         # TMDB/IMDb scraping, audio/sub track parsing
│   └── config/           # Environment & runtime configurations
├── web/                  # Frontend web application (React/Next.js UI)
├── AGENTS.md             # Guidelines & context for AI coding agents
└── README.md             # Project overview & documentation
```

---

## 📋 Roadmap

- [ ] **Phase 1: Streaming Core**: Implement sequential piece downloading and HTTP Range streaming engine.
- [ ] **Phase 2: Media Pipeline**: Fast on-the-fly MKV-to-fMP4/HLS remuxing pipeline with audio normalization.
- [ ] **Phase 3: Web UI & Player**: Build YouTube-like catalog and video player with buffer health visualization.
- [ ] **Phase 4: WebRTC P2P Mesh**: Integrate `p2p-media-loader` for peer-assisted viewer mesh delivery.
- [ ] **Phase 5: Search & Discovery**: Automated torrent metadata extraction, category filters, and peer health monitoring.

---

## 📄 License

MIT License

### Anime discovery and torrent ranking

The catalog now navigates through shareable anime, season, and episode pages. Main
continuity, movies, and side stories appear in separate groups. Catalog identities
come from AniList relationships rather than title-prefix grouping; incomplete
upstream results show a warning and offer retry.

Episode sources are matched to the selected season before ranking. Seeded releases
appear first. Within that group the score adds 100 points for explicit VF/VOSTFR
(once), 2 for MULTI, up to 60 for seeders using logarithmic weighting, and 0–4 for
resolution. Leechers do not add points. MULTI alone does not confirm French
availability. Zero-seeder releases are attempted last. Packs select the requested
episode file; ambiguous matches automatically advance to the next source.

The season-aware API is under `/api/v1/catalog/anime/{id}`:
`/franchise`, `/seasons/{season}`, and
`/seasons/{season}/episodes/{ep}/sources`. The older episode-source URL remains
supported. Catalog responses expose `has_next_page`; source responses expose
`partial`, `warning`, and score breakdowns. Provider pagination and searches are
bounded; “all torrents” means all matching results returned within these bounds.

Confirmed absolute-episode or split-part release mappings can be added to the
versioned `internal/indexer/numbering.go` registry. It starts empty: ambiguous
numbering is deliberately excluded instead of guessing a season offset.

Verification: `go test -tags=nosqlite ./...`, then from `web`: `npm run lint`,
`node_modules/.bin/tsc --noEmit`, `npm run test:files`, and
`node_modules/.bin/next build --webpack`. For the mocked browser flow, start the
frontend on port 4391 and run `npm run test:seasons` (set
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` for your Chromium installation).

Episode searches prioritize `Anime Name S01E01` for each title alias. The franchise
season shown in the interface is separate from the release season: for example,
Naruto Shippuden appears as Naruto’s second entry but searches
`Naruto Shippuden S01E01`, not `Naruto S02E01`. Renamed continuations restart release
numbering at S01; explicitly numbered titles keep their stated season. More
specific sibling titles are excluded from original-series matches, including
punctuation variants such as `Naruto: Shippuden`.


Player verification: `go test -tags=nosqlite ./internal/stream -run TestHEVCAudioSwitch -v`
checks HEVC 8/10-bit, AAC copy, AC3 conversion, selected audio tones and packet
sync after a non-keyframe seek. From `web`, `npm run test:player` tests the real
modal and JASSUB in Firefox with a simulated media clock (set `FIREFOX_PATH`
when using a system Playwright Firefox). This test reports native HEVC support
separately; simulated captions do not establish HEVC playback support.

Subtitle worker/WASM/font assets are generated locally by the dev/build scripts.
JASSUB is pinned because the worker build selects its Canvas2D renderer on
Firefox to avoid static captions disappearing during control opacity changes.
An upgrade must review this compatibility patch. Other browsers use JASSUB's
normal renderer selection. Subtitle extraction defaults to WebVTT; `format=ass`
preserves ASS styles and converts other text subtitles for JASSUB. Fonts absent
from the supplied track use the bundled Liberation Sans fallback.


Episode pages start playback automatically with seeded sources first, then score,
then swarm strength. Duplicate magnets are tried once; equally ranked individual
releases are preferred over packs. Packs are resolved from filenames before any
video probe: only an unambiguous episode match is opened. Failed sources advance
automatically, preserving playback position. Each attempt allows 12 seconds for
metadata and 20 seconds for its first video; a playing stream without time
advancement for 15 seconds is replaced. Pausing does not trigger replacement.
After exhausting candidates, retry is explicit rather than an endless loop.

Torrent readers inherit request cancellation. Their shared priority leases keep
other viewers' windows alive and release abandoned lookahead on close/seek;
headers are limited to the first/last piece of the selected episode, and lookahead
is capped by the configured byte budget. Metadata probes run one at a time on the
frontend and their piece reads obey the probe timeout. `go test -tags=nosqlite -race
./internal/torrent ./internal/metadata ./internal/api` verifies cancellation and
pack isolation; `npm run test:sources` from `web` tests score ordering and automatic
fallback in Firefox using simulated media events (`FIREFOX_PATH` optional).

The discovery layout follows the supplied desktop/mobile artwork references:
full-width featured artwork, a dark gradient, a responsive poster grid, and a glass search header. Mobile uses the featured season's portrait artwork; desktop
uses its banner. Discovery responses retain `media_id` and `media_poster_image`
alongside canonical franchise `id`/`poster_image`, so “Regarder” opens the season
shown while poster cards and “En savoir plus” open its franchise. The `/api/v1/catalog/seasonal` endpoint filters by the current calendar quarter
and year, keeping each release’s `media_title`, `media_id`, and
`media_poster_image`. Seasonal cards open that release’s episode list, rather
than the entire franchise. Season and episode pages share the same visual styles. With the frontend on port 4391, run from `web`
`npm run test:design` for desktop/mobile layout, scrolling, search, keyboard focus,
and featured-season navigation checks (`TEST_BASE_URL` can override the address).

The featured hero is selected independently from all-time popular released anime
with a banner, rather than from the seasonal grid. Seasonal card metadata appears
above its title. The grid picks a divisor of the displayed item count within the
viewport's column capacity, keeping all rows complete and recalculating on resize.

The fixed glass navigation and copyright footer are shared by the root layout,
including franchise, season, episode and search pages. Search always returns to
the catalog and theme selection persists across navigation. Seasonal cards sort
by their complete premiere date, earliest first; unconfirmed/partial dates follow.

Interface localization covers French and English, including the player, shared
navigation/footer, catalog/season pages, errors and accessibility labels. The UI
uses browser language preferences by default; the footer selector saves an explicit
choice in local storage and updates the document language and date formatting.
Titles, descriptions and subtitle/audio track names supplied by external catalogs
remain their original content. The footer clarifies that Gazes does not permanently
host videos; the streaming bridge and its ephemeral buffers remain unchanged.
`npm run test:i18n` exercises automatic detection, switching, persistence, navigation
and localized dates against a frontend on port 4392 (`TEST_BASE_URL` overrides it).
Player harnesses also support `PLAYWRIGHT_BROWSER=chromium` when Firefox is absent.

### Indexers and one-command deployment

With Docker Engine and Compose v2.24+ installed:

```sh
docker compose up -d --build --wait
```

`devenv up` runs the same Docker Compose command from the project root.
Docker Engine must be running and accessible to your user. Containers run detached;
use `docker compose down` to stop them. For native development with live reload,
run `go run -tags=nosqlite ./cmd/server` and `pnpm --prefix web dev` manually.

Open `http://SERVER:8080` (`GAZES_PORT` changes this port). The production
frontend, FFmpeg-enabled backend and Prowlarr start automatically. Initialization
creates a random API key in the persistent `prowlarr-config` volume. Provisioning
reuses existing AniDex and The Pirate Bay indexers or creates them from official
Prowlarr schemas. Missing definitions are skipped with a warning. Existing manual
settings, disabled indexers and unrelated indexers are preserved.

New indexers are created disabled with `forceSave=true`, then enabled with a
forced PUT: Prowlarr still tests enabled indexers on POST even with forceSave.
This sequence avoids external network tests during deployment. A private pending
checkpoint makes activation resumable if provisioning is interrupted. API errors
fail provisioning visibly; unavailable external sites do not prevent startup.
Gazes starts after successful provisioning and reads Torznab endpoints and the key
from a read-only internal volume, accessible only to its service user. Credentials
are not sent to the frontend or included in Gazes/bootstrap error messages.

Nyaa remains direct and independent. In manifest mode, AniDex and The Pirate Bay
are queried only through Prowlarr. EXT and MagnetDL are disabled by default; enable
an optional provider with `INDEXER_EXT_DISABLED=false` or a configured endpoint.
Individual provider settings may be placed in `.env` (excluded from builds):

- `INDEXER_<NAME>_DISABLED=true` disables the provider.
- `INDEXER_<NAME>_TORZNAB_URL` and `INDEXER_<NAME>_API_KEY` override its gateway.
- `INDEXER_<NAME>_URL` overrides a direct connector's URL.

Names: `EXT`, `MAGNETDL`, `ANIDEX`, `THEPIRATEBAY`. Native development without
`INDEXER_CONFIG_FILE` can still use direct AniDex/The Pirate Bay connectors.
Prowlarr is not published publicly. Optional local administration:

```sh
docker compose -f compose.yaml -f compose.admin.yaml up -d --build --wait
```

Access `http://127.0.0.1:9696` on the server or through an SSH tunnel. The fresh
installation uses External authentication; only enable this localhost override on
a trusted server. Existing authentication configuration is never replaced.

Queries retain the per-provider concurrency limit of two, four-second deadlines,
coalescing, bounded caches (256 entries/provider, two-minute positive results,
15-second empty results), and a 30-second failure cooldown. Results merge by
infohash; VF/VOSTFR ranking remains in the episode resolver. Prowlarr outages
return partial results from Nyaa rather than stopping Gazes. Real torrent and VF
availability still depend on upstream sites and swarms.

### Updating and backing up

```sh
docker compose pull
docker compose up -d --build --wait
```

BuildKit caches Go modules, pnpm dependencies and unchanged build layers. Images
use maintained upstream tags; `pull` deliberately upgrades Prowlarr. Re-running
provisioning does not create duplicate indexers. The backend's torrent buffers
remain ephemeral and are cleared when its container is recreated.

Back up Prowlarr while stopped to keep SQLite databases consistent:

```sh
docker compose stop prowlarr
docker compose run --rm --no-deps --entrypoint sh prowlarr-init -c 'tar -C /config -czf - .' > prowlarr-backup.tar.gz
docker compose up -d --wait
```

Keep the archive private: it contains the API key and configuration. Restore into
the config volume with services stopped, then run the normal command to regenerate
the internal manifest. `docker compose down` preserves volumes; `down -v` deletes
configuration and keys. Inspect startup with `docker compose logs -f`.

### Gateway verification

```sh
python3 -m unittest discover -s deploy -p 'test_*.py'
go test -tags=nosqlite -race ./internal/indexer/... ./internal/api
```

Tests cover initialization, persistent keys, manual settings, idempotence,
missing schemas, API failures, protected files, Torznab parsing, partial results,
infohash deduplication and French source preference.

### Persistent development diagnostics

The backend honors `LOG_LEVEL` (default `debug`). Sanitized structured events go
to stdout, timestamped JSONL files and `events.sqlite` in the persistent
`diagnostics` Compose volume. Its directory is mode 0700 and files are private to
the backend user. Browser playback events are correlated with backend requests,
provider searches and remux operations using session and attempt identifiers.

```sh
# Reconstruct the Tensura episode-one timeline.
docker compose exec backend gazes-logs --anime 101280 --season 101280 --episode 1 --limit 10000

# Look up the reference displayed by a failed player.
docker compose exec backend gazes-logs --session SESSION_ID --limit 10000

# Follow warnings, or export a complete session as JSONL.
docker compose exec backend gazes-logs --level WARN --follow
docker compose exec -T backend gazes-logs --session SESSION_ID --json --limit 100000 > session.jsonl

# Date filters use RFC3339 timestamps; all events are stored in UTC.
docker compose exec backend gazes-logs --since 2026-10-02T00:00:00Z --provider nyaa.si
```

Optional filters: `--request`, `--event`, `--service`, `--level`, `--anime`,
`--season`, `--episode`, `--provider`, `--session`, `--since` and `--until`.
`--limit` defaults to 1000; increase it for large exports. Follow mode reads new
persisted events every second. There is no HTTP endpoint for reading logs.

Configuration (set in `.env` for Docker; exported variables also work natively):

| Variable | Default | Purpose |
| --- | --- | --- |
| `LOG_LEVEL` | `debug` | Minimum severity: debug, info, warn, error |
| `LOG_DIR` | `/app/diagnostics` in Docker | JSONL and SQLite directory |
| `LOG_RETENTION` | `168h` | Event/file retention |
| `LOG_FILE_BYTES` | `52428800` | JSONL rotation size |
| `LOG_FILES_MAX_BYTES` | `1073741824` | JSONL storage budget |
| `LOG_DB_MAX_BYTES` | `1073741824` | SQLite budget, including reserved WAL space |

The Docker log directory is fixed to its mounted volume. The other retention and
size variables pass through `.env`. A single background writer uses a bounded
4096-event queue, groups SQLite inserts and flushes once per second or 200 events.
The bounded queue keeps disk/SQLite stalls off streaming request goroutines.
`diagnostics.events_dropped` reports saturation losses. A process killed abruptly
can lose events still in memory; a graceful shutdown drains the queue with a
five-second deadline. Files already written are replayed on startup/recovery
using checkpoints and unique event IDs, without duplicate rows.

A locked/unavailable SQLite database keeps JSONL output working; errors are
reported on stderr. A failed file sink retains stdout. Age/size cleanup runs on
startup and periodically, with WAL checkpoints and incremental vacuuming. API
keys, credentials, auth headers and magnets are masked before any sink. Messages
and subprocess stderr are bounded and marked when truncated. Browser events are
marked `client_reported`, validated against an allowlist, capped at 32 KiB per
batch and rate-limited; they are diagnostic evidence, not trusted server facts.

Local Go builds/tests use `-tags=nosqlite` to make the torrent engine use its
maintained BoltDB piece-completion backend. Diagnostic SQLite remains enabled.
This avoids linking two different bundled C SQLite runtimes into one binary.
`devenv` supplies this tag through `GOFLAGS`; Docker builds supply it explicitly.
No additional database service is needed.

```sh
go test -tags=nosqlite -race ./internal/... ./cmd/...
node web/scripts/episode-file.test.mjs
PLAYWRIGHT_BROWSER=chromium node web/scripts/auto-playback.test.mjs
TEST_BASE_URL=http://127.0.0.1:8080 node web/scripts/tensura-diagnostic.mjs
```

The live diagnostic writes `/tmp/gazes-tensura-browser.json` by default
(`DIAGNOSTIC_REPORT` overrides it). It reports success only when an actual video
with nonzero dimensions advances for at least 30 seconds. Browser executable
paths can be overridden with `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`.

The episode player now selects the requested file using `episodeFile`; an absent
or ambiguous match never silently selects the pack's main video. Errors distinguish
no matching torrents, unavailable providers and exhausted playback attempts,
with a searchable diagnostic reference. See [the verified Tensura report](deploy/TENSURA-DIAGNOSTIC.md).

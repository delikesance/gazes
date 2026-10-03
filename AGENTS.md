# AGENTS.md

> Operational guidelines, architectural invariants, and codebase conventions for AI agents working on the **Gazes** project.

## TypeSafe Skill

Use the installed TypeSafe skill at [.agents/skills/typesafe-ai/SKILL.md](.agents/skills/typesafe-ai/SKILL.md) when working on this project. Read its instructions and the relevant live TypeSafe documentation before implementing TypeSafe features. Keep API credentials server-side in environment variables and out of tracked files.

### Mandatory Jev Workflow for Codex

Jev is a required decision-support tool for agents working in this repository. Use an actual successful Jev MCP request before acting on any non-mechanical, bounded development decision, including diagnostic next steps, command selection when several viable commands exist, implementation options, UI choices, search-result triage, and pre-completion patch or claim review.

For each use, frame one bounded question from the evidence already gathered, provide two to six explicit candidates (including investigate or ask the user when appropriate), execute the selected option, and verify the result locally. Batch independent questions where their evidence is unchanged. Record the relevant outcome succinctly in the final handoff when it materially influenced the work.

Do not use Jev for deterministic work: reading a named file, performing exact calculations, applying an already-selected patch, running tests, enforcing authorization or safety checks, or executing an unambiguous command. Code remains responsible for those operations and for all safety and authorization enforcement. Never claim Jev was consulted without a successful response. If Jev is unavailable or its result is uncertain, state that limitation and proceed only with deterministic evidence or request direction.

---

## 🎯 Project Vision & Context

**Gazes** is a high-performance, cost-efficient video streaming platform that bridges standard BitTorrent swarms to web browsers using a hybrid approach:
1. **Lightweight Backend Streaming Bridge**: Connects to standard BitTorrent swarms (TCP/uTP/DHT), prioritizing pieces sequentially for instant playback.
2. **On-the-Fly Remuxing Pipeline**: Re-wraps video containers (MKV $\rightarrow$ fMP4/HLS) and transcodes non-browser audio (AC3/DTS $\rightarrow$ AAC) with minimal CPU usage.
3. **Browser WebRTC P2P Mesh**: Concurrent viewers sharing the same stream share video segments over WebRTC, dramatically offloading origin bandwidth.

---

## 🏛️ Core Architectural Invariants

Whenever proposing, generating, or refactoring code in this repository, you **MUST** adhere to the following core rules:

### 1. Remux Over Transcode (CPU Preservation)
* **Never** re-encode video tracks (`copy` video codec) unless the user or client explicitly requests quality downscaling.
* Most anime torrents (e.g., from Nyaa.si) use H.264 (`AVC`) or H.265 (`HEVC`) 8/10-bit. Browsers support these streams natively once remuxed from `.mkv` into fragmented `.mp4` (`fMP4`) or `.ts` / HLS chunks.
* Only transcode audio if incompatible with standard browser decoders (transcode multi-channel `AC3`/`EAC3`/`DTS`/`FLAC` into stereo `AAC` / `Opus`).
* **Subtitles & Audio Tracks**: Anime MKVs often bundle multiple audio tracks (Japanese / English dubs) and advanced styled subtitles (`.ass` / `.ssa`). Extract subtitles as `.vtt` or expose them via subtitle renderers (e.g., `libass` / WebAssembly subtitle octosub).

### 2. Sequential & Priority-Based Piece Scheduling
* Standard BitTorrent client heuristics (rarest-first) will cause stream buffering stalls.
* Always prioritize:
  1. **Metadata & Header/Index** (first and last pieces of the file, container headers like `moov atom` or `EBML/MKV header`).
  2. **Active Playback Window** (pieces $N$ to $N + K$ where $N$ is current seek offset).
  3. **Sliding Lookahead Buffer** (next 20–60 seconds of video chunks).
* When a seek event occurs, immediately cancel or de-prioritize older lookahead piece requests and shift the priority window to the new offset.

### 3. Ephemeral Sliding-Window Buffers (Disk & RAM Preservation)
* Do not store entire multi-gigabyte video torrents indefinitely on the server disk unless explicitly flagged for persistent caching.
* Utilize an LRU (Least Recently Used) cache or ring buffer for active streams. Unwatched pieces/torrents should be purged automatically when no clients are attached.

### 4. Robust Error Handling & Swarm Stalls
* Torrent swarms are volatile: peers drop, speeds fluctuate, or seeders become unavailable.
* Always handle read timeouts, slow swarm fallbacks, and connection resets gracefully. Provide clear health indicators (seeders, download speed, buffer percentage) to the frontend API.

---

## 📁 Repository Structure & Module Responsibilities

```text
gazes/
├── cmd/
│   ├── server/           # Main HTTP streaming & API server entrypoint
│   └── indexer/          # Torrent tracker scraping & metadata service
├── internal/
│   ├── torrent/          # Torrent client wrapper, sequential engine, piece priority scheduler
│   ├── stream/           # HTTP range handler, HLS packager, FFmpeg remuxing pipeline
│   ├── metadata/         # Torrent info parsing, video stream analysis (ffprobe), posters
│   ├── cache/            # Sliding buffer, chunk store, and LRU memory/disk eviction
│   └── config/           # App configuration and environment loading
├── web/                  # Web frontend (Next.js / React / TypeScript + Video Player)
├── tests/                # Integration & unit test suites
├── AGENTS.md             # This agent instruction file
└── README.md             # Project documentation
```

---

## 💻 Tech Conventions & Coding Standards

### Backend (Go / Rust)
* Keep streaming handlers non-blocking and memory-conscious. Use `io.Reader`, `io.Writer`, and piped streams (`io.Pipe`) instead of loading entire chunks or files into memory.
* Support standard HTTP `Range` headers (`206 Partial Content`) for native browser scrubbing.
* Structure concurrency cleanly using idiomatic context cancellation (`context.Context`), channels, or async tasks when streams are closed.

### Media & Transcoding Pipeline
* Ensure FFmpeg / remuxing child processes terminate immediately when the client disconnects or aborts the HTTP request.
* Use lightweight probes (`ffprobe`) to inspect video/audio codecs before initiating the stream pipeline.

### Frontend (TypeScript / React)
* Use modern video player integrations (Video.js, Plyr, or native HTML5 `<video>` with `hls.js`).
* Integrate `p2p-media-loader` (WebRTC P2P) smoothly with fallback to HTTP endpoints.
* Provide clean, responsive UI components with real-time stream status (Buffer % / Seeders / Speed).

---

## 🧪 Verification & Testing Strategy
* **Unit Tests**: Test piece priority calculators, byte-range math, and metadata parsers.
* **Integration Tests**: Verify HTTP Range request handling (`206 Partial Content`, `Content-Range`, `Accept-Ranges: bytes`).
* **Pipeline Tests**: Validate remuxing output against sample MKV/MP4 fixtures.

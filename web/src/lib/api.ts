import { httpError } from "@/lib/error-code";
import type { SkipSegment } from "./skip-segments";
import { diagnosticHeaders, diagnosticURL, type PlaybackDiagnostic } from "./diagnostics";
import {
  SearchResponse,
  LoadTorrentResponse,
  SwarmStats,
  VideoMetadata,
  CatalogResponse,
  AnimeCatalogItem,
  EpisodeSourcesResponse,
  ScheduleResponse,
  LibraryCopy,
} from "@/types/api";

export function getApiBase(): string {
  if (typeof window !== "undefined") {
    return process.env.NEXT_PUBLIC_API_BASE || "/api/v1";
  }
  return process.env.BACKEND_URL ? `${process.env.BACKEND_URL}/api/v1` : "http://127.0.0.1:8090/api/v1";
}

/**
 * API base for URLs that end up in markup the browser fetches (img src, ...). Unlike getApiBase() it never resolves
 * to the docker-internal BACKEND_URL, which would otherwise leak into server-rendered HTML.
 */
export function publicApiBase(): string {
  return process.env.NEXT_PUBLIC_API_BASE || "/api/v1";
}

/**
 * The backend answers an upstream (AniList) throttle with 503 + Retry-After. Keep the request pending
 * through up to two cooldowns of at most 30 s each, so the page shows its loader instead of an error.
 */
async function fetchRetryingThrottle(url: string, init?: RequestInit): Promise<Response> {
  let res = await fetch(url, init);
  for (let attempt = 0; attempt < 2 && res.status === 503; attempt++) {
    const wait = Number(res.headers.get("Retry-After"));
    if (!Number.isFinite(wait) || wait <= 0 || wait > 30) break;
    await new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, wait * 1000);
      init?.signal?.addEventListener("abort", () => { clearTimeout(timer); resolve(); }, { once: true });
    });
    if (init?.signal?.aborted) break;
    res = await fetch(url, init);
  }
  return res;
}

/** Catalog answers are kept for good by the backend: Next reuses them for 5 min (browsers follow the Cache-Control header). */
const CATALOG_CACHE = { next: { revalidate: 300 } } as const;

export async function getCatalogTrending(page: number = 1, perPage: number = 20): Promise<CatalogResponse> {
  const url = `${getApiBase()}/catalog/trending?page=${page}&per_page=${perPage}`;
  console.log("[API CLIENT] Fetching:", url);
  const res = await fetch(url, CATALOG_CACHE);
  console.log("[API CLIENT] Response status:", res.status, res.statusText);
  if (!res.ok) {
    throw new Error(`Failed to fetch trending anime: ${res.statusText}`);
  }
  const data = await res.json();
  console.log("[API CLIENT] Data parsed:", data?.items?.length, "items");
  return data;
}

export async function getCatalogPopular(page: number = 1, perPage: number = 20, signal?: AbortSignal): Promise<CatalogResponse> {
  const res = await fetchRetryingThrottle(`${getApiBase()}/catalog/popular?page=${page}&per_page=${perPage}`, { signal, ...CATALOG_CACHE });
  if (!res.ok) {
    throw httpError("Impossible de charger les animes populaires.", "CAT", res);
  }
  return res.json();
}

/**
 * Suggestions. Signed in, the server profiles the stored watch log; otherwise the visitor's local
 * sessions travel in the body so their device history still shapes the feed. `seedIds` is the fallback.
 */
export async function getCatalogForYou(seedIds: number[], sessions: unknown[] = [], hidden: number[] = [], page: number = 1, perPage: number = 24, signal?: AbortSignal): Promise<CatalogResponse> {
  const ids = seedIds.length ? `&ids=${seedIds.join(",")}` : "";
  const res = await fetchRetryingThrottle(`${getApiBase()}/catalog/foryou?page=${page}&per_page=${perPage}${ids}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ sessions, hidden }),
    signal,
  });
  if (!res.ok) {
    throw httpError("Impossible de charger vos suggestions.", "CAT", res);
  }
  return res.json();
}

export async function searchCatalog(
  query: string = "",
  genre: string = "",
  page: number = 1,
  perPage: number = 24,
  exclude: string = ""
): Promise<CatalogResponse> {
  const params = new URLSearchParams();
  if (query) params.set("q", query);
  // `genre` holds the wanted genres/tags, comma separated; `exclude` the unwanted ones.
  if (genre) params.set("genres", genre);
  if (exclude) params.set("exclude", exclude);
  if (page > 1) params.set("page", page.toString());
  if (perPage !== 24) params.set("per_page", perPage.toString());

  const res = await fetchRetryingThrottle(`${getApiBase()}/catalog/search?${params.toString()}`, CATALOG_CACHE);
  if (!res.ok) {
    throw httpError("Impossible de charger le catalogue.", "CAT", res);
  }
  return res.json();
}

export async function getAnimeCatalogDetail(id: number): Promise<AnimeCatalogItem> {
  const res = await fetch(`${getApiBase()}/catalog/anime/${id}`, CATALOG_CACHE);
  if (!res.ok) {
    throw new Error(`Failed to fetch anime details: ${res.statusText}`);
  }
  return res.json();
}

export async function getEpisodeSources(
  id: number,
  episodeNum: number,
  customTitle?: string
): Promise<EpisodeSourcesResponse> {
  let url = `${getApiBase()}/catalog/anime/${id}/episodes/${episodeNum}/sources`;
  if (customTitle) {
    url += `?title=${encodeURIComponent(customTitle)}`;
  }
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`Failed to resolve episode sources: ${res.statusText}`);
  }
  return res.json();
}

export async function searchAnime(
  query: string = "",
  category: string = "1_2",
  sort: string = "seeders",
  page: number = 1
): Promise<SearchResponse> {
  const params = new URLSearchParams();
  if (query) params.set("q", query);
  if (category) params.set("category", category);
  if (sort) params.set("sort", sort);
  if (page > 1) params.set("page", page.toString());

  const res = await fetch(`${getApiBase()}/search?${params.toString()}`);
  if (!res.ok) {
    throw new Error(`Failed to search: ${res.statusText}`);
  }
  return res.json();
}

export async function getLatestAnime(
  category: string = "1_2",
  page: number = 1
): Promise<SearchResponse> {
  const params = new URLSearchParams();
  if (category) params.set("category", category);
  if (page > 1) params.set("page", page.toString());

  const res = await fetch(`${getApiBase()}/latest?${params.toString()}`);
  if (!res.ok) {
    throw new Error(`Failed to fetch latest: ${res.statusText}`);
  }
  return res.json();
}

export async function loadTorrent(magnetURI: string, options: {metadataOnly?: boolean; signal?: AbortSignal; diagnostic?:PlaybackDiagnostic; prewarm?: boolean} = {}): Promise<LoadTorrentResponse> {
  const res = await fetch(`${getApiBase()}/torrent/load`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...diagnosticHeaders(options.diagnostic), ...(options.prewarm ? { "X-Gazes-Prewarm": "1" } : {}) },
    body: JSON.stringify({ magnet: magnetURI, metadata_only: options.metadataOnly }),
 signal: options.signal,
  });
  if (!res.ok) {
    throw new Error(`Failed to load torrent metadata: ${res.statusText}`);
  }
  return res.json();
}

export async function fetchVideoMetadata(infoHash: string, fileIdx: number = 0, signal?: AbortSignal, diagnostic?:PlaybackDiagnostic): Promise<VideoMetadata> {
  const res = await fetch(`${getApiBase()}/metadata?ih=${encodeURIComponent(infoHash)}&file_idx=${fileIdx}`, { signal, headers:diagnosticHeaders(diagnostic) });
  if (!res.ok) {
    throw new Error(`Failed to fetch video metadata: ${res.statusText}`);
  }
  return res.json();
}

export async function getTorrentStats(infoHash: string, diagnostic?:PlaybackDiagnostic): Promise<SwarmStats> {
  const res = await fetch(`${getApiBase()}/torrent/stats?ih=${encodeURIComponent(infoHash)}`,{headers:diagnosticHeaders(diagnostic)});
  if (!res.ok) {
    throw new Error(`Failed to get torrent stats: ${res.statusText}`);
  }
  return res.json();
}

export function getStreamUrl(
  infoHash: string,
  fileIdx: number = 0,
  remux: boolean = false,
  timeOffset: number = 0,
  audioTrack: number = 0,
 diagnostic?:PlaybackDiagnostic
): string {
  let url = `${getApiBase()}/stream?ih=${encodeURIComponent(infoHash)}&file_idx=${fileIdx}${remux ? "&remux=true" : ""}`;
  if (timeOffset > 0) {
    url += `&time_offset=${timeOffset}`;
  }
  if (audioTrack > 0) {
    url += `&audio_track=${audioTrack}`;
  }
  return diagnosticURL(url,diagnostic);
}

export function getSubtitleUrl(infoHash: string, fileIdx: number = 0, trackIdx: number = 0, format: "webvtt" | "ass" | "sup" = "webvtt",diagnostic?:PlaybackDiagnostic): string {
  return diagnosticURL(`${getApiBase()}/subtitles?ih=${encodeURIComponent(infoHash)}&file_idx=${fileIdx}&track_idx=${trackIdx}&format=${format}`,diagnostic);
}

export function formatBytes(bytes: number, decimals: number = 2): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + " " + sizes[i];
}

export async function getFranchise(id: number, signal?: AbortSignal): Promise<import("@/types/api").Franchise> {
 return catalogFetch(`/catalog/anime/${id}/franchise`, signal, undefined, true);
}
export async function getSeason(id: number, season: number, signal?: AbortSignal): Promise<AnimeCatalogItem> {
 return catalogFetch(`/catalog/anime/${id}/seasons/${season}`, signal, undefined, true);
}
export async function getSeasonSources(id: number, season: number, episode: number, signal?: AbortSignal, session?:string, discovery: 'fast' | 'full' = 'fast'): Promise<EpisodeSourcesResponse> {
 return catalogFetch(`/catalog/anime/${id}/seasons/${season}/episodes/${episode}/sources${discovery==='full'?'?discovery=full':''}`, signal,session?{playback_session_id:session}:undefined);
}
async function catalogFetch<T>(path: string, signal?: AbortSignal,diagnostic?:PlaybackDiagnostic, cacheable = false): Promise<T> {
 const response = await fetch(`${getApiBase()}${path}`, { signal,headers:diagnosticHeaders(diagnostic), ...(cacheable ? CATALOG_CACHE : {}) });
 if (!response.ok) {
  const body=await response.json().catch(()=>({}));
  const message=response.status===404?"Cette saison ou cet épisode est introuvable.":"Les fournisseurs de torrents sont temporairement indisponibles.";
  throw httpError(message, "SRC", response, body.playback_session_id||diagnostic?.playback_session_id);
 }
 return response.json();
}

export async function getSchedule(from: number, to: number, signal?: AbortSignal): Promise<ScheduleResponse> {
 const response = await fetch(`${getApiBase()}/catalog/schedule?from=${from}&to=${to}`, { signal, ...CATALOG_CACHE });
 if (!response.ok) throw httpError("Impossible de charger le calendrier des sorties.", "CAL", response);
 return response.json();
}

export async function getCatalogSeasonal(page = 1, perPage = 24): Promise<CatalogResponse> {
 const response = await fetch(`${getApiBase()}/catalog/seasonal?page=${page}&per_page=${perPage}`, CATALOG_CACHE);
 if (!response.ok) throw httpError("Impossible de charger les sorties de cette saison.", "CAT", response);
 return response.json();
}

/** Frame cut from the middle of the episode once someone has played it; 404 until then. */
export function episodePreviewUrl(seasonId: number, episode: number): string {
  // Rendered into <img src>, also during SSR: the browser must be able to resolve it.
  return `${publicApiBase()}/catalog/seasons/${seasonId}/episodes/${episode}/preview`;
}

/**
 * Asks the backend to cut the preview from the file being played, then waits for it to appear.
 * Resolves true when this call got a frame generated, false when one already existed or none came.
 */
export async function requestEpisodePreview(seasonId: number, episode: number, infoHash: string, fileIndex: number, duration: number, alive: () => boolean = () => true): Promise<boolean> {
  if (!Number.isFinite(duration) || duration < 120) return false;
  const url = episodePreviewUrl(seasonId, episode);
  const query = new URLSearchParams({ ih: infoHash, file_idx: String(fileIndex), duration: String(Math.round(duration)) });
  try {
    const started = await fetch(`${url}?${query}`, { method: "POST", keepalive: true });
    if (started.status !== 202) return false;
    for (let attempt = 0; attempt < 15 && alive(); attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 4000));
      if (!alive()) return false;
      if ((await fetch(url, { cache: "no-store" })).ok) return true;
    }
  } catch { /* the preview is a nicety: stay silent */ }
  return false;
}

/** Opening/ending times for a season episode from the backend; any failure resolves to none. */
export async function getSkipTimes(seasonId: number, episode: number, duration: number, signal?: AbortSignal): Promise<SkipSegment[]> {
  try {
    const query = new URLSearchParams({ duration: String(Math.round(duration)) });
    const res = await fetch(`${getApiBase()}/catalog/seasons/${seasonId}/episodes/${episode}/skip-times?${query}`, { signal });
    if (!res.ok) return [];
    const body = (await res.json()) as { segments?: SkipSegment[] };
    return Array.isArray(body.segments) ? body.segments : [];
  } catch { /* skipping is a nicety: stay silent */ }
  return [];
}

/** Copies of an episode cached by the server library; any failure means "none", never an error. */
export type { LibraryCopy };

export async function getLibraryCopies(seasonId: number, episode: number, signal?: AbortSignal): Promise<LibraryCopy[]> {
  try {
    const res = await fetch(`${getApiBase()}/library/episodes/${seasonId}/${episode}`, { signal, credentials: "same-origin" });
    if (!res.ok) return [];
    const data = await res.json();
    return Array.isArray(data?.copies) ? data.copies : [];
  } catch {
    return [];
  }
}

/** Offers a torrent file that just played for caching; the server validates it and every error is ignored. */
export async function registerLibraryCopy(
  seasonId: number,
  episode: number,
  lang: 'vf' | 'vostfr',
  body: { info_hash: string; file_index: number; release_name: string; anime_id: number; title: string },
): Promise<void> {
  try {
    await fetch(`${getApiBase()}/library/episodes/${seasonId}/${episode}/${lang}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify(body),
    });
  } catch {
    /* best effort */
  }
}

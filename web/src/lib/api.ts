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
} from "@/types/api";

export function getApiBase(): string {
  if (typeof window !== "undefined") {
    return process.env.NEXT_PUBLIC_API_BASE || "/api/v1";
  }
  return process.env.BACKEND_URL ? `${process.env.BACKEND_URL}/api/v1` : "http://127.0.0.1:8090/api/v1";
}

export async function getCatalogTrending(page: number = 1, perPage: number = 20): Promise<CatalogResponse> {
  const url = `${getApiBase()}/catalog/trending?page=${page}&per_page=${perPage}`;
  console.log("[API CLIENT] Fetching:", url);
  const res = await fetch(url);
  console.log("[API CLIENT] Response status:", res.status, res.statusText);
  if (!res.ok) {
    throw new Error(`Failed to fetch trending anime: ${res.statusText}`);
  }
  const data = await res.json();
  console.log("[API CLIENT] Data parsed:", data?.items?.length, "items");
  return data;
}

export async function getCatalogPopular(page: number = 1, perPage: number = 20): Promise<CatalogResponse> {
  const res = await fetch(`${getApiBase()}/catalog/popular?page=${page}&per_page=${perPage}`);
  if (!res.ok) {
    throw new Error(`Failed to fetch popular anime: ${res.statusText}`);
  }
  return res.json();
}

export async function searchCatalog(
  query: string = "",
  genre: string = "",
  page: number = 1,
  perPage: number = 24
): Promise<CatalogResponse> {
  const params = new URLSearchParams();
  if (query) params.set("q", query);
  if (genre) params.set("genre", genre);
  if (page > 1) params.set("page", page.toString());
  if (perPage !== 24) params.set("per_page", perPage.toString());

  const res = await fetch(`${getApiBase()}/catalog/search?${params.toString()}`);
  if (!res.ok) {
    throw new Error(`Failed to search anime catalog: ${res.statusText}`);
  }
  return res.json();
}

export async function getAnimeCatalogDetail(id: number): Promise<AnimeCatalogItem> {
  const res = await fetch(`${getApiBase()}/catalog/anime/${id}`);
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
 return catalogFetch(`/catalog/anime/${id}/franchise`, signal);
}
export async function getSeason(id: number, season: number, signal?: AbortSignal): Promise<AnimeCatalogItem> {
 return catalogFetch(`/catalog/anime/${id}/seasons/${season}`, signal);
}
export async function getSeasonSources(id: number, season: number, episode: number, signal?: AbortSignal, session?:string): Promise<EpisodeSourcesResponse> {
 return catalogFetch(`/catalog/anime/${id}/seasons/${season}/episodes/${episode}/sources`, signal,session?{playback_session_id:session}:undefined);
}
async function catalogFetch<T>(path: string, signal?: AbortSignal,diagnostic?:PlaybackDiagnostic): Promise<T> {
 const response = await fetch(`${getApiBase()}${path}`, { signal,headers:diagnosticHeaders(diagnostic) });
 if (!response.ok) {
  const body=await response.json().catch(()=>({}));
  const message=response.status===404?"Cette saison ou cet épisode est introuvable.":"Les fournisseurs de torrents sont temporairement indisponibles.";
  throw Object.assign(new Error(message),{diagnosticReference:body.playback_session_id||response.headers.get('X-Playback-Session-ID')||diagnostic?.playback_session_id});
 }
 return response.json();
}

export async function getSchedule(from: number, to: number, signal?: AbortSignal): Promise<ScheduleResponse> {
 const response = await fetch(`${getApiBase()}/catalog/schedule?from=${from}&to=${to}`, { signal });
 if (!response.ok) throw new Error("Impossible de charger le calendrier des sorties.");
 return response.json();
}

export async function getCatalogSeasonal(page = 1, perPage = 24): Promise<CatalogResponse> {
 const response = await fetch(`${getApiBase()}/catalog/seasonal?page=${page}&per_page=${perPage}`);
 if (!response.ok) throw new Error("Impossible de charger les sorties de cette saison.");
 return response.json();
}

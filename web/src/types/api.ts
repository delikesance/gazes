export interface EpisodeInfo {
 airing_at?: number;
 upcoming?: boolean;
  episode_number: number;
  title: string;
  thumbnail?: string;
  url?: string;
  site?: string;
  summary?: string;
}

export interface AnimeSeason {
 group: "main" | "movies" | "extras";
 season_number?: number;
 release_season_number?: number;
 start_date?: string;
  id: number;
  title: string;
  season_name: string;
  format?: string;
  episodes?: number;
  poster_image?: string;
  season_year?: number;
  status?: string;
  is_current: boolean;
}

export interface AnimeRelation {
  id: number;
  title_english?: string;
  title_romaji?: string;
  display_title: string;
  format?: string;
  relation_type: string; // "PREQUEL" | "SEQUEL" | "SIDE_STORY" | "ALTERNATIVE" | "PARENT" | "SPIN_OFF" | etc.
  season_year?: number;
  season?: string;
  episodes?: number;
  poster_image?: string;
  status?: string;
}

export interface AnimeCatalogItem {
  available_episodes?: number;
  start_date?: string;
  id: number;
  media_id?: number;
  media_title?: string;
  media_poster_image?: string;
  title_english?: string;
  title_romaji?: string;
  title_native?: string;
  display_title: string;
  franchise_title?: string;
  poster_image?: string;
  banner_image?: string;
  description?: string;
  genres?: string[];
  episodes?: number;
  average_score?: number;
  season_year?: number;
  status?: string;
  episode_list?: EpisodeInfo[];
  relations?: AnimeRelation[];
  seasons?: AnimeSeason[];
}

export interface CatalogResponse {
 season?: string;
 season_year?: number;
 partial?: boolean;
 warning?: string;
 has_next_page: boolean;
  page: number;
  per_page: number;
  total?: number;
  items: AnimeCatalogItem[];
}

export type LanguageTag = "VF" | "VOSTFR" | "MULTI" | "VOSTEN" | "RAW" | "OTHER";

export interface EpisodeSource extends TorrentItem {
 provider?: string;
 anime_title?: string;
 anime_aliases?: string[];
 excluded_titles?: string[];
 language_flags: LanguageTag[];
 french_evidence: string;
 score_breakdown: { french: number; multi: number; swarm: number; leechers: number; quality: number };
 is_batch: boolean;
 season_number: number;
 absolute_episode?: number;
 tagged_episode?: number;
  episode_number: number;
  language_tag: LanguageTag;
  language_label: string;
  release_group: string;
  quality: string;
  is_french: boolean;
  score_rank: number;
}

export interface EpisodeSourcesResponse {
 request_id?: string;
 playback_session_id?: string;
 partial?: boolean;
 warning?: string;
  anime_title: string;
  episode_number: number;
  total_sources: number;
  french_sources: number;
  sources: EpisodeSource[];
}

export interface AnimeDetails {
  id?: number;
  title_english?: string;
  title_romaji?: string;
  title_native?: string;
  display_title: string;
  poster_image?: string;
  banner_image?: string;
  description?: string;
  genres?: string[];
  episodes?: number;
  average_score?: number;
  season_year?: number;
}

export interface TorrentItem {
  id: string;
  title: string;
  info_hash: string;
  magnet_uri: string;
  torrent_url?: string;
  size_bytes: number;
  size_display: string;
  seeders: number;
  leechers: number;
  downloads: number;
  category: string;
  publish_date: string;
  anime_details?: AnimeDetails;
}

export interface AnimeGroup {
  id: string;
  title: string;
  anime_details?: AnimeDetails;
  releases: TorrentItem[];
  release_count: number;
  max_seeders: number;
  best_release?: TorrentItem;
  qualities: string[];
  release_groups: string[];
}

export interface SearchResponse {
  count: number;
  query: string;
  items: TorrentItem[];
  groups?: AnimeGroup[];
}

export interface FileInfo {
  index: number;
  path: string;
  length: number;
  is_video: boolean;
  mime_type: string;
}

export interface AudioTrack {
  index: number;
  stream_index: number;
  language: string;
  title: string;
  codec: string;
  channels: number;
  is_default: boolean;
}

export interface SubtitleTrack {
  index: number;
  stream_index: number;
  language: string;
  title: string;
  codec: string;
  is_default: boolean;
  is_forced: boolean;
}

export interface VideoMetadata {
  duration_sec: number;
  formatted_duration: string;
  width: number;
  height: number;
  resolution: string;
  video_codec: string;
  audio_codec: string;
  bitrate_bps: number;
  total_bytes: number;
  audio_tracks?: AudioTrack[];
  subtitle_tracks?: SubtitleTrack[];
}

export interface LoadTorrentResponse {
  info_hash: string;
  files: FileInfo[];
  main_video_index: number;
  main_video_metadata?: VideoMetadata;
}

export interface SwarmStats {
  info_hash: string;
  title: string;
  total_bytes: number;
  completed_bytes: number;
  progress_pct: number;
  download_rate_bps: number;
  upload_rate_bps: number;
  total_peers: number;
  active_seeders: number;
}

export interface Franchise { id: number; title: string; description?: string; poster_image?: string; seasons: AnimeSeason[]; complete: boolean; warning?: string; }

export interface ScheduleEntry {
  airing_at: number;
  episode: number;
  media_id: number;
  title: string;
  poster_image?: string;
  genres?: string[];
  studio?: string;
  format?: string;
  episodes?: number;
}

export interface ScheduleResponse {
  from: number;
  to: number;
  partial?: boolean;
  entries: ScheduleEntry[];
}

import type { EpisodeSource } from '../types/api';

/** Mirror language priority, season-pack preference and quality fallback. */
 // Which codecs this browser cannot decode; checked once, empty outside a browser (tests, SSR).
let undecodable: RegExp | null | undefined;
function undecodableCodecs(): RegExp | null {
 if (undecodable !== undefined) return undecodable;
 if (typeof document === "undefined") return undecodable = null;
 const video = document.createElement("video");
 const names: string[] = [];
 if (!video.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"')) names.push("x265", "h\\.?265", "hevc");
 if (!video.canPlayType('video/mp4; codecs="av01.0.05M.08"')) names.push("av1");
 return undecodable = names.length ? new RegExp(`\\b(?:${names.join("|")})\\b`, "i") : null;
}
/** A release whose title advertises a codec this browser cannot play would only fail and trigger a failover. */
const unplayable = (source: EpisodeSource) => Number(undecodableCodecs()?.test(source.title) ?? false);

/** Mirror of the backend LanguageTier: VF > VOSTFR > unconfirmed MULTI (never French points) > other. */
function languageTier(source: EpisodeSource): number {
 const breakdown = source.score_breakdown;
 if (!breakdown) return source.language_tag==='VF'?200:source.language_tag==='VOSTFR'?100:0;
 return breakdown.french > 0 ? breakdown.french : (breakdown.multi ?? 0) > 0 ? 75 : 0;
}

export function playbackSources(sources: EpisodeSource[]): EpisodeSource[] {
 const seen = new Set<string>();
 return [...sources].sort((a,b) =>
  unplayable(a) - unplayable(b) ||
  Number(b.seeders > 0) - Number(a.seeders > 0) ||
  languageTier(b) - languageTier(a) ||
  Number(b.is_batch) - Number(a.is_batch) ||
  (b.score_breakdown?.quality ?? 0) - (a.score_breakdown?.quality ?? 0) ||
  b.score_rank - a.score_rank || b.seeders - a.seeders ||
  (a.info_hash || a.id).localeCompare(b.info_hash || b.id)
 ).filter(source => {
  const identity = (source.info_hash || source.magnet_uri || source.id).toLowerCase();
  if (!identity || seen.has(identity)) return false;
  seen.add(identity);
  return true;
 });
}

/** Keep the current and previously attempted sources stable; rank only pending fallbacks. */
export function extendPlaybackSources(current: EpisodeSource[], discovered: EpisodeSource[], activeIndex: number): EpisodeSource[] {
 const fixed = current.slice(0, Math.min(activeIndex + 1, current.length));
 const hashes = new Set(fixed.map(source => (source.info_hash || source.id).toLowerCase()));
 return [...fixed, ...playbackSources([...current.slice(fixed.length), ...discovered]).filter(source => !hashes.has((source.info_hash || source.id).toLowerCase()))];
}

/**
 * Idle windows, not deadlines: a source is only abandoned after this long with no swarm activity
 * (no connected peer while fetching metadata, no new bytes while starting or stalled). A big file on
 * a slow swarm may take as long as it needs; `max` is only a safety net against a source that is
 * "alive" but never delivers.
 */
export const PLAYBACK_TIMEOUTS = { metadata: 15_000, startup: 15_000, stall: 30_000, max: 600_000 };

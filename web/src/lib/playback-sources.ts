import type { EpisodeSource } from '../types/api';

/** Mirror resolver ranking, de-duplicate magnets, and prefer smaller releases
 * only when score and swarm strength are equal. */
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

export function playbackSources(sources: EpisodeSource[]): EpisodeSource[] {
 const seen = new Set<string>();
 return [...sources].sort((a,b) =>
  unplayable(a) - unplayable(b) ||
  Number(b.seeders > 0) - Number(a.seeders > 0) ||
  b.score_rank - a.score_rank || b.seeders - a.seeders ||
  Number(a.is_batch) - Number(b.is_batch) ||
  (a.info_hash || a.id).localeCompare(b.info_hash || b.id)
 ).filter(source => {
  const identity = (source.info_hash || source.magnet_uri || source.id).toLowerCase();
  if (!identity || seen.has(identity)) return false;
  seen.add(identity);
  return true;
 });
}

export const PLAYBACK_TIMEOUTS = { metadata: 12_000, startup: 20_000, stall: 15_000 };

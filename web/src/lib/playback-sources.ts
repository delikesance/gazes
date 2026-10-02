import type { EpisodeSource } from '../types/api';

/** Mirror resolver ranking, de-duplicate magnets, and prefer smaller releases
 * only when score and swarm strength are equal. */
export function playbackSources(sources: EpisodeSource[]): EpisodeSource[] {
 const seen = new Set<string>();
 return [...sources].sort((a,b) =>
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

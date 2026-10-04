import type { EpisodeSource, LibraryCopy } from '../types/api';

export const AV1_MIME = 'video/mp4; codecs="av01.0.08M.10"';

export type LibraryLang = 'vf' | 'vostfr';

/** Language a torrent source would be cached under; null when it is never cached. */
export function libraryLang(source: EpisodeSource): LibraryLang | null {
  if (source.language_tag === 'VF' || source.language_tag === 'MULTI') return 'vf';
  if (source.language_tag === 'VOSTFR') return 'vostfr';
  return null;
}

export function canPlayCopy(copy: LibraryCopy, isTypeSupported: (mime: string) => boolean): boolean {
  if (copy.video_codec !== 'av1') return true;
  return isTypeSupported(AV1_MIME);
}

/** A library copy shaped like a torrent source so the player and the failover logic treat it uniformly. */
export function librarySource(copy: LibraryCopy, base: Partial<EpisodeSource>): EpisodeSource {
  const tag = copy.lang === 'vf' ? 'VF' : 'VOSTFR';
  return {
    id: copy.stream_id,
    title: base.title || 'Bibliothèque',
    size_bytes: 0,
    size_display: '',
    leechers: 0,
    downloads: 0,
    category: '',
    publish_date: '',
    language_flags: [tag],
    french_evidence: '',
    score_breakdown: { french: 0, multi: 0, swarm: 0, leechers: 0, quality: 0 },
    is_batch: false,
    season_number: 0,
    episode_number: 0,
    language_label: tag,
    release_group: 'Bibliothèque',
    quality: copy.video_codec.toUpperCase(),
    score_rank: 0,
    ...base,
    info_hash: copy.stream_id,
    magnet_uri: '',
    language_tag: tag,
    is_french: true,
    seeders: 9999,
    library: { stream_id: copy.stream_id, lang: copy.lang, video_codec: copy.video_codec, duration_ms: copy.duration_ms },
  };
}

/**
 * Inserts each playable copy just before the first ranked source of its language (appended when there is none),
 * so a library copy never outranks a torrent in a better language.
 */
export function withLibraryCandidates(
  ranked: EpisodeSource[],
  copies: LibraryCopy[],
  isTypeSupported: (mime: string) => boolean,
  base: Partial<EpisodeSource> = {},
): EpisodeSource[] {
  const out = [...ranked];
  for (const copy of copies) {
    if (!canPlayCopy(copy, isTypeSupported)) continue;
    if (out.some((s) => s.info_hash === copy.stream_id)) continue;
    const at = out.findIndex((s) => !s.library && libraryLang(s) === copy.lang);
    const source = librarySource(copy, { ...base, episode_number: base.episode_number ?? 0 });
    if (at < 0) out.push(source);
    else out.splice(at, 0, source);
  }
  return out;
}

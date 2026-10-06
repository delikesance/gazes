import type { EpisodeSource, LibraryCopy } from '../types/api';

export const AV1_MIME = 'video/mp4; codecs="av01.0.08M.10"';

export type LibraryLang = 'vf' | 'vostfr';

/** Language a torrent source would be cached under; null when it is never cached. */
export function libraryLang(source: EpisodeSource): LibraryLang | null {
  if (source.language_tag === 'VF' || source.language_tag === 'MULTI') return 'vf';
  if (source.language_tag === 'VOSTFR') return 'vostfr';
  return null;
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
 * Inserts each copy (an AV1 copy is transcoded server-side for browsers that cannot decode it) just before the first ranked source of its language (appended when there is none),
 * so a library copy never outranks a torrent in a better language.
 */
export function withLibraryCandidates(
  ranked: EpisodeSource[],
  copies: LibraryCopy[],
  base: Partial<EpisodeSource> = {},
): EpisodeSource[] {
  const out = [...ranked];
  for (const copy of copies) {
    if (out.some((s) => s.info_hash === copy.stream_id)) continue;
    const at = out.findIndex((s) => !s.library && libraryLang(s) === copy.lang);
    const source = librarySource(copy, { ...base, episode_number: base.episode_number ?? 0 });
    if (at < 0) out.push(source);
    else out.splice(at, 0, source);
  }
  return out;
}

/** Resolves with the lookup's copies, or with [] once `ms` elapsed or the lookup failed; never rejects, never hangs. */
export function lookupWithin(ms: number, lookup: (signal: AbortSignal) => Promise<LibraryCopy[]>): Promise<{ copies: LibraryCopy[]; timedOut: boolean }> {
  const controller = new AbortController();
  return new Promise((resolve) => {
    const timer = setTimeout(() => { controller.abort(); resolve({ copies: [], timedOut: true }); }, ms);
    let pending: Promise<LibraryCopy[]>;
    try { pending = lookup(controller.signal); } catch { pending = Promise.resolve([]); }
    pending.then(
      (copies) => { clearTimeout(timer); resolve({ copies: Array.isArray(copies) ? copies : [], timedOut: false }); },
      () => { clearTimeout(timer); resolve({ copies: [], timedOut: false }); },
    );
  });
}

export type SourcesState = 'pending' | 'loaded' | 'failed';

export interface WatchData<F, Se, S> { franchise: F; season: Se | undefined; copies: LibraryCopy[]; sources: S | undefined; sourcesState: SourcesState }

/**
 * Gathers what the watch page needs without letting the (slow) source search delay a local replay.
 * `copies` must be a bounded lookup (see lookupWithin). When the library has a copy, `ready` fires as soon as
 * the catalog and the library answered; `sourcesState` is then 'pending' while the search is in flight and the
 * outcome arrives through `hooks.sources` (success) or `hooks.sourcesFailed` (failure, 'failed' in `ready` if
 * already known). With no copy it behaves as before: `ready` waits for the sources and their failure is thrown.
 */
export async function loadWatchData<F, Se, S>(
  parts: { franchise: Promise<F>; season: Promise<Se | undefined>; sources: Promise<S | undefined>; copies: Promise<LibraryCopy[]> },
  hooks: { ready: (data: WatchData<F, Se, S>) => void; sources: (sources: S | undefined) => void; sourcesFailed?: (error: unknown) => void },
): Promise<void> {
  type Outcome = { ok: true; value: S | undefined } | { ok: false; error: unknown };
  let settled: Outcome | null = null;
  const outcome: Promise<Outcome> = parts.sources.then(
    (value): Outcome => (settled = { ok: true, value }),
    (error): Outcome => (settled = { ok: false, error }),
  );
  const [franchise, season, copies] = await Promise.all([parts.franchise, parts.season, parts.copies.catch((): LibraryCopy[] => [])]);
  const early = settled as Outcome | null;
  if (copies.length === 0 || early) {
    const o = early ?? await outcome;
    if (!o.ok) {
      if (copies.length === 0) throw o.error;
      hooks.ready({ franchise, season, copies, sources: undefined, sourcesState: 'failed' });
      return;
    }
    hooks.ready({ franchise, season, copies, sources: o.value, sourcesState: 'loaded' });
    return;
  }
  hooks.ready({ franchise, season, copies, sources: undefined, sourcesState: 'pending' });
  const o = await outcome;
  if (o.ok) hooks.sources(o.value);
  else hooks.sourcesFailed?.(o.error);
}

/** What the player shows while it has no source to play: the pending notice only while the search is in flight. */
export function playerWaitState(input: { hasSource: boolean; sourcesState: SourcesState }): 'playing' | 'pending' | 'exhausted' {
  if (input.hasSource) return 'playing';
  return input.sourcesState === 'pending' ? 'pending' : 'exhausted';
}

/** Torrent sources worth warming up behind the one playing; none while a library copy plays (it needs no swarm). */
export function prewarmTargets(candidates: EpisodeSource[], index: number): EpisodeSource[] {
  if (candidates[index]?.library) return [];
  return candidates.slice(index + 1, index + 3).filter((next) => !next.library);
}

/** The loaded data only when it belongs to the current request (episode and retry counter); never a stale one. */
export function currentFor<T extends { key: string }>(loaded: T | null, requestKey: string): T | null {
  return loaded && loaded.key === requestKey ? loaded : null;
}

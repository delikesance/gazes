export interface SkipSegment {
  kind: 'opening' | 'ending';
  start: number;
  end: number;
  source: 'chapters' | 'aniskip';
}

/** Chapter segments win per kind; AniSkip only fills the kinds they lack. Sorted by start. */
export function mergeSkipSegments(fromChapters: SkipSegment[], fromAniSkip: SkipSegment[]): SkipSegment[] {
  const have = new Set(fromChapters.map((s) => s.kind));
  return [...fromChapters, ...fromAniSkip.filter((s) => !have.has(s.kind))].sort((a, b) => a.start - b.start);
}

/** Segment under the playhead; it goes away in its last second. */
export function activeSkipSegment(segments: SkipSegment[], position: number): SkipSegment | null {
  return segments.find((s) => s.start <= position && position < s.end - 1) ?? null;
}

/** True while an opening or an ending is still missing. */
export function needsAniSkip(fromChapters: SkipSegment[]): boolean {
  return !(fromChapters.some((s) => s.kind === 'opening') && fromChapters.some((s) => s.kind === 'ending'));
}

/** An ending that runs to the end of the file skips to the next episode when there is one. */
export function skipAction(segment: SkipSegment, totalDuration: number, hasNextEpisode: boolean): 'seek' | 'next-episode' {
  return segment.kind === 'ending' && segment.end >= totalDuration - 5 && hasNextEpisode ? 'next-episode' : 'seek';
}

/** Moves segments by -offset (HLS timeline origin) without mutating the input. */
export function shiftSegments(segments: SkipSegment[], offset: number): SkipSegment[] {
  return segments.map((s) => ({ ...s, start: s.start - offset, end: s.end - offset }));
}

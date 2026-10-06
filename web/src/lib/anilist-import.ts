export interface AniListEntry { id: number; title: string }
export interface AniListList { entries: AniListEntry[]; truncated: boolean }

export type ImportErrorCode = "user_not_found" | "list_private" | "busy" | "network";
export class ImportError extends Error {
  constructor(public code: ImportErrorCode) { super(code); }
}

export async function fetchAniListList(user: string, signal?: AbortSignal): Promise<AniListList> {
  let res: Response;
  try {
    res = await fetch(`${process.env.NEXT_PUBLIC_API_BASE || "/api/v1"}/import/anilist?user=${encodeURIComponent(user.trim())}`, { cache: "no-store", signal });
  } catch {
    throw new ImportError("network");
  }
  if (res.ok) return res.json();
  const body = await res.json().catch(() => ({}));
  if (body?.error === "user_not_found" || body?.error === "list_private") throw new ImportError(body.error);
  throw new ImportError(res.status === 429 || res.status === 503 ? "busy" : "network");
}

/** Consecutive failed lookups after which the rest is left for later: AniList is most likely throttling. */
export const MAX_CONSECUTIVE_FAILURES = 3;

export interface ResolvedImport { franchiseIds: number[]; resolved: number; skipped: number; stopped: boolean }

/**
 * Maps AniList titles (one id per season) to franchise ids, the unit "ma liste" stores, one lookup
 * at a time so the backend is never flooded. Titles of the same franchise collapse into one id.
 */
export async function resolveFranchises(
  entries: AniListEntry[],
  lookup: (id: number) => Promise<number>,
  onProgress?: (done: number) => void,
  signal?: AbortSignal,
): Promise<ResolvedImport> {
  const ids = new Set<number>();
  let resolved = 0, skipped = 0, failures = 0, stopped = false;
  for (const entry of entries) {
    if (signal?.aborted) { stopped = true; break; }
    try {
      ids.add(await lookup(entry.id));
      resolved++;
      failures = 0;
    } catch {
      skipped++;
      if (++failures >= MAX_CONSECUTIVE_FAILURES) { stopped = true; break; }
    }
    onProgress?.(resolved + skipped);
  }
  return { franchiseIds: [...ids], resolved, skipped, stopped };
}

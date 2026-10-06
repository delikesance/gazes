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

/** MyAnimeList statuses worth importing into "ma liste": watching, plan to watch, on hold (text or numeric export). */
const MAL_WANTED = new Set(["watching", "plan to watch", "on-hold", "on hold", "1", "3", "6"]);

/** MyAnimeList ids of the titles to import from a MAL XML export (animelist). Plain scan: no DOM needed. */
export function parseMalExport(xml: string): number[] {
  const ids = new Set<number>();
  for (const block of xml.match(/<anime>[\s\S]*?<\/anime>/g) ?? []) {
    const id = Number(/<series_animedb_id>\s*(?:<!\[CDATA\[)?\s*(\d+)/.exec(block)?.[1]);
    const status = /<my_status>\s*(?:<!\[CDATA\[)?\s*([^<\]]*)/.exec(block)?.[1]?.trim().toLowerCase();
    if (Number.isInteger(id) && id > 0 && status && MAL_WANTED.has(status)) ids.add(id);
  }
  return [...ids];
}

/** Maps MAL ids to AniList titles through the backend (50 ids per upstream request). */
export async function fetchMalTitles(malIds: number[], signal?: AbortSignal): Promise<AniListList> {
  let res: Response;
  try {
    res = await fetch(`${process.env.NEXT_PUBLIC_API_BASE || "/api/v1"}/import/mal`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ids: malIds }), cache: "no-store", signal });
  } catch {
    throw new ImportError("network");
  }
  if (res.ok) return res.json();
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

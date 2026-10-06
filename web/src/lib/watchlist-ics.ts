import { getFranchise, getSchedule } from "./api";

const stamp = (unix: number) => new Date(unix * 1000).toISOString().replace(/[-:]/g, "").replace(/\.\d{3}/, "");
/** TEXT value escaping (RFC 5545 §3.3.11): backslash first, then ; , and newlines. */
export const icsText = (text: string) => text.replace(/[\\;,]/g, (c) => `\\${c}`).replace(/\r?\n/g, "\\n");

/** Folds a content line at 75 octets (RFC 5545 §3.1), never inside a UTF-8 character. */
export function foldLine(line: string): string {
  const encoder = new TextEncoder();
  const parts: string[] = [];
  let current = "";
  let size = 0;
  for (const char of line) {
    const bytes = encoder.encode(char).length;
    // Continuation lines start with a space, which counts toward their 75 octets.
    if (size + bytes > 75) { parts.push(current); current = " "; size = 1; }
    current += char;
    size += bytes;
  }
  parts.push(current);
  return parts.join("\r\n");
}

// A list holds up to 2000 anime: fetch their franchises a few at a time, not all at once.
const FRANCHISE_CONCURRENCY = 6;

/** Maps items through fn with at most `limit` calls in flight; results keep the input order. */
export async function mapLimited<T, R>(items: T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length);
  let next = 0;
  const worker = async () => { while (next < items.length) { const i = next++; out[i] = await fn(items[i]); } };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
  return out;
}

/** Airings of the next six weeks for the saved anime, as an iCalendar file (null when none air). */
export async function watchlistCalendar(ids: number[], origin: string): Promise<string | null> {
  const franchises = await mapLimited(ids, FRANCHISE_CONCURRENCY, (id) => getFranchise(id).catch(() => null));
  const owner = new Map<number, { id: number; title: string }>();
  for (const f of franchises) for (const s of f?.seasons ?? []) owner.set(s.id, { id: f!.id, title: f!.title });
  const from = Math.floor(Date.now() / 1000);
  const schedule = await getSchedule(from, from + 42 * 24 * 3600 - 60);
  const entries = schedule.entries.filter((e) => owner.has(e.media_id));
  if (!entries.length) return null;
  const lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Gazes//Ma liste//FR", "CALSCALE:GREGORIAN"];
  for (const e of entries) {
    const anime = owner.get(e.media_id)!;
    lines.push("BEGIN:VEVENT", `UID:${e.media_id}-${e.episode}@gazes`, `DTSTAMP:${stamp(from)}`, `DTSTART:${stamp(e.airing_at)}`, `DTEND:${stamp(e.airing_at + 1440)}`,
      `SUMMARY:${icsText(`${anime.title} · épisode ${e.episode}`)}`, `URL:${origin}/anime/${anime.id}`, "END:VEVENT");
  }
  lines.push("END:VCALENDAR");
  return lines.map(foldLine).join("\r\n");
}

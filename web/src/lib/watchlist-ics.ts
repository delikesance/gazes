import { getFranchise, getSchedule } from "./api";

const stamp = (unix: number) => new Date(unix * 1000).toISOString().replace(/[-:]/g, "").replace(/\.\d{3}/, "");
const escape = (text: string) => text.replace(/[\;,]/g, (c) => `\\${c}`).replace(/\r?\n/g, "\\n");

/** Airings of the next six weeks for the saved anime, as an iCalendar file (null when none air). */
export async function watchlistCalendar(ids: number[], origin: string): Promise<string | null> {
  const franchises = await Promise.all(ids.map((id) => getFranchise(id).catch(() => null)));
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
      `SUMMARY:${escape(`${anime.title} · épisode ${e.episode}`)}`, `URL:${origin}/anime/${anime.id}`, "END:VEVENT");
  }
  lines.push("END:VCALENDAR");
  return lines.join("\r\n");
}

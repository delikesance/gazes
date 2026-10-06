"use client";
import { useSyncExternalStore } from "react";
import { pullNotes, pushNotes, type RemoteNote } from "./auth";

/**
 * Private rating (1-10) and note per anime. Local, and mirrored to the account while signed in
 * (newest write wins per anime). Clearing keeps an empty entry so other devices learn about it.
 */
export interface AnimeNote { rating: number; note: string; updatedAt: number }
export const MAX_NOTE_LENGTH = 500;

const KEY = "gazes-notes";
const changed = "gazes-notes-change";
let syncEnabled = false;

export function parseNotes(raw: string | null): Record<number, AnimeNote> {
  const out: Record<number, AnimeNote> = {};
  try {
    const value = JSON.parse(raw || "{}");
    for (const [id, n] of Object.entries(value ?? {})) {
      const note = n as Partial<AnimeNote>;
      const anime = Number(id);
      if (Number.isInteger(anime) && anime > 0 && Number.isInteger(note.rating) && typeof note.note === "string" && Number.isFinite(note.updatedAt))
        out[anime] = { rating: Math.min(10, Math.max(0, note.rating as number)), note: note.note.slice(0, MAX_NOTE_LENGTH), updatedAt: note.updatedAt as number };
    }
  } catch {}
  return out;
}

/** Newest write wins per anime; `push` holds what the server lacks or has older. */
export function mergeNotes(local: Record<number, AnimeNote>, remote: RemoteNote[]) {
  const merged: Record<number, AnimeNote> = { ...local };
  const known = new Map(remote.map((n) => [n.anime_id, n]));
  for (const n of remote) {
    const mine = local[n.anime_id];
    if (!mine || n.updated_at > mine.updatedAt) merged[n.anime_id] = { rating: n.rating, note: n.note, updatedAt: n.updated_at };
  }
  const push: RemoteNote[] = [];
  for (const [id, mine] of Object.entries(local)) {
    const other = known.get(Number(id));
    if (!other || mine.updatedAt > other.updated_at) push.push({ anime_id: Number(id), rating: mine.rating, note: mine.note, updated_at: mine.updatedAt });
  }
  return { merged, push };
}

function read(): Record<number, AnimeNote> { try { return parseNotes(localStorage.getItem(KEY)); } catch { return {}; } }
function write(notes: Record<number, AnimeNote>) {
  try { localStorage.setItem(KEY, JSON.stringify(notes)); } catch {}
  window.dispatchEvent(new Event(changed));
}

/** Saves the note of one anime; a rating of 0 with an empty note clears it. */
export function saveNote(animeId: number, rating: number, note: string) {
  const entry = { rating: Math.min(10, Math.max(0, Math.round(rating))), note: note.slice(0, MAX_NOTE_LENGTH), updatedAt: Math.floor(Date.now() / 1000) };
  write({ ...read(), [animeId]: entry });
  if (syncEnabled) void pushNotes([{ anime_id: animeId, rating: entry.rating, note: entry.note, updated_at: entry.updatedAt }]).catch(() => {});
}

/** Forgets every local note (account deletion). */
export function clearLocalNotes() {
  try { localStorage.removeItem(KEY); window.dispatchEvent(new Event(changed)); } catch {}
}

export function setNotesSyncEnabled(enabled: boolean) { syncEnabled = enabled; }

/** After sign-in: take in what other devices wrote, upload what this one has newer. */
export async function syncNotesOnLogin() {
  syncEnabled = true;
  try {
    const remote = await pullNotes();
    const { merged, push } = mergeNotes(read(), remote);
    write(merged);
    for (let i = 0; i < push.length; i += 200) await pushNotes(push.slice(i, i + 200));
  } catch {}
}

function subscribe(callback: () => void) {
  window.addEventListener(changed, callback);
  window.addEventListener("storage", callback);
  return () => { window.removeEventListener(changed, callback); window.removeEventListener("storage", callback); };
}

/** The viewer's note for an anime, or null. Raw JSON is the snapshot so the value is stable between changes. */
export function useAnimeNote(animeId: number): AnimeNote | null {
  const raw = useSyncExternalStore(subscribe, () => { try { return localStorage.getItem(KEY); } catch { return null; } }, () => null);
  const note = parseNotes(raw)[animeId];
  return note && (note.rating > 0 || note.note) ? note : null;
}

"use client";
import { useSyncExternalStore } from "react";

/**
 * Manual "vu / non vu" per episode, kept on this device. It overrides what the resume point
 * implies (every episode before it counts as watched): `seen` forces an episode to watched,
 * `unseen` forces it back to not watched.
 */
const PREFIX = "gazes-watched:";
const changed = "gazes-watched-change";

export type WatchedMarks = { seen: number[]; unseen: number[] };
const EMPTY: WatchedMarks = { seen: [], unseen: [] };

const numbers = (value: unknown): number[] => (Array.isArray(value) ? value.filter((n) => Number.isInteger(n) && n > 0) : []);

export function parseMarks(raw: string | null): WatchedMarks {
  try {
    const value = JSON.parse(raw || "null");
    return value ? { seen: numbers(value.seen), unseen: numbers(value.unseen) } : EMPTY;
  } catch { return EMPTY; }
}

/** Watched when forced, or when the resume point is past it and nobody cleared it. */
export function isWatched(marks: WatchedMarks, episode: number, resumeEpisode?: number): boolean {
  if (marks.unseen.includes(episode)) return false;
  return marks.seen.includes(episode) || (resumeEpisode !== undefined && episode < resumeEpisode);
}

/** Flips an episode, storing the override only when it differs from the implied state. */
export function toggleWatched(season: number, episode: number, resumeEpisode?: number) {
  try {
    const marks = parseMarks(localStorage.getItem(PREFIX + season));
    const implied = resumeEpisode !== undefined && episode < resumeEpisode;
    const next = !isWatched(marks, episode, resumeEpisode);
    const without = (list: number[]) => list.filter((n) => n !== episode);
    const seen = without(marks.seen), unseen = without(marks.unseen);
    if (next && !implied) seen.push(episode);
    if (!next && implied) unseen.push(episode);
    if (seen.length || unseen.length) localStorage.setItem(PREFIX + season, JSON.stringify({ seen, unseen }));
    else localStorage.removeItem(PREFIX + season);
    window.dispatchEvent(new Event(changed));
  } catch {}
}

/** Forgets every manual mark (account log erasure, account deletion). */
export function clearWatchedMarks() {
  try {
    const keys: string[] = [];
    for (let i = 0; i < localStorage.length; i++) { const key = localStorage.key(i); if (key?.startsWith(PREFIX)) keys.push(key); }
    for (const key of keys) localStorage.removeItem(key);
    window.dispatchEvent(new Event(changed));
  } catch {}
}

function subscribe(callback: () => void) {
  window.addEventListener(changed, callback);
  window.addEventListener("storage", callback);
  return () => { window.removeEventListener(changed, callback); window.removeEventListener("storage", callback); };
}

export function useWatchedMarks(season: number): WatchedMarks {
  const raw = useSyncExternalStore(subscribe, () => { try { return localStorage.getItem(PREFIX + season); } catch { return null; } }, () => null);
  return parseMarks(raw);
}

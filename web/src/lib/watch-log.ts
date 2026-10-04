"use client";
import { pullWatchSessions, pushWatchSessions } from "./auth";

/**
 * Permanent record of every watching session, kept for recommendations.
 * Unlike the resume point (watch-progress), finishing an episode never removes anything here.
 * Stored on this device, and mirrored to the account while signed in.
 */
export type WatchSession = {
  id: string;
  season_id: number;
  anime_id: number;
  episode: number;
  title: string;
  genres: string[];
  format: string;
  started_at: number;
  updated_at: number;
  start_position: number;
  end_position: number;
  /** Seconds actually played (seeking and pauses excluded). */
  watched_seconds: number;
  duration: number;
  completed: boolean;
  audio_lang: string;
  sub_lang: string;
  /** Minutes east of UTC when watching, to learn at which hour of the day the viewer watches. */
  tz_offset: number;
};

export type SessionMeta = { animeId?: number; title?: string; genres?: string[]; format?: string; audioLang?: string; subLang?: string };

const KEY = "gazes-watchlog";
const MAX_LOCAL = 3000;
const GAP_SECONDS = 30 * 60;
const MAX_TICK = 20; // a larger forward jump is a seek, not playback
const changed = "gazes-watchlog-change";

let sessions: WatchSession[] | null = null;
let current: { session: WatchSession; lastPosition: number } | null = null;
let persistTimer: ReturnType<typeof setTimeout> | undefined;
let syncEnabled = false;
const dirty = new Set<string>();
let flushTimer: ReturnType<typeof setTimeout> | undefined;

function load(): WatchSession[] {
  if (sessions) return sessions;
  try { sessions = JSON.parse(localStorage.getItem(KEY) || "[]"); } catch { sessions = []; }
  if (!Array.isArray(sessions)) sessions = [];
  return sessions;
}
function persist() {
  persistTimer = undefined;
  const all = load();
  if (all.length > MAX_LOCAL) all.splice(0, all.length - MAX_LOCAL);
  try { localStorage.setItem(KEY, JSON.stringify(all)); } catch {}
  window.dispatchEvent(new Event(changed));
}
function schedulePersist() { if (!persistTimer) persistTimer = setTimeout(persist, 5000); }
function newId() { return typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`; }

/** Called on every playback tick; cheap, writes to storage at most every few seconds. */
export function recordPlayback(season: number, episode: number, position: number, duration: number, meta: SessionMeta = {}) {
  if (typeof window === "undefined" || !Number.isFinite(position) || position < 0) return;
  const now = Math.floor(Date.now() / 1000);
  const all = load();
  let live = current;
  if (!live || live.session.season_id !== season || live.session.episode !== episode || now - live.session.updated_at > GAP_SECONDS) {
    const session: WatchSession = {
      id: newId(), season_id: season, anime_id: meta.animeId ?? 0, episode, title: (meta.title ?? "").slice(0, 200),
      genres: (meta.genres ?? []).slice(0, 20).map((g) => g.slice(0, 40)), format: (meta.format ?? "").slice(0, 24),
      started_at: now, updated_at: now, start_position: Math.floor(position), end_position: Math.floor(position),
      watched_seconds: 0, duration: Math.floor(duration) || 0, completed: false,
      audio_lang: (meta.audioLang ?? "").slice(0, 16), sub_lang: (meta.subLang ?? "").slice(0, 16), tz_offset: -new Date().getTimezoneOffset(),
    };
    all.push(session);
    live = current = { session, lastPosition: position };
  }
  const session = live.session;
  const delta = position - live.lastPosition;
  if (delta > 0 && delta <= MAX_TICK) session.watched_seconds += delta;
  live.lastPosition = position;
  session.end_position = Math.floor(position);
  session.updated_at = now;
  if (duration > 0) { session.duration = Math.floor(duration); if (position >= duration - 15) session.completed = true; }
  if (meta.animeId) session.anime_id = meta.animeId;
  if (meta.title) session.title = meta.title.slice(0, 200);
  if (meta.genres?.length && !session.genres.length) session.genres = meta.genres.slice(0, 20).map((g) => g.slice(0, 40));
  if (meta.format && !session.format) session.format = meta.format.slice(0, 24);
  if (meta.audioLang) session.audio_lang = meta.audioLang.slice(0, 16);
  if (meta.subLang) session.sub_lang = meta.subLang.slice(0, 16);
  schedulePersist();
  if (syncEnabled) { dirty.add(session.id); if (!flushTimer) flushTimer = setTimeout(flush, 30000); }
}

export function listWatchSessions(): WatchSession[] { return load().slice(); }

/** Anime the viewer actually watched (at least two minutes), most recent first: seeds for suggestions. */
export function watchedAnimeIds(limit = 5): number[] {
  const last = new Map<number, number>();
  for (const s of load()) {
    if (s.watched_seconds < 120) continue;
    const id = s.anime_id || s.season_id;
    last.set(id, Math.max(last.get(id) ?? 0, s.updated_at));
  }
  return [...last.entries()].sort((a, b) => b[1] - a[1]).slice(0, limit).map(([id]) => id);
}

async function push(batch: WatchSession[]) {
  for (let i = 0; i < batch.length; i += 200) {
    await pushWatchSessions(batch.slice(i, i + 200));
  }
}
async function flush() {
  flushTimer = undefined;
  if (!syncEnabled || !dirty.size) return;
  const ids = new Set(dirty);
  dirty.clear();
  const batch = load().filter((s) => ids.has(s.id));
  try { await push(batch); } catch { for (const id of ids) dirty.add(id); }
}
if (typeof window !== "undefined") window.addEventListener("pagehide", () => { if (persistTimer) persist(); void flush(); });

export function setWatchLogSyncEnabled(enabled: boolean) {
  syncEnabled = enabled;
  if (!enabled) { dirty.clear(); if (flushTimer) clearTimeout(flushTimer); flushTimer = undefined; }
}

/** After sign-in: upload every local session and take in the ones recorded on other devices. */
export async function syncWatchLogOnLogin() {
  setWatchLogSyncEnabled(true);
  try {
    const all = load();
    const remote = await pullWatchSessions<WatchSession>();
    const remoteById = new Map(remote.map((r) => [r.id, r]));
    const byId = new Map(all.map((s) => [s.id, s]));
    const toPush: WatchSession[] = [];
    for (const s of all) { const other = remoteById.get(s.id); if (!other || s.updated_at > other.updated_at) toPush.push(s); }
    for (const r of remote) { const mine = byId.get(r.id); if (!mine) all.push(r); else if (r.updated_at > mine.updated_at) Object.assign(mine, r); }
    all.sort((a, b) => a.started_at - b.started_at);
    persist();
    if (toPush.length) await push(toPush);
  } catch {}
}

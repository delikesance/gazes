"use client";
import { useSyncExternalStore } from "react";
import { pullWatchlist, updateWatchlist } from "./auth";
import { chunks, planWatchlistSync } from "./watchlist-sync";

/** "Ma liste": anime the viewer saved for later. Local, and mirrored to the account while signed in. */
const KEY = "gazes-watchlist";
const SYNCED_KEY = "gazes-watchlist-synced"; // { user, ids }: that account's server list as of the last successful sync
const changed = "gazes-watchlist-change";
const MAX_IDS = 2000; // the server's cap
const PUT_BATCH = 500; // the server's limit per request
let syncEnabled = false;
let syncedUser = 0; // account the synced snapshot belongs to

function parse(raw: string | null): number[] {
  try {
    const value = JSON.parse(raw || "[]");
    return Array.isArray(value) ? value.filter((id) => Number.isInteger(id) && id > 0) : [];
  } catch { return []; }
}
function read(): number[] { try { return parse(localStorage.getItem(KEY)); } catch { return []; } }
function write(ids: number[]) {
  try { localStorage.setItem(KEY, JSON.stringify(ids.slice(0, MAX_IDS))); } catch {}
  window.dispatchEvent(new Event(changed));
}
/** Another account's snapshot (or the legacy unowned one) says nothing about this account: it must never drive removals. */
function readSynced(user: number): number[] {
  try {
    const value = JSON.parse(localStorage.getItem(SYNCED_KEY) || "null");
    return value && value.user === user ? parse(JSON.stringify(value.ids)) : [];
  } catch { return []; }
}
function writeSynced(ids: number[]) {
  if (!syncedUser) return;
  try { localStorage.setItem(SYNCED_KEY, JSON.stringify({ user: syncedUser, ids })); } catch {}
}
function push(add: number[], remove: number[]): Promise<number[]> {
  return updateWatchlist(add, remove).then((ids) => { writeSynced(ids); return ids; });
}

export function listWatchlist(): number[] { return typeof window === "undefined" ? [] : read(); }

export function addToWatchlist(id: number) {
  const ids = read();
  if (!ids.includes(id)) write([id, ...ids]);
  if (syncEnabled) void push([id], []).catch(() => {});
}

export function removeFromWatchlist(id: number) {
  write(read().filter((other) => other !== id));
  if (syncEnabled) void push([], [id]).catch(() => {});
}

export function setWatchlistSyncEnabled(enabled: boolean) {
  syncEnabled = enabled;
  if (!enabled) syncedUser = 0;
}

/**
 * After sign-in: upload what this device saved, take in what other devices saved, drop what they
 * removed, and send what this device removed while signed out (or whose removal failed to send).
 */
export async function syncWatchlistOnLogin(user: number) {
  syncEnabled = true;
  try {
    const remote = await pullWatchlist();
    const synced = readSynced(user);
    syncedUser = user;
    const { upload, keep, remove } = planWatchlistSync(read(), remote, synced);
    let merged = remote;
    // A failed batch is retried next time: a removal the server missed stays in the snapshot (and
    // out of the local list), an upload (e.g. refused, the list is full) stays local and out of it.
    for (const batch of chunks(remove, PUT_BATCH)) merged = await updateWatchlist([], batch).catch(() => merged);
    const removed = new Set(remove);
    writeSynced(merged);
    for (const batch of chunks(upload, PUT_BATCH)) merged = await push(batch, []).catch(() => merged);
    write([...new Set([...merged, ...keep])].filter((id) => !removed.has(id)));
  } catch {}
}

function subscribe(callback: () => void) {
  window.addEventListener(changed, callback);
  window.addEventListener("storage", callback);
  return () => { window.removeEventListener(changed, callback); window.removeEventListener("storage", callback); };
}

/** Raw JSON is the snapshot, so the hook returns a stable value between changes. */
function useRaw(): string | null {
  return useSyncExternalStore(subscribe, () => { try { return localStorage.getItem(KEY); } catch { return null; } }, () => null);
}

export function useWatchlist(): number[] { return parse(useRaw()); }
export function useInWatchlist(id: number): boolean { return parse(useRaw()).includes(id); }

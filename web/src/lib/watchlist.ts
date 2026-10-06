"use client";
import { useSyncExternalStore } from "react";
import { pullWatchlist, updateWatchlist } from "./auth";
import { chunks, planWatchlistSync } from "./watchlist-sync";

/** "Ma liste": anime the viewer saved for later. Local, and mirrored to the account while signed in. */
const KEY = "gazes-watchlist";
const SYNCED_KEY = "gazes-watchlist-synced"; // server list as of the last successful sync
const changed = "gazes-watchlist-change";
const MAX_IDS = 2000; // the server's cap
const PUT_BATCH = 500; // the server's limit per request
let syncEnabled = false;

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
function readSynced(): number[] { try { return parse(localStorage.getItem(SYNCED_KEY)); } catch { return []; } }
function writeSynced(ids: number[]) { try { localStorage.setItem(SYNCED_KEY, JSON.stringify(ids)); } catch {} }
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

export function setWatchlistSyncEnabled(enabled: boolean) { syncEnabled = enabled; }

/** After sign-in: upload what this device saved, take in what other devices saved, drop what they removed. */
export async function syncWatchlistOnLogin() {
  syncEnabled = true;
  try {
    const remote = await pullWatchlist();
    const { upload, keep } = planWatchlistSync(read(), remote, readSynced());
    let merged = remote;
    writeSynced(merged);
    // A failed batch (e.g. the list is full) stays local and unsynced, so it is retried next time.
    for (const batch of chunks(upload, PUT_BATCH)) merged = await push(batch, []).catch(() => merged);
    write([...new Set([...merged, ...keep])]);
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

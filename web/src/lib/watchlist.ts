"use client";
import { useSyncExternalStore } from "react";
import { pullWatchlist, updateWatchlist } from "./auth";

/** "Ma liste": anime the viewer saved for later. Local, and mirrored to the account while signed in. */
const KEY = "gazes-watchlist";
const changed = "gazes-watchlist-change";
let syncEnabled = false;

function parse(raw: string | null): number[] {
  try {
    const value = JSON.parse(raw || "[]");
    return Array.isArray(value) ? value.filter((id) => Number.isInteger(id) && id > 0) : [];
  } catch { return []; }
}
function read(): number[] { try { return parse(localStorage.getItem(KEY)); } catch { return []; } }
function write(ids: number[]) {
  try { localStorage.setItem(KEY, JSON.stringify(ids.slice(0, 2000))); } catch {}
  window.dispatchEvent(new Event(changed));
}

export function listWatchlist(): number[] { return typeof window === "undefined" ? [] : read(); }

export function addToWatchlist(id: number) {
  const ids = read();
  if (!ids.includes(id)) write([id, ...ids]);
  if (syncEnabled) void updateWatchlist([id], []).catch(() => {});
}

export function removeFromWatchlist(id: number) {
  write(read().filter((other) => other !== id));
  if (syncEnabled) void updateWatchlist([], [id]).catch(() => {});
}

export function setWatchlistSyncEnabled(enabled: boolean) { syncEnabled = enabled; }

/** After sign-in: upload what this device saved and take in what other devices saved. */
export async function syncWatchlistOnLogin() {
  syncEnabled = true;
  try {
    const local = read();
    const remote = await pullWatchlist();
    const missing = local.filter((id) => !remote.includes(id));
    const merged = missing.length ? await updateWatchlist(missing, []) : remote;
    write([...new Set([...merged, ...local])]);
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

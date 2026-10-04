"use client";
import { pullHidden, updateHidden } from "./auth";

/** Anime the viewer marked "pas intéressé": never suggested again. Local, and mirrored to the account while signed in. */
const KEY = "gazes-hidden";
let syncEnabled = false;

function read(): number[] {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || "[]");
    return Array.isArray(value) ? value.filter((id) => Number.isInteger(id) && id > 0) : [];
  } catch { return []; }
}
function write(ids: number[]) { try { localStorage.setItem(KEY, JSON.stringify(ids.slice(0, 2000))); } catch {} }

export function listHidden(): number[] { return typeof window === "undefined" ? [] : read(); }

export function hideAnime(id: number) {
  const ids = read();
  if (!ids.includes(id)) write([id, ...ids]);
  if (syncEnabled) void updateHidden([id], []).catch(() => {});
}

export function unhideAnime(id: number) {
  write(read().filter((other) => other !== id));
  if (syncEnabled) void updateHidden([], [id]).catch(() => {});
}

export function setHiddenSyncEnabled(enabled: boolean) { syncEnabled = enabled; }

/** After sign-in: upload what this device hid and take in what other devices hid. */
export async function syncHiddenOnLogin() {
  syncEnabled = true;
  try {
    const local = read();
    const remote = await pullHidden();
    const missing = local.filter((id) => !remote.includes(id));
    const merged = missing.length ? await updateHidden(missing, []) : remote;
    write([...new Set([...local, ...merged])]);
  } catch {}
}

export function clearHidden() { try { localStorage.removeItem(KEY); } catch {} }

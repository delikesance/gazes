"use client";
import { useSyncExternalStore } from "react";
import { pullProgress, pushProgress, type RemoteProgress } from "./auth";
import { setHiddenSyncEnabled, syncHiddenOnLogin } from "./hidden-anime";
import { recordPlayback, setWatchLogSyncEnabled, syncWatchLogOnLogin } from "./watch-log";

export type WatchProgress = {episode:number; position:number};
export type SavedProgress = WatchProgress & {season:number; animeId:number; title:string; updatedAt:number; duration?:number};
export type ProgressMeta = {animeId?:number; title?:string; genres?:string[]; format?:string; audioLang?:string; subLang?:string};

const PREFIX = "gazes-progress:";
const lastSaved = new Map<number,string>();
const changed = "gazes-progress-change";

// Accounts: while signed in, progress is mirrored to the server (newest write wins).
let syncEnabled = false;
const pending = new Map<number,RemoteProgress>();
let flushTimer: ReturnType<typeof setTimeout> | undefined;

export function setProgressSyncEnabled(enabled:boolean) {
 syncEnabled = enabled;
 setWatchLogSyncEnabled(enabled);
 setHiddenSyncEnabled(enabled);
 if (!enabled) { pending.clear(); if (flushTimer) clearTimeout(flushTimer); flushTimer = undefined; }
}
function queuePush(entry:RemoteProgress) {
 if (!syncEnabled) return;
 pending.set(entry.season_id, entry);
 if (!flushTimer) flushTimer = setTimeout(flush, 15000);
}
async function flush() {
 flushTimer = undefined;
 if (!syncEnabled || !pending.size) return;
 const items = [...pending.values()];
 pending.clear();
 try { await pushProgress(items); } catch { for (const item of items) if (!pending.has(item.season_id)) pending.set(item.season_id, item); }
}
if (typeof window !== "undefined") window.addEventListener("pagehide", () => { void flush(); });

function readAll():SavedProgress[] {
 const out:SavedProgress[] = [];
 try {
  for (let i = 0; i < localStorage.length; i++) {
   const key = localStorage.key(i);
   if (!key?.startsWith(PREFIX)) continue;
   const season = Number(key.slice(PREFIX.length));
   const value = JSON.parse(localStorage.getItem(key) || "null");
   if (Number.isInteger(season) && value && Number.isInteger(value.episode) && value.episode > 0 && Number.isFinite(value.position) && value.position >= 5)
    out.push({season, episode:value.episode, position:value.position, animeId:Number(value.animeId) || 0, title:String(value.title || ""), updatedAt:Number(value.updatedAt) || 1, duration:Number.isFinite(value.duration) && value.duration > 0 ? value.duration : undefined});
  }
 } catch {}
 return out.sort((a, b) => b.updatedAt - a.updatedAt);
}
export const listProgress = readAll;

export function saveProgress(season:number, episode:number, position:number, duration:number, meta:ProgressMeta = {}) {
 if (position < 5 || !Number.isFinite(position)) return;
 recordPlayback(season, episode, position, duration, meta);
 try {
  const key = PREFIX + season;
  const complete=duration>0&&position>=duration-15;
  const stamp=`${episode}:${complete?"complete":Math.floor(position/5)}`;
  if(lastSaved.get(season)===stamp) return;
  const updatedAt = Math.floor(Date.now() / 1000);
  const previous = (() => { try { return JSON.parse(localStorage.getItem(key) || "null"); } catch { return null; } })();
  const animeId = meta.animeId ?? previous?.animeId ?? 0;
  const title = (meta.title ?? previous?.title ?? "").slice(0, 200);
  if (complete) localStorage.removeItem(key);
  else localStorage.setItem(key, JSON.stringify({episode, position:Math.floor(position), animeId, title, updatedAt, duration:duration > 0 ? Math.floor(duration) : undefined}));
  queuePush({season_id:season, anime_id:animeId, title, episode, position:Math.floor(position), completed:complete, updated_at:updatedAt});
  lastSaved.set(season,stamp);
  window.dispatchEvent(new Event(changed));
 } catch {}
}

/** Merge local and server progress once after sign-in: the newest entry per season wins on both sides. */
export async function syncProgressOnLogin() {
 setProgressSyncEnabled(true);
 void syncWatchLogOnLogin();
 void syncHiddenOnLogin();
 try {
  const remote = await pullProgress();
  const bySeason = new Map(remote.map((item) => [item.season_id, item]));
  const local = new Map(readAll().map((item) => [item.season, item]));
  const toPush:RemoteProgress[] = [];
  for (const item of local.values()) {
   const other = bySeason.get(item.season);
   if (!other || item.updatedAt > other.updated_at)
    toPush.push({season_id:item.season, anime_id:item.animeId, title:item.title, episode:item.episode, position:Math.floor(item.position), completed:false, updated_at:item.updatedAt});
  }
  for (const item of remote) {
   const mine = local.get(item.season_id);
   if (item.updated_at <= (mine?.updatedAt ?? 0)) continue;
   if (item.completed) localStorage.removeItem(PREFIX + item.season_id);
   else localStorage.setItem(PREFIX + item.season_id, JSON.stringify({episode:item.episode, position:Math.floor(item.position), animeId:item.anime_id, title:item.title, updatedAt:item.updated_at}));
  }
  if (toPush.length) await pushProgress(toPush);
  window.dispatchEvent(new Event(changed));
 } catch {}
}

function subscribe(callback:()=>void) {window.addEventListener(changed,callback);window.addEventListener("storage",callback);return()=>{window.removeEventListener(changed,callback);window.removeEventListener("storage",callback);};}
export function useWatchProgress(season:number):WatchProgress|null {
 const raw=useSyncExternalStore(subscribe,()=>{try{return localStorage.getItem(PREFIX+season);}catch{return null;}},()=>null);
 try {const value=raw?JSON.parse(raw):null;return value&&Number.isInteger(value.episode)&&value.episode>0&&Number.isFinite(value.position)&&value.position>=5?value:null;}catch{return null;}
}

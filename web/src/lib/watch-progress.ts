"use client";
import { useSyncExternalStore } from "react";
export type WatchProgress = {episode:number; position:number};
const lastSaved = new Map<number,string>();
const changed = "gazes-progress-change";
export function saveProgress(season:number, episode:number, position:number, duration:number) {
 if (position < 5 || !Number.isFinite(position)) return;
 try {
  const key = `gazes-progress:${season}`;
  const complete=duration>0&&position>=duration-15;
  const stamp=`${episode}:${complete?"complete":Math.floor(position/5)}`;
  if(lastSaved.get(season)===stamp) return;
  if (complete) localStorage.removeItem(key);
  else localStorage.setItem(key, JSON.stringify({episode, position:Math.floor(position)}));
  lastSaved.set(season,stamp);
  window.dispatchEvent(new Event(changed));
 } catch {}
}
function subscribe(callback:()=>void) {window.addEventListener(changed,callback);window.addEventListener("storage",callback);return()=>{window.removeEventListener(changed,callback);window.removeEventListener("storage",callback);};}
export function useWatchProgress(season:number):WatchProgress|null {
 const raw=useSyncExternalStore(subscribe,()=>{try{return localStorage.getItem(`gazes-progress:${season}`);}catch{return null;}},()=>null);
 try {const value=raw?JSON.parse(raw):null;return value&&Number.isInteger(value.episode)&&value.episode>0&&Number.isFinite(value.position)&&value.position>=5?value:null;}catch{return null;}
}

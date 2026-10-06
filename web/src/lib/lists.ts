"use client";
import { useSyncExternalStore } from "react";
import * as api from "./auth";
import type { UserList } from "./auth";

/**
 * Named lists live on the account (they need a sign-in): the server answers every call with the
 * whole state, which replaces this cache. Components read it through useLists().
 */
let state: UserList[] | null = null;
const listeners = new Set<() => void>();
const set = (lists: UserList[] | null) => { state = lists; listeners.forEach((fn) => fn()); };

export async function loadLists(): Promise<void> {
  try { set(await api.pullLists()); } catch { /* keep what we have */ }
}
export function resetLists() { set(null); }

export const createNamedList = async (name: string) => set(await api.createList(name));
export const renameNamedList = async (id: number, name: string) => set(await api.renameList(id, name));
export const deleteNamedList = async (id: number) => set(await api.deleteList(id));
export const setInList = async (listId: number, animeId: number, inList: boolean) =>
  set(await api.updateListItems(listId, inList ? [animeId] : [], inList ? [] : [animeId]));

export const MAX_LIST_NAME = 40;

/** Lists of the signed-in viewer; `null` until first loaded. */
export function useLists(): UserList[] | null {
  return useSyncExternalStore((cb) => { listeners.add(cb); return () => { listeners.delete(cb); }; }, () => state, () => null);
}

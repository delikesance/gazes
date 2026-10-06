"use client";
import { useSyncExternalStore } from "react";

function subscribe(notify: () => void) {
  window.addEventListener("popstate", notify);
  window.addEventListener("gazes-url", notify);
  return () => { window.removeEventListener("popstate", notify); window.removeEventListener("gazes-url", notify); };
}

/** One query parameter, read without a Suspense boundary (empty during server rendering). */
export function useUrlParam(name: string): string | null {
  const search = useSyncExternalStore(subscribe, () => window.location.search, () => "");
  return new URLSearchParams(search).get(name);
}

/** Changes query parameters in place (the entry is replaced, not pushed); a null value removes the parameter. */
export function setUrlParams(patch: Record<string, string | null>) {
  const next = new URLSearchParams(window.location.search);
  for (const [key, value] of Object.entries(patch)) { if (value) next.set(key, value); else next.delete(key); }
  const query = next.toString();
  window.history.replaceState(window.history.state, "", `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`);
  window.dispatchEvent(new Event("gazes-url"));
}

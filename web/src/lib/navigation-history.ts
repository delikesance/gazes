"use client";

const EPISODE_ROUTE = /\/episodes\/\d+$/;
let previous: string | null = null;
let prior: string | null = null;
let cameFromApp = false;

/** Called on every client-side route change: remembers whether the current watch session was entered from another page of the app. */
export function trackRoute(path: string) {
 const onEpisode = EPISODE_ROUTE.test(path);
 if (previous !== null && onEpisode && !EPISODE_ROUTE.test(previous)) cameFromApp = true;
 else if (!onEpisode) cameFromApp = false;
 if (path !== previous) prior = previous;
 previous = path;
}

/** True when closing the player can go back to the page the viewer actually came from (history, catalogue, series…). */
export function canGoBackInApp() {
 return cameFromApp;
}

/** True once the viewer has moved between pages of the app, so a back step stays inside it. */
export function canGoBack() {
 return previous !== null;
}

/** The page the viewer navigated from, whether or not the current route was already recorded. */
export function cameFrom(): string | null {
  return typeof window !== "undefined" && previous === window.location.pathname ? prior : previous;
}

const RESULTS_KEY = "gazes-last-results";
/** Remembers the results page (search or genre) being shown, so a series page can offer a way back to it; null clears it. */
export function rememberResults(url: string | null, label = "") {
  try { if (url) sessionStorage.setItem(RESULTS_KEY, JSON.stringify({ url, label })); else sessionStorage.removeItem(RESULTS_KEY); } catch {}
}
/** The results page the viewer just came from, when they did come straight from one. */
export function resultsBehind(): { url: string; label: string } | null {
  const from = cameFrom();
  if (from !== "/" && !from?.startsWith("/genre/")) return null;
  try {
    const value = JSON.parse(sessionStorage.getItem(RESULTS_KEY) || "null");
    return value && typeof value.url === "string" ? { url: value.url, label: String(value.label || "") } : null;
  } catch { return null; }
}

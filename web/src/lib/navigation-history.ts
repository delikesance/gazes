"use client";

const EPISODE_ROUTE = /\/episodes\/\d+$/;
let previous: string | null = null;
let cameFromApp = false;

/** Called on every client-side route change: remembers whether the current watch session was entered from another page of the app. */
export function trackRoute(path: string) {
 const onEpisode = EPISODE_ROUTE.test(path);
 if (previous !== null && onEpisode && !EPISODE_ROUTE.test(previous)) cameFromApp = true;
 else if (!onEpisode) cameFromApp = false;
 previous = path;
}

/** True when closing the player can go back to the page the viewer actually came from (history, catalogue, series…). */
export function canGoBackInApp() {
 return cameFromApp;
}

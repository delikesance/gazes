"use client";

const RECENT_KEY = "gazes-recent-searches";

export function readRecentSearches(): string[] {
  try {
    const value = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").slice(0, 6) : [];
  } catch { return []; }
}

/** Keeps the last six searches; a search that extends or shortens an earlier one replaces it. */
export function saveRecentSearch(text: string) {
  const lower = text.toLowerCase();
  const kept = readRecentSearches().filter((item) => { const other = item.toLowerCase(); return !(other.startsWith(lower) || lower.startsWith(other)); });
  try { localStorage.setItem(RECENT_KEY, JSON.stringify([text, ...kept].slice(0, 6))); } catch {}
}

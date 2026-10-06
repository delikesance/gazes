"use client";
import { deleteWatchData, exportAccount } from "./auth";
import { clearLocalWatchLog } from "./watch-log";
import { clearHidden } from "./hidden-anime";
import { clearWatchedMarks } from "./watched";

/** Downloads everything the account holds (profile, resume points, watch log, hidden anime, watchlist) as one JSON file. */
export async function exportMyData() {
  const url = URL.createObjectURL(new Blob([JSON.stringify(await exportAccount(), null, 2)], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "gazes-mes-donnees.json";
  link.click();
  URL.revokeObjectURL(url);
}

/** Erases the watch log and the "pas intéressé" list, on the account and on this device. Resume points stay. */
export async function eraseMyWatchLog() {
  await deleteWatchData();
  clearLocalWatchLog();
  clearHidden();
  clearWatchedMarks();
}

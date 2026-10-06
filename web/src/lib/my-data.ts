"use client";
import { deleteWatchData, pullHidden, pullProgress, pullWatchSessions } from "./auth";
import { clearLocalWatchLog } from "./watch-log";
import { clearHidden } from "./hidden-anime";
import { clearWatchedMarks } from "./watched";

/** Downloads everything the account holds about the viewer's watching, as one JSON file. */
export async function exportMyData(pseudo: string) {
  const [resumePoints, watchSessions, hiddenAnime] = await Promise.all([pullProgress(), pullWatchSessions<unknown>(), pullHidden()]);
  const payload = { exported_at: new Date().toISOString(), account: pseudo, resume_points: resumePoints, watch_sessions: watchSessions, hidden_anime: hiddenAnime };
  const url = URL.createObjectURL(new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" }));
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

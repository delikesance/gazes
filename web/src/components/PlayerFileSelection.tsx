"use client";
import { useState } from "react";
import { useI18n } from "@/lib/i18n";
import type { FileInfo } from "@/types/api";

interface PlayerFileSelectionProps {
  /** Files that match the episode; empty when none does. */
  matchingFiles: FileInfo[];
  allFiles: FileInfo[];
  episodeNumber?: number;
  /** The next source is tried automatically: no manual choice is offered. */
  failingOver: boolean;
  onSelect: (index: number) => void;
}

/** Shown when the pack has no single file for the episode: the viewer picks one, nothing plays meanwhile. */
export function PlayerFileSelection({ matchingFiles, allFiles, episodeNumber, failingOver, onSelect }: PlayerFileSelectionProps) {
  const { t } = useI18n();
  const [fileSearch, setFileSearch] = useState("");
  const selectionFiles = (matchingFiles.length ? matchingFiles : allFiles.filter(file=>file.is_video)).filter(file=>file.path.toLowerCase().includes(fileSearch.toLowerCase()));
  return (
    <div className="p-6 pt-24 space-y-4">
      <p>{failingOver ? t("Cet épisode ne peut pas être identifié dans ce pack. Essai de la source suivante…") : t("Choisissez le fichier correspondant à l’épisode {episode}. Aucun fichier n’a été lancé automatiquement.", {episode:episodeNumber ?? "—"})}</p>
      {!failingOver && <>
      <input aria-label={t("Rechercher un fichier")} placeholder={t("Rechercher un fichier…")} value={fileSearch} onChange={event=>setFileSearch(event.target.value)} className="w-full bg-zinc-900 border border-zinc-700 p-3 rounded-full" />
      <p>{selectionFiles.length} {" "}{t("fichiers")}{matchingFiles.length ? t(" correspondant à cet épisode") : t(" vidéo")}</p>
      {selectionFiles.slice(0,50).map(file => <button key={file.index} className="block p-3 bg-zinc-900 rounded-2xl text-left w-full" onClick={() => onSelect(file.index)}>{file.path}</button>)}
      {selectionFiles.length>50 && <p>{t("Affichage des 50 premiers fichiers. Affinez la recherche.")}</p>}
      </>}
    </div>
  );
}

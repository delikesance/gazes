"use client";
import { useState } from "react";
import { PenLine, Star } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { MAX_NOTE_LENGTH, saveNote, useAnimeNote } from "@/lib/notes";

/** "Ma note": a private 1-10 rating and a short note on an anime, visible only to the viewer. */
export function AnimeNoteButton({ animeId }: { animeId: number }) {
  const { t } = useI18n();
  const current = useAnimeNote(animeId);
  const [open, setOpen] = useState(false);
  const [rating, setRating] = useState(0);
  const [note, setNote] = useState("");

  const toggle = () => {
    if (!open) { setRating(current?.rating ?? 0); setNote(current?.note ?? ""); }
    setOpen(!open);
  };
  const save = (event: React.FormEvent) => { event.preventDefault(); saveNote(animeId, rating, note.trim()); setOpen(false); };
  const clear = () => { saveNote(animeId, 0, ""); setOpen(false); };

  return (
    <div className="anime-note">
      <button type="button" className="clay clay-secondary watchlist-button" aria-expanded={open} onClick={toggle}>
        {current?.rating ? <Star size={16} aria-hidden="true" fill="currentColor" /> : <PenLine size={16} aria-hidden="true" />}
        {current?.rating ? `${current.rating}/10` : current?.note ? t("Ma note") : t("Noter")}
      </button>
      {open && (
        <form className="anime-note-panel" onSubmit={save}>
          <div role="group" aria-label={t("Note sur 10")} className="anime-note-scale">
            {Array.from({ length: 10 }, (_, i) => i + 1).map((value) => (
              <button key={value} type="button" aria-pressed={rating === value} onClick={() => setRating(rating === value ? 0 : value)}>{value}</button>
            ))}
          </div>
          <label htmlFor={`note-${animeId}`}>{t("Note personnelle (privée)")}</label>
          <textarea id={`note-${animeId}`} value={note} onChange={(e) => setNote(e.target.value)} maxLength={MAX_NOTE_LENGTH} rows={3} />
          <div className="account-delete-actions">
            <button type="submit" className="clay clay-primary clay-sm">{t("Enregistrer")}</button>
            {current && <button type="button" className="clay clay-secondary clay-sm" onClick={clear}>{t("Effacer")}</button>}
            <button type="button" className="clay clay-secondary clay-sm" onClick={() => setOpen(false)}>{t("Annuler")}</button>
          </div>
        </form>
      )}
    </div>
  );
}

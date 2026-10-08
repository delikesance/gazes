"use client";
import { useI18n } from "@/lib/i18n";

/** End of episode: the next one starts when the countdown runs out, or now, or never. */
export function PlayerNextEpisodeCard({ seconds, onStartNow, onCancel }: { seconds: number; onStartNow: () => void; onCancel: () => void }) {
  const { t } = useI18n();
  return (
    <div className="next-episode-card" role="status" aria-live="polite">
      <p>{t("Épisode suivant dans {seconds} s", { seconds })}</p>
      <div className="next-episode-actions">
        <button type="button" className="clay clay-primary clay-sm" onClick={onStartNow}>{t("Lancer maintenant")}</button>
        <button type="button" className="clay clay-secondary clay-sm" onClick={onCancel}>{t("Annuler")}</button>
      </div>
    </div>
  );
}

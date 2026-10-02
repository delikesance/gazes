"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { Play } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { listProgress, type SavedProgress } from "@/lib/watch-progress";
import { PageGrid } from "@/components/ui/PageGrid";
import { Scribble } from "@/components/ui/Scribble";

function clock(seconds: number): string {
  const total = Math.floor(seconds), h = Math.floor(total / 3600), m = Math.floor((total % 3600) / 60), s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
}

export default function HistoryPage() {
  const { t, locale } = useI18n();
  const [items, setItems] = useState<SavedProgress[] | null>(null);

  useEffect(() => {
    const refresh = () => setItems(listProgress());
    refresh();
    window.addEventListener("gazes-progress-change", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("gazes-progress-change", refresh); window.removeEventListener("storage", refresh); };
  }, []);

  return (
    <main className="history-page">
      <PageGrid />
      <Scribble shape="b" width={380} rotate={-6} style={{ right: 40, top: 120 }} />
      <div className="history-inner page-inset">
        <span className="eyebrow">{t("Historique")}</span>
        <h1 className="serif">{t("Reprendre la lecture")}</h1>
        {items && items.length === 0 && <p className="history-empty">{t("Rien à reprendre pour l’instant.")}</p>}
        <ul className="history-list">
          {(items || []).map((item) => (
            <li key={item.season}>
              <Link href={item.animeId ? `/anime/${item.animeId}/seasons/${item.season}/episodes/${item.episode}` : "/"} className="history-row">
                <span className="history-play" aria-hidden="true"><Play size={14} /></span>
                <span className="history-title">{item.title || t("Anime")}</span>
                <span className="chip">{t("Épisode")} {item.episode} · {clock(item.position)}</span>
                <span className="history-date">{new Date(item.updatedAt * 1000).toLocaleDateString(locale)}</span>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </main>
  );
}

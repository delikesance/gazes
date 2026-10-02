"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { PageGrid } from "./ui/PageGrid";
import { Scribble } from "./ui/Scribble";

const STEPS = ["Recherche des sources…", "Vérification des fichiers…", "Préparation de la lecture…"];

interface WatchLoadingProps {
  episode: number;
  backHref: string;
}

/** Shown while an episode's sources are being found, so the wait never looks like a black screen. */
export function WatchLoading({ episode, backHref }: WatchLoadingProps) {
  const { t } = useI18n();
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => setSeconds((s) => s + 1), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const step = Math.min(STEPS.length - 1, Math.floor(seconds / 4));

  return (
    <div className="watch-loading" role="status" aria-live="polite">
      <PageGrid />
      <Scribble shape="a" width={360} rotate={-10} style={{ left: -100, top: 90 }} />
      <Scribble shape="b" width={400} rotate={8} style={{ right: -120, bottom: 60 }} />
      <div className="watch-loading-card">
        <svg className="watch-loading-ring" viewBox="0 0 72 72" aria-hidden="true">
          <circle className="watch-loading-track" cx="36" cy="36" r="32" />
          <circle className="watch-loading-arc" cx="36" cy="36" r="32" pathLength={100} />
        </svg>
        <span className="eyebrow">{t("Épisode")} {episode}</span>
        <h1 className="serif">{t("Préparation de l’épisode")}</h1>
        <ol className="watch-loading-steps">
          {STEPS.map((label, index) => (
            <li key={label} data-state={index < step ? "done" : index === step ? "active" : "todo"}>
              <span aria-hidden="true" className="watch-loading-dot" />
              {t(label)}
            </li>
          ))}
        </ol>
        <p className="watch-loading-hint" data-visible={seconds >= 8}>{t("La première recherche peut prendre une vingtaine de secondes. Les suivantes seront instantanées.")}</p>
        <Link href={backHref} className="text-action">{t("Voir les saisons")}</Link>
      </div>
    </div>
  );
}

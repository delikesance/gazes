"use client";
/* eslint-disable @next/next/no-img-element -- artwork URLs come from the catalog CDN */
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";

interface PlayerStartupProps {
  /** "connect" while the torrent metadata loads, "stream" while waiting for the first video bytes. */
  stage: "connect" | "stream";
  overlay?: boolean;
  image?: string;
  title?: string;
  episode?: number;
}

/** Shown instead of a black frame while a stream starts: artwork, what is happening, and a hint once it drags on. */
export function PlayerStartup({ stage, overlay, image, title, episode }: PlayerStartupProps) {
  const { t } = useI18n();
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => setSeconds((s) => s + 1), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const heading = stage === "connect" ? t("Connexion au swarm…") : t("Démarrage du flux…");
  const detail = stage === "connect" ? t("Récupération des métadonnées et des premières pièces") : t("Le premier chargement peut prendre quelques secondes");

  return (
    <div className={`player-connecting${overlay ? " is-overlay" : ""}`} role="status" aria-live="polite">
      {image && <img className="player-startup-art" src={image} alt="" aria-hidden="true" />}
      <div className="player-startup-body">
        <svg className="watch-loading-ring" viewBox="0 0 72 72" aria-hidden="true">
          <circle className="watch-loading-track" cx="36" cy="36" r="32" />
          <circle className="watch-loading-arc" cx="36" cy="36" r="32" pathLength={100} />
        </svg>
        {(title || episode) && <span className="eyebrow">{[title, episode ? `${t("Épisode")} ${episode}` : ""].filter(Boolean).join(" · ")}</span>}
        <h2 className="serif">{heading}</h2>
        <p>{detail}</p>
        <p className="player-startup-hint" data-visible={seconds >= 8}>{t("Peu de pairs répondent pour l’instant, la lecture démarrera dès que possible.")} {seconds}s</p>
      </div>
    </div>
  );
}

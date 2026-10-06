"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { PageGrid } from "@/components/ui/PageGrid";

type State = "loading" | "ok" | "degraded" | "unknown";

/** Public health of playback, from GET /api/v1/status (refreshed every minute). */
export default function StatusPage() {
  const { t } = useI18n();
  const [state, setState] = useState<State>("loading");

  useEffect(() => {
    let active = true;
    const load = () => {
      fetch(`${process.env.NEXT_PUBLIC_API_BASE || "/api/v1"}/status`, { cache: "no-store" })
        .then((res) => (res.ok ? res.json() : Promise.reject(new Error("status"))))
        .then((body: { playback?: string }) => { if (active) setState(body.playback === "ok" || body.playback === "degraded" ? body.playback : "unknown"); })
        .catch(() => { if (active) setState("unknown"); });
    };
    load();
    const timer = window.setInterval(load, 60_000);
    return () => { active = false; window.clearInterval(timer); };
  }, []);

  const title = { loading: "Vérification…", ok: "Tout fonctionne", degraded: "Lecture perturbée", unknown: "État indisponible" }[state];
  const detail = {
    loading: "",
    ok: "La lecture démarre normalement.",
    degraded: "Des lectures échouent en ce moment. Essayez une autre source depuis le lecteur ou réessayez dans quelques minutes.",
    unknown: "Impossible de joindre le service pour le moment.",
  }[state];

  return (
    <main className="history-page">
      <PageGrid />
      <div className="history-inner page-inset account-page">
        <span className="eyebrow">{t("État du service")}</span>
        <h1 className="serif" role="status">{t(title)}</h1>
        {detail && <p className="history-empty">{t(detail)}</p>}
      </div>
    </main>
  );
}

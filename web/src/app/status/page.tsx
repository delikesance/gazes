"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { PageGrid } from "@/components/ui/PageGrid";

type State = "loading" | "ok" | "degraded" | "unknown";
type Maintenance = { starts_at: string; ends_at: string; message?: string };

/** Public health of playback and the scheduled maintenance, from GET /api/v1/status (refreshed every minute). */
export default function StatusPage() {
  const { t, locale } = useI18n();
  const [state, setState] = useState<State>("loading");
  const [maintenance, setMaintenance] = useState<(Maintenance & { running: boolean }) | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      fetch(`${process.env.NEXT_PUBLIC_API_BASE || "/api/v1"}/status`, { cache: "no-store" })
        .then((res) => (res.ok ? res.json() : Promise.reject(new Error("status"))))
        .then((body: { playback?: string; maintenance?: Maintenance }) => {
          if (!active) return;
          setState(body.playback === "ok" || body.playback === "degraded" ? body.playback : "unknown");
          const m = body.maintenance;
          setMaintenance(m ? { ...m, running: new Date(m.starts_at).getTime() <= Date.now() } : null);
        })
        .catch(() => { if (active) setState("unknown"); });
    };
    load();
    const timer = window.setInterval(load, 60_000);
    return () => { active = false; window.clearInterval(timer); };
  }, []);

  const when = (iso: string) => new Date(iso).toLocaleString(locale, { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });

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
        {maintenance && (
          <section aria-label={t("Maintenance")}>
            <span className="eyebrow">{t("Maintenance")}</span>
            <p>{maintenance.running ? t("Maintenance en cours jusqu’à {end}.", { end: when(maintenance.ends_at) }) : t("Maintenance prévue du {start} au {end}.", { start: when(maintenance.starts_at), end: when(maintenance.ends_at) })}</p>
            {maintenance.message && <p className="history-empty">{maintenance.message}</p>}
          </section>
        )}
      </div>
    </main>
  );
}

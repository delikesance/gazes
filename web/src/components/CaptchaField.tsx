"use client";
import { useEffect, useRef, useState } from "react";
import { Check, Loader2, ShieldCheck } from "lucide-react";
import { solveFreshCaptcha } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";

type Status = "idle" | "solving" | "verified" | "error";

interface CaptchaFieldProps {
  /** Receives the solved payload, or `null` when it is invalid/consumed. */
  onChange: (payload: string | null) => void;
  /** Change this value to discard the current solution (a payload is single-use). */
  resetKey?: number;
}

/** Pill-styled proof-of-work control: one click, solved in a worker, no third party. */
export function CaptchaField({ onChange, resetKey = 0 }: CaptchaFieldProps) {
  const { t } = useI18n();
  // The status belongs to the reset generation it was produced in; a new generation reads as idle.
  const [state, setState] = useState<{ status: Status; key: number }>({ status: "idle", key: resetKey });
  const status: Status = state.key === resetKey ? state.status : "idle";
  const abort = useRef<AbortController | null>(null);

  useEffect(() => { abort.current?.abort(); }, [resetKey]);

  useEffect(() => () => abort.current?.abort(), []);

  const start = async () => {
    if (status === "solving" || status === "verified") return;
    abort.current?.abort();
    const controller = new AbortController();
    abort.current = controller;
    setState({ status: "solving", key: resetKey });
    try {
      const payload = await solveFreshCaptcha(controller.signal);
      if (controller.signal.aborted) return;
      setState({ status: "verified", key: resetKey });
      onChange(payload);
    } catch (error) {
      if ((error as Error).name === "AbortError") return;
      setState({ status: "error", key: resetKey });
      onChange(null);
    }
  };

  const label = {
    idle: t("Je ne suis pas un robot"),
    solving: t("Vérification…"),
    verified: t("Vérifié"),
    error: t("Échec de la vérification. Réessayer"),
  }[status];

  return (
    <button type="button" className="captcha-field" data-status={status} onClick={start} aria-live="polite" aria-pressed={status === "verified"}>
      <span className="captcha-box" aria-hidden="true">
        {status === "solving" ? <Loader2 size={14} className="animate-spin" /> : status === "verified" ? <Check size={14} /> : null}
      </span>
      <span>{label}</span>
      <ShieldCheck size={16} aria-hidden="true" className="captcha-shield" />
    </button>
  );
}

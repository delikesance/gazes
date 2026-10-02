"use client";

import { useState, type ReactNode } from "react";
import { X } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { copyText } from "@/lib/clipboard";

/** The one red error box used everywhere: message, error code, optional reference, copy / retry / close. */
export function ErrorAlert({ message, code, reference, onRetry, onClose, className = "", children }: {
  message: string;
  code?: string;
  /** Diagnostic reference (playback session id); shown in full under the message. */
  reference?: string | null;
  onRetry?: () => void;
  onClose?: () => void;
  className?: string;
  /** Extra actions rendered after the standard ones. */
  children?: ReactNode;
}) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const text = t(message);
  async function copy() {
    const report = [code && `Code: ${code}`, reference && `Reference: ${reference}`, text].filter(Boolean).join("\n");
    if (await copyText(report)) { setCopied(true); setTimeout(() => setCopied(false), 2000); }
  }
  return (
    <div role="alert" className={`error-alert ${className}`}>
      <div className="error-alert-body">
        <p>{text}</p>
        {(code || reference) && (
          <p className="error-alert-meta">
            {code && <code title={t("Code d’erreur")}>{code}</code>}
            {reference && <span>{t("Référence de diagnostic")} : <code>{reference}</code></span>}
          </p>
        )}
      </div>
      <div className="error-alert-actions">
        {onRetry && <button type="button" onClick={onRetry}>{t("Réessayer")}</button>}
        {(code || reference) && <button type="button" onClick={copy}>{t(copied ? "Copié" : "Copier")}</button>}
        {children}
        {onClose && <button type="button" aria-label={t("Fermer")} className="error-alert-close" onClick={onClose}><X className="h-3.5 w-3.5" /></button>}
      </div>
    </div>
  );
}

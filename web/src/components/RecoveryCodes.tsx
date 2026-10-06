"use client";
import { useState } from "react";
import { Check, Copy, Download } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { copyText } from "@/lib/clipboard";

/** One-time display of the recovery codes, with copy and download so they can be stored safely. */
export function RecoveryCodes({ codes, onDone, doneLabel }: { codes: string[]; onDone: () => void; doneLabel: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const text = codes.join("\n");

  const download = () => {
    const url = URL.createObjectURL(new Blob([`Gazes\n${text}\n`], { type: "text/plain" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "gazes-codes-de-secours.txt";
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="recovery-codes">
      <p>{t("Ces codes de secours permettent de retrouver votre compte si vous perdez votre mot de passe. Chaque code ne sert qu’une fois. Ils ne seront plus affichés : conservez-les maintenant.")}</p>
      <ul>{codes.map((code) => <li key={code}><code>{code}</code></li>)}</ul>
      <div className="recovery-actions">
        <button type="button" className="clay clay-secondary clay-sm" onClick={async () => { if (await copyText(text)) { setCopied(true); window.setTimeout(() => setCopied(false), 2000); } }}>
          {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}{t(copied ? "Copié" : "Copier")}
        </button>
        <button type="button" className="clay clay-secondary clay-sm" onClick={download}><Download size={14} aria-hidden="true" />{t("Télécharger")}</button>
        <button type="button" className="clay clay-primary clay-sm" onClick={onDone}>{doneLabel}</button>
      </div>
    </div>
  );
}

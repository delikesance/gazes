"use client";
import { useEffect, useId, useState, type FormEvent } from "react";
import { Bitcoin, ExternalLink, EyeOff, Loader2, ShieldCheck } from "lucide-react";
import { DonationError, MAX_NAME, PRESET_CENTS, createInvoice, eur, fetchPublicDonations, goalPercent, parseAmount, validName, type PublicDonations } from "@/lib/donations";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "./AuthProvider";

const ERRORS: Record<string, string> = {
  invalid_amount: "Montant invalide.",
  invalid_name: "Ce nom n’est pas valide : 2 à 24 lettres, chiffres, espaces ou - _ ’, sans lien.",
  unavailable: "Les dons ne sont pas encore ouverts.",
  provider_unavailable: "Le paiement crypto est momentanément indisponible. Réessayez dans quelques minutes.",
  rate_limited: "Trop de tentatives. Réessayez plus tard.",
  network: "Impossible de joindre le serveur.",
};

export function SupportPage() {
  const { t } = useI18n();
  const { user } = useAuth();
  const [info, setInfo] = useState<PublicDonations | null>(null);
  const [failed, setFailed] = useState(false);
  const [preset, setPreset] = useState<number | "custom">(500);
  const [custom, setCustom] = useState("");
  const [named, setNamed] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const ids = { custom: useId(), name: useId(), err: useId() };

  useEffect(() => {
    const ctl = new AbortController();
    fetchPublicDonations(ctl.signal).then(setInfo).catch((e) => { if (!ctl.signal.aborted) { setFailed(true); console.debug(e); } });
    return () => ctl.abort();
  }, []);

  const cents = preset === "custom" ? parseAmount(custom) : preset;
  const inRange = cents !== null && info !== null && cents >= info.min_cents && cents <= info.max_cents;
  const nameOk = !named || validName(name);
  const canPay = !!info?.btcpay && inRange && nameOk && !busy;
  const percent = info ? goalPercent(info) : null;

  async function pay(event: FormEvent) {
    event.preventDefault();
    if (!canPay || cents === null) return;
    setBusy(true);
    setError(null);
    try {
      const url = await createInvoice({ amount_cents: cents, visibility: named ? "named" : "anonymous", display_name: name });
      window.location.assign(url);
    } catch (e) {
      setError(ERRORS[e instanceof DonationError ? e.code : "network"] ?? "Une erreur est survenue. Réessayez.");
      setBusy(false);
    }
  }

  const open = !!info && (info.btcpay || !!info.kofi_url);

  return (
    <main className="history-page">
      <div className="history-inner page-inset support-page">
        <span className="eyebrow">{t("Soutenir Gazes")}</span>
        <h1 className="serif">{t("Aidez à garder Gazes en ligne")}</h1>
        <p className="support-lead">{t("Gazes est gratuit et sans publicité. Les dons servent uniquement à payer le serveur et la bande passante. Rien n’est retiré à ceux qui ne donnent pas.")}</p>

        {failed ? <p className="form-error" role="alert">{t("Impossible de charger la page de dons. Réessayez plus tard.")}</p> : null}
        {info && !open ? <section className="support-card"><h2>{t("Bientôt")}</h2><p>{t("Les dons ne sont pas encore ouverts. Merci de l’intention : repassez dans quelques jours.")}</p></section> : null}

        {info && percent !== null ? (
          <section className="support-card" aria-labelledby="support-goal">
            <h2 id="support-goal">{t("Coûts du mois")}</h2>
            <div className="support-meter" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent} aria-label={t("Part des coûts du mois couverte")}><span style={{ width: `${percent}%` }} /></div>
            <p className="support-meter-text">{t("{raised} sur {goal} couverts ce mois-ci", { raised: eur(info.month_cents ?? 0), goal: eur(info.goal_cents ?? 0) })}</p>
          </section>
        ) : null}

        {info && open ? (
          <form className="support-card" onSubmit={pay} aria-describedby={error ? ids.err : undefined}>
            {info.btcpay ? (
              <>
                <h2>{t("Donner en crypto")}</h2>
                <fieldset className="support-fieldset">
                  <legend>{t("Montant")}</legend>
                  <div className="support-amounts" role="radiogroup" aria-label={t("Montant")}>
                    {PRESET_CENTS.map((c) => (
                      <button key={c} type="button" role="radio" aria-checked={preset === c} className="support-chip" data-active={preset === c} onClick={() => setPreset(c)}>{eur(c)}</button>
                    ))}
                    <button type="button" role="radio" aria-checked={preset === "custom"} className="support-chip" data-active={preset === "custom"} onClick={() => setPreset("custom")}>{t("Autre")}</button>
                  </div>
                  {preset === "custom" ? (
                    <div className="auth-field">
                      <label className="auth-label" htmlFor={ids.custom}><span>{t("Montant en euros")}</span></label>
                      <div className="field-pill" data-invalid={custom !== "" && !inRange}>
                        <input id={ids.custom} inputMode="decimal" autoComplete="off" placeholder="5" value={custom} onChange={(e) => setCustom(e.target.value)} aria-invalid={custom !== "" && !inRange} />
                        <span aria-hidden="true">€</span>
                      </div>
                      {custom !== "" && !inRange ? <p className="field-error">{t("Entre {min} et {max}.", { min: eur(info.min_cents), max: eur(info.max_cents) })}</p> : null}
                    </div>
                  ) : null}
                </fieldset>

                <fieldset className="support-fieldset">
                  <legend>{t("Votre nom sur le mur des donateurs")}</legend>
                  <label className="support-radio"><input type="radio" name="vis" checked={!named} onChange={() => setNamed(false)} /><span><strong>{t("Rester anonyme")} {t("(par défaut)")}</strong><small>{t("Personne ne voit votre don.")}</small></span></label>
                  <label className="support-radio"><input type="radio" name="vis" checked={named} onChange={() => setNamed(true)} /><span><strong>{t("Afficher un nom")}</strong><small>{t("Le nom que vous choisissez apparaît ci-dessous après le paiement.")}</small></span></label>
                  {named ? (
                    <div className="auth-field">
                      <label className="auth-label" htmlFor={ids.name}><span>{t("Nom affiché")}</span><span>{name.length}/{MAX_NAME}</span></label>
                      <div className="field-pill" data-invalid={name !== "" && !nameOk}>
                        <input id={ids.name} maxLength={MAX_NAME} autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} aria-invalid={name !== "" && !nameOk} />
                      </div>
                      {name !== "" && !nameOk ? <p className="field-error">{t("2 à 24 lettres, chiffres, espaces ou - _ ’, sans lien.")}</p> : null}
                    </div>
                  ) : null}
                </fieldset>

                {user ? <p className="support-note"><ShieldCheck size={15} aria-hidden="true" />{t("Ce don sera rattaché à votre compte, visible uniquement par l’administrateur.")}</p> : null}
                {error ? <p id={ids.err} className="form-error" role="alert">{error}</p> : null}
                <button type="submit" className="design-button support-pay" disabled={!canPay}>
                  {busy ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Bitcoin size={16} aria-hidden="true" />}
                  {cents !== null && inRange ? t("Donner {amount} en crypto", { amount: eur(cents) }) : t("Choisissez un montant")}
                </button>
              </>
            ) : null}
            {info.kofi_url ? (
              <a className={`design-button secondary-button support-pay${info.btcpay ? "" : " support-pay--solo"}`} href={info.kofi_url} target="_blank" rel="noopener noreferrer">
                <ExternalLink size={16} aria-hidden="true" />{t("Donner sur Ko-fi")}
              </a>
            ) : null}
          </form>
        ) : null}

        <section className="support-card" aria-labelledby="support-privacy">
          <h2 id="support-privacy"><EyeOff size={16} aria-hidden="true" /> {t("Ce qui est enregistré")}</h2>
          <ul className="support-list">
            <li>{t("Le montant, la date, le moyen de paiement et la référence de la transaction : visibles uniquement par l’administrateur.")}</li>
            <li>{t("Si vous êtes connecté, le don est rattaché à votre compte pour que l’administrateur puisse vous remercier.")}</li>
            <li>{t("Jamais votre e-mail ni vos coordonnées bancaires. Le paiement crypto ne demande aucun compte.")}</li>
            <li>{t("Publiquement : seulement le nom que vous avez choisi d’afficher, jamais le montant.")}</li>
          </ul>
        </section>

        {info ? (
          <section className="support-card" aria-labelledby="support-wall">
            <h2 id="support-wall">{t("Merci à")}</h2>
            {info.donors.length > 0 ? (
              <ul className="support-wall">{info.donors.map((d) => <li key={d}>{d}</li>)}</ul>
            ) : (
              <p>{t("Les premiers noms apparaîtront ici. La plupart des donateurs restent anonymes, et c’est très bien.")}</p>
            )}
          </section>
        ) : null}
      </div>
    </main>
  );
}

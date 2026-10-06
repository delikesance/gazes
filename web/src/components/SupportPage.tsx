"use client";
import { useEffect, useId, useState, type FormEvent } from "react";
import { Bitcoin, Check, Download, ExternalLink, EyeOff, HardDrive, Heart, Loader2, Server, ShieldCheck, type LucideIcon } from "lucide-react";
import { DonationError, MAX_NAME, PRESET_CENTS, createInvoice, eur, fetchPublicDonations, goalPercent, parseAmount, validName, type PublicDonations } from "@/lib/donations";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "./AuthProvider";
import { PageGrid } from "./ui/PageGrid";
import { Scribble } from "./ui/Scribble";

const ERRORS: Record<string, string> = {
  invalid_amount: "Montant invalide.",
  invalid_name: "Ce nom n’est pas valide : 2 à 24 lettres, chiffres, espaces ou - _ ’, sans lien.",
  unavailable: "Les dons ne sont pas encore ouverts.",
  provider_unavailable: "Le paiement crypto est momentanément indisponible. Réessayez dans quelques minutes.",
  rate_limited: "Trop de tentatives. Réessayez plus tard.",
  network: "Impossible de joindre le serveur.",
};

/** Indicative split of the hosting costs (not measured: the admin cost inputs only give a total). */
const USES: { icon: LucideIcon; pct: number; title: string; text: string }[] = [
  { icon: Server, pct: 45, title: "Le serveur", text: "Il convertit chaque épisode à la volée pour que la lecture démarre vite, même en soirée." },
  { icon: Download, pct: 30, title: "La bande passante", text: "Chaque épisode diffusé en HD consomme des gigaoctets. C’est le poste qui grossit avec la communauté." },
  { icon: HardDrive, pct: 10, title: "Le stockage", text: "Garder les épisodes les plus regardés à portée de main, prêts à se lancer en un clic." },
  { icon: ShieldCheck, pct: 15, title: "Le VPN", text: "Une connexion chiffrée pour aller chercher les sources en toute discrétion et garder Gazes en ligne." },
];

const PROMISES: { title: string; text: string }[] = [
  { title: "Tout reste accessible", text: "Aucune fonctionnalité n’est réservée aux donateurs. Ce que vous voyez, tout le monde le voit." },
  { title: "Jamais de publicité", text: "Les dons remplacent les annonceurs. Pas de bannière, pas de pistage, pas de revente de données." },
  { title: "Un objectif visible de tous", text: "Le total du mois et l’objectif sont publics. Le montant de chaque don, jamais." },
  { title: "Aucun compte requis", text: "Pas d’e-mail, pas de coordonnées bancaires pour donner en crypto. Vous pouvez rester anonyme." },
];

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
  const [daysLeft, setDaysLeft] = useState<number | null>(null);
  const ids = { custom: useId(), name: useId(), err: useId() };

  useEffect(() => {
    const ctl = new AbortController();
    fetchPublicDonations(ctl.signal).then((d) => {
      const now = new Date();
      setDaysLeft(new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate() - now.getDate());
      setInfo(d);
    }).catch((e) => { if (!ctl.signal.aborted) { setFailed(true); console.debug(e); } });
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
      <PageGrid />
      <Scribble shape="a" width={420} rotate={12} style={{ right: -60, top: 110 }} />
      <div className="history-inner page-inset support-page">
        <section className="support-hero">
          <div className="support-hero-copy">
            <span className="eyebrow">{t("Soutenir Gazes")}</span>
            <h1 className="serif">{t("Gardez Gazes gratuit, rapide et sans pub.")}</h1>
            <p className="support-lead">{t("Gazes est un projet indépendant. Chaque épisode demande un serveur, de la bande passante et un VPN. Votre don paie cet hébergement, et rien d’autre. Rien n’est retiré à ceux qui ne donnent pas.")}</p>
            <ul className="support-chips">
              <li>{t("Gratuit pour tous")}</li>
              <li>{t("Zéro publicité")}</li>
              <li>{t("Aucune donnée revendue")}</li>
            </ul>
          </div>

          <div className="support-hero-card" id="don">
            {failed ? <p className="form-error" role="alert">{t("Impossible de charger la page de dons. Réessayez plus tard.")}</p> : null}
            {info && !open ? <section className="support-card"><h2>{t("Bientôt")}</h2><p>{t("Les dons ne sont pas encore ouverts. Merci de l’intention : repassez dans quelques jours.")}</p></section> : null}

            {info && open ? (
              <form className="support-card support-card--main" onSubmit={pay} aria-describedby={error ? ids.err : undefined}>
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
          </div>
        </section>

        {info && percent !== null ? (
          <section className="support-card support-goal" aria-labelledby="support-goal">
            <div className="support-goal-head">
              <div>
                <span className="eyebrow">{t("Objectif du mois")}</span>
                <h2 id="support-goal" className="support-goal-title">{t("{raised} sur {goal} couverts ce mois-ci", { raised: eur(info.month_cents ?? 0), goal: eur(info.goal_cents ?? 0) })}</h2>
              </div>
              {daysLeft !== null ? <span className="support-mono">{t("Fin du mois dans {days} j", { days: daysLeft })}</span> : null}
            </div>
            <div className="support-meter" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent} aria-label={t("Part des coûts du mois couverte")}><span style={{ width: `${percent}%` }} /></div>
            <p>{t("Une fois l’objectif atteint, les dons suivants financent les nouveautés : plus de sources, de meilleurs sous-titres, de nouvelles fonctionnalités.")}</p>
          </section>
        ) : null}

        <section className="support-section" aria-labelledby="support-uses">
          <div className="support-section-head">
            <span className="eyebrow">{t("Transparence")}</span>
            <h2 id="support-uses" className="support-h2">{t("Où va chaque euro")}</h2>
            <p>{t("Pas de flou : voici ce que votre don finance, poste par poste. Répartition indicative.")}</p>
          </div>
          <ul className="support-uses">
            {USES.map(({ icon: Icon, pct, title, text }) => (
              <li key={title} className="support-use">
                <span className="support-icon"><Icon size={24} aria-hidden="true" /></span>
                <span className="support-mono support-pct">{pct} %</span>
                <h3>{t(title)}</h3>
                <p>{t(text)}</p>
              </li>
            ))}
          </ul>
        </section>

        <section className="support-section support-promise" aria-labelledby="support-promise">
          <div className="support-section-head">
            <span className="eyebrow">{t("Notre promesse")}</span>
            <h2 id="support-promise" className="support-h2 serif">{t("Donner ne change rien pour vous. Ça change tout pour Gazes.")}</h2>
          </div>
          <ul className="support-promises">
            {PROMISES.map(({ title, text }) => (
              <li key={title}>
                <Check size={24} aria-hidden="true" />
                <div><h3>{t(title)}</h3><p>{t(text)}</p></div>
              </li>
            ))}
          </ul>
        </section>

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
          <section className="support-section" aria-labelledby="support-wall">
            <div className="support-section-head">
              <span className="eyebrow">{t("Mur des donateurs")}</span>
              <h2 id="support-wall" className="support-h2">{t("Merci à")}</h2>
            </div>
            {info.donors.length > 0 ? (
              <ul className="support-wall">{info.donors.map((d) => <li key={d}>{d}</li>)}</ul>
            ) : (
              <p className="support-wall-empty"><Heart size={24} aria-hidden="true" />{t("Les premiers noms apparaîtront ici. Le vôtre sera peut-être le premier.")}</p>
            )}
            <p className="support-fine">{t("Seuls les noms choisis apparaissent ici. La plupart des donateurs restent anonymes, et c’est très bien comme ça.")}</p>
          </section>
        ) : null}

        {open ? (
          <section className="support-card support-cta" aria-labelledby="support-cta">
            <div>
              <h2 id="support-cta" className="support-h2 serif">{t("Votre prochain épisode commence ici.")}</h2>
              <p>{t("Même 3 € aident. Merci de faire partie de ce qui rend Gazes possible.")}</p>
            </div>
            <a className="design-button" href="#don"><Heart size={16} aria-hidden="true" />{t("Faire un don")}</a>
          </section>
        ) : null}
      </div>
    </main>
  );
}

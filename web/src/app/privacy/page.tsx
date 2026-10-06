"use client";
import { useI18n } from "@/lib/i18n";

const CONTENT = {
  fr: {
    eyebrow: "Confidentialité",
    title: "Vos données de visionnage",
    sections: [
      ["Ce que Gazes enregistre", "Pour reprendre votre lecture et vous suggérer des animes, Gazes enregistre chaque séance de visionnage : la série, la saison et l’épisode, ses genres, l’heure et la durée réellement regardée, la position atteinte, si l’épisode est terminé, la langue audio et celle des sous-titres choisies, ainsi que votre fuseau horaire. Les animes que vous marquez « pas intéressé » sont aussi conservés."],
      ["Où c’est stocké", "Sans compte, ces données restent dans votre navigateur, sur cet appareil. Avec un compte, elles sont aussi enregistrées sur le serveur de Gazes pour se synchroniser entre vos appareils."],
      ["À quoi cela sert", "Uniquement à reprendre là où vous vous êtes arrêté et à personnaliser l’onglet Suggestions. Ces données ne sont ni vendues ni partagées. Pour les recommandations, seuls des identifiants d’animés sont envoyés à AniList, jamais votre identité."],
      ["Vos choix", "Connecté, vous pouvez à tout moment exporter vos données (« Exporter mes données ») ou effacer votre journal de visionnage et vos « pas intéressé » (« Effacer mon journal ») depuis le menu du compte. Sans compte, vider les données du site dans votre navigateur suffit."],
      ["Durée de conservation", "Le journal est conservé tant que vous ne l’effacez pas."],
      ["Dons", "Si vous faites un don, Gazes enregistre le montant, la date, le moyen de paiement, la référence de la transaction et, si vous êtes connecté, votre compte. Seul l’administrateur y a accès. Publiquement, seul apparaît le nom que vous avez choisi d’afficher, jamais le montant ; sans choix de votre part, le don est anonyme. Votre e-mail et vos coordonnées bancaires ne sont jamais enregistrés. Pour retirer un nom affiché ou faire effacer un don, écrivez à l’administrateur."],
    ],
  },
  en: {
    eyebrow: "Privacy",
    title: "Your watching data",
    sections: [
      ["What Gazes records", "To resume playback and suggest anime, Gazes records each watching session: the series, season and episode, its genres, the time and the duration actually watched, the position reached, whether the episode was finished, the audio and subtitle languages chosen, and your time zone. Anime you mark “not interested” are kept too."],
      ["Where it is stored", "Without an account, this data stays in your browser, on this device. With an account, it is also saved on the Gazes server so it syncs across your devices."],
      ["What it is for", "Only to resume where you left off and to personalise the Suggestions tab. It is neither sold nor shared. For recommendations, only anime identifiers are sent to AniList, never your identity."],
      ["Your choices", "Signed in, you can at any time export your data (“Export my data”) or erase your watch log and your “not interested” list (“Erase my log”) from the account menu. Without an account, clearing the site's data in your browser is enough."],
      ["How long it is kept", "The log is kept until you erase it."],
      ["Donations", "If you donate, Gazes records the amount, date, payment method, transaction reference and, if you are signed in, your account. Only the administrator can see it. Publicly, only the name you chose to show appears, never the amount; unless you choose otherwise, the donation is anonymous. Your e-mail and bank details are never stored. To remove a displayed name or have a donation erased, contact the administrator."],
    ],
  },
} as const;

export default function PrivacyPage() {
  const { locale } = useI18n();
  const content = CONTENT[locale === "en" ? "en" : "fr"];
  return (
    <main className="history-page">
      <div className="history-inner page-inset legal-page">
        <span className="eyebrow">{content.eyebrow}</span>
        <h1 className="serif">{content.title}</h1>
        {content.sections.map(([heading, body]) => (
          <section key={heading}>
            <h2>{heading}</h2>
            <p>{body}</p>
          </section>
        ))}
      </div>
    </main>
  );
}

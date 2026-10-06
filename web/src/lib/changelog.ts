/** Public dev log. Newest entry first; `date` (YYYY-MM-DD) is also its identity for the announcement. */
export type ChangelogEntry = {
  date: string;
  fr: { title: string; items: readonly string[] };
  en: { title: string; items: readonly string[] };
};

export const CHANGELOG: readonly ChangelogEntry[] = [
  {
    date: "2026-10-06",
    fr: { title: "Nouveautés et journal de développement", items: [
      "Cette page : retrouvez ici ce qui change sur Gazes, mise à jour par mise à jour.",
      "Le compte à rebours du prochain épisode est plus fiable.",
      "Mise en page téléphone repensée : barre d’onglets flottante, rails d’affiches, synopsis repliable et lecteur adapté.",
      "Nouvelle page « Pour vous » : un flux infini de recommandations, accessible depuis la barre du bas sur téléphone.",
      "Accueil : « Reprendre la lecture » et « Ma liste » passent au-dessus du calendrier, en rail défilant.",
    ] },
    en: { title: "What’s new and dev log", items: [
      "This page: follow what changes on Gazes, update by update.",
      "The next-episode countdown is more reliable.",
      "Phone layout reworked: floating tab bar, poster rails, collapsible synopsis and a player built for small screens.",
      "New “For you” page: an endless feed of recommendations, reachable from the bottom bar on phones.",
      "Home: “Continue watching” and “My list” now sit above the calendar, as a scrolling rail.",
    ] },
  },
  {
    date: "2026-10-05",
    fr: { title: "Lecture plus fiable", items: [
      "Les montages de fans et versions recoupées ne sont plus proposés à la place des vrais épisodes.",
      "Si la recherche de sources échoue, l’erreur s’affiche au lieu d’un chargement sans fin.",
      "Les aperçus d’épisodes se chargent plus vite.",
    ] },
    en: { title: "More reliable playback", items: [
      "Fan edits and recuts are no longer offered in place of the real episodes.",
      "When the source search fails, the error is shown instead of an endless loader.",
      "Episode previews load faster.",
    ] },
  },
  {
    date: "2026-10-04",
    fr: { title: "Suggestions et recherche par genres", items: [
      "L’onglet « Pour vous » propose des animes selon ce que vous regardez.",
      "Bouton « Pas intéressé » pour écarter une suggestion.",
      "Recherche par plusieurs genres : clic pour inclure, clic droit ou appui long pour exclure.",
      "Nouvelle page Historique en grille d’affiches, et page Confidentialité avec export et effacement de vos données.",
      "Les épisodes déjà en bibliothèque démarrent sans attendre la recherche de sources.",
    ] },
    en: { title: "Suggestions and genre search", items: [
      "The “For you” tab suggests anime based on what you watch.",
      "“Not interested” button to dismiss a suggestion.",
      "Search by several genres: click to include, right click or long press to exclude.",
      "New poster-grid History page, and a Privacy page to export or erase your data.",
      "Episodes already in the library start without waiting for the source search.",
    ] },
  },
  {
    date: "2026-10-02",
    fr: { title: "Nouvelle page d’accueil", items: [
      "Accueil redessiné avec un carrousel à la une et un calendrier des sorties de la saison.",
      "Sous-titres image (PGS) affichés dans le navigateur.",
      "Les erreurs de lecture affichent un code à copier pour nous aider à les corriger.",
    ] },
    en: { title: "New home page", items: [
      "Redesigned home with a featured carousel and the season’s release calendar.",
      "Image subtitles (PGS) rendered in the browser.",
      "Playback errors show a code you can copy to help us fix them.",
    ] },
  },
];

/** How long a new entry is announced in the site-wide banner. */
export const ANNOUNCE_DAYS = 14;
const DAY_MS = 86_400_000;

/** The entry to announce, or null when the latest one is older than ANNOUNCE_DAYS or was already seen. */
export function announcedEntry(entries: readonly ChangelogEntry[], now: number, seen: string | null): ChangelogEntry | null {
  const latest = entries[0];
  if (!latest || latest.date === seen) return null;
  const published = Date.parse(`${latest.date}T00:00:00Z`);
  if (Number.isNaN(published) || now < published || now - published > ANNOUNCE_DAYS * DAY_MS) return null;
  return latest;
}

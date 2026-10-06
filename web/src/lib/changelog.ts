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
      "Téléphone : un bouton ← en haut des fiches et saisons ramène à la page d’où vous venez, et « Reprendre » apparaît directement sur la fiche d’une série en cours.",
      "Bibliothèque : reprise de lecture et Ma liste réunies. Sur téléphone, une puce « Reprendre » reste à portée de pouce, et un bouton lecture apparaît sur les affiches.",
      "Recherche à la frappe, avec vos dernières recherches. Le geste retour ferme la recherche et les menus au lieu de quitter la page.",
      "Le calendrier retient sa vue et son jour, le flux « Pour vous » garde sa position au retour, et chaque sortie peut s’ajouter à Ma liste en un toucher. Raccourcis clavier : g puis c, p, b ou n, et u pour remonter d’un niveau.",
      "Ordinateur : menu Catalogue · Pour vous · Bibliothèque dans l’en-tête, bouton « Reprendre » toujours visible, retour vers vos résultats de recherche depuis une fiche, lecture et Ma liste au survol des affiches. Appuyez sur ? pour voir les raccourcis clavier.",
      "Nouvelle page « Pour vous » : un flux infini de recommandations, accessible depuis la barre du bas sur téléphone.",
      "Accueil : « Reprendre la lecture » et « Ma liste » passent au-dessus du calendrier, en rail défilant.",
    ] },
    en: { title: "What’s new and dev log", items: [
      "This page: follow what changes on Gazes, update by update.",
      "The next-episode countdown is more reliable.",
      "Phone layout reworked: floating tab bar, poster rails, collapsible synopsis and a player built for small screens.",
      "Phone: a ← button at the top of series and season pages takes you back to where you came from, and “Resume” now shows right on the page of a series in progress.",
      "Library: resume and My list in one place. On phones a “Resume” chip stays within thumb reach and posters get a play button.",
      "Search as you type, with your latest searches. The back gesture closes search and menus instead of leaving the page.",
      "The calendar remembers its view and day, the “For you” feed keeps its position on the way back, and every release can be added to My list in one tap. Keyboard shortcuts: g then c, p, b or n, and u to go up a level.",
      "Computer: Catalogue · For you · Library menu in the header, an always-visible “Resume” button, a way back to your search results from a series page, and play and My list on poster hover. Press ? to see the keyboard shortcuts.",
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

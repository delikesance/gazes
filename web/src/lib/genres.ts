/** Filters the catalogue search accepts: AniList genres first, then popular tags. Values are the English names the API expects. */
export const GENRES: [value: string, label: string][] = [
  ["Action", "Action"], ["Adventure", "Aventure"], ["Comedy", "Comédie"], ["Drama", "Drame"], ["Ecchi", "Ecchi"], ["Fantasy", "Fantastique"],
  ["Horror", "Horreur"], ["Mahou Shoujo", "Magical girl"], ["Mecha", "Mecha"], ["Music", "Musique"], ["Mystery", "Mystère"], ["Psychological", "Psychologique"],
  ["Romance", "Romance"], ["Sci-Fi", "Science-fiction"], ["Slice of Life", "Tranche de vie"], ["Sports", "Sport"], ["Supernatural", "Surnaturel"], ["Thriller", "Thriller"],
  ["Isekai", "Isekai"], ["Reincarnation", "Réincarnation"], ["Magic", "Magie"], ["School", "École"], ["Harem", "Harem"], ["Gore", "Gore"], ["Shounen", "Shonen"], ["Seinen", "Seinen"],
];

export function genreLabel(value: string): string {
  return GENRES.find(([v]) => v === value)?.[1] ?? value;
}

export function parseList(raw: string | null | undefined): string[] {
  return (raw ?? "").split(",").map((part) => part.trim()).filter(Boolean);
}

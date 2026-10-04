# Passer l'opening et l'ending — design

Date : 2026-10-04

## Objectif

Pendant la lecture d'un épisode, afficher un bouton « Passer l'opening » / « Passer l'ending » lorsque le
lecteur se trouve dans un opening ou un ending détecté. Un clic amène à la fin du segment.

Succès :

- La majorité des releases (MKV chapitrés) obtiennent le bouton sans appel externe.
- Les fichiers sans chapitres exploitables obtiennent le bouton grâce à AniSkip quand la base le connaît.
- Une détection incertaine n'affiche rien : jamais de bouton qui saute au mauvais endroit.
- Aucune régression de la lecture : un échec de détection ne bloque ni ne retarde la lecture.

Hors périmètre : saut automatique, détection par empreinte audio, saut du récapitulatif ou de l'aperçu
de l'épisode suivant, chapitres ordonnés (segment linking vers des fichiers OP/ED externes).

## Vue d'ensemble

```
MKV (torrent) ──ProbeReader──▶ chapitres (parseur natif) ──DetectSkipSegments──▶ VideoMetadata.skip_segments
                                                                                       │
AniList idMal + durée ──GET …/skip-times──▶ AniSkip (cache kv) ───────────────────────▶ │ (types manquants)
                                                                                       ▼
                                                          Lecteur : segment actif ▶ bouton ▶ handleSeek(fin)
```

## 1. Lecture des chapitres Matroska

Nouveau fichier `internal/metadata/matroska_chapters.go`.

```go
type Chapter struct {
	Start float64 `json:"start"` // secondes
	End   float64 `json:"end"`   // secondes ; 0 si absent dans le fichier
	Title string  `json:"title"`
}

func readMatroskaChapters(ctx context.Context, r io.ReadSeeker, size int64) ([]Chapter, error)
```

- Parcourt les enfants de premier niveau du `Segment` (0x18538067) jusqu'au premier `Cluster` (0x1F43B675),
  sans jamais lire de cluster.
- Trouve `Chapters` (0x1043A770) soit directement parmi ces enfants, soit via une entrée de la `SeekHead`
  (0x114D9B74 → Seek 0x4DBB → SeekID 0x53AB / SeekPosition 0x53AC, position relative au début des données du
  Segment). Couvre ainsi les chapitres écrits en fin de fichier.
- Budget de lecture borné : au plus 1 Mo lus au total et un élément `Chapters` d'au plus 512 Ko ; au-delà,
  erreur.
- Édition retenue : la première `EditionEntry` (0x45B9) portant `EditionFlagDefault` (0x45DB) = 1, sinon la
  première. Une édition `EditionFlagOrdered` (0x45DD) = 1 est ignorée entièrement (chapitres ordonnés hors
  périmètre) ; si toutes les éditions sont ordonnées, aucun chapitre.
- Dans l'édition : chaque `ChapterAtom` (0xB6) de premier niveau (les atomes imbriqués sont ignorés),
  `ChapterTimeStart` (0x91) et `ChapterTimeEnd` (0x92) en nanosecondes, ignorés si `ChapterFlagHidden` (0x98) = 1
  ou `ChapterFlagEnabled` (0x4598) = 0 ou si `ChapterSegmentUID` (0x6E67) est présent. Titre : le premier
  `ChapString` (0x85) des `ChapterDisplay` (0x80), en privilégiant une `ChapLanguage` (0x437C) `eng`, sinon le
  premier trouvé.
- Résultat trié par `Start`. Un `End` manquant vaut le `Start` du chapitre suivant ; pour le dernier, la durée
  du fichier est appliquée plus tard (voir §2).
- Entrée non Matroska, sans chapitres ou illisible : `nil, nil` pour « pas de chapitres », erreur pour les
  données corrompues / budget dépassé. L'appelant traite les deux comme « pas de chapitres ».

Intégration dans `FFprobeAnalyzer.ProbeReader` (`internal/metadata/probe.go`) : après un probe ffprobe réussi,
si le lecteur implémente `io.ReadSeeker`, rembobiner, appeler `readMatroskaChapters` avec un contexte limité à
1 s (via `ReadContext` quand le lecteur l'expose, comme pour le probe), puis rembobiner à 0. Un échec est
journalisé en Debug et ignoré. `VideoMetadata` gagne :

```go
Chapters     []Chapter     `json:"chapters,omitempty"`
SkipSegments []SkipSegment `json:"skip_segments,omitempty"`
```

Les deux chemins de lecture (HLS via `internal/playback/manager.go`, lecteur classique via
`internal/api/torrent_handlers.go`) passent par `ProbeReader`. Le pipeline de remux
(`internal/stream/pipeline_impl.go`), qui sonde à chaque requête de flux et n'utilise que les codecs, ne lit pas
les chapitres.

## 2. Détection

Nouveau fichier `internal/metadata/skip_segments.go`, fonction pure :

```go
type SkipSegment struct {
	Kind   string  `json:"kind"`   // "opening" | "ending"
	Start  float64 `json:"start"`  // secondes
	End    float64 `json:"end"`    // secondes
	Source string  `json:"source"` // "chapters" | "aniskip"
}

func DetectSkipSegments(chapters []Chapter, duration float64) []SkipSegment
```

Préparation : `End` manquant (0) du dernier chapitre → `duration` ; chapitres de longueur ≤ 0 ignorés ;
`duration` ≤ 0 → aucun segment.

Étape 1 — titres. Comparaison insensible à la casse, sur des mots entiers :

- opening : `op`, `op1`…`op9`, `opening`, `intro`, `introduction`, `ouverture`, `générique de début`,
  `generique de debut`, `opening song`, `オープニング` ;
- ending : `ed`, `ed1`…`ed9`, `ending`, `outro`, `credits`, `générique de fin`, `generique de fin`,
  `ending song`, `エンディング`.

Un titre qui correspond aux deux listes est ignoré. Un segment issu d'un titre doit durer entre 20 et 180 s.
Si plusieurs chapitres d'un même type correspondent, on garde le premier pour l'opening et le dernier pour
l'ending ; pour l'opening, un mot-clé fort (tous sauf `intro` et `introduction`) l'emporte sur `intro` /
`introduction`, souvent utilisés pour le cold open.

Étape 2 — durée et position, uniquement pour les types non trouvés à l'étape 1 et uniquement parmi les
chapitres dont le titre ne correspond à aucune des deux listes :

- candidat opening : durée entre 75 et 110 s, début avant `duration / 3` ;
- candidat ending : durée entre 75 et 110 s, début après `duration * 0.75`.

On retient le premier candidat opening et le dernier candidat ending. Un même chapitre ne peut pas être à la
fois opening et ending.

Garde-fous : un fichier de moins de 5 minutes ne produit aucun segment ; un chapitre couvrant plus de la moitié
du fichier n'est jamais retenu ; au plus un segment par type ; un opening doit se terminer avant le début de
l'ending retenu, sinon les deux sont abandonnés. Résultat trié par `Start`, `Source = "chapters"`.

`ProbeReader` remplit `SkipSegments = DetectSkipSegments(Chapters, DurationSec)`.

## 3. AniSkip en secours

### Identifiant MAL

`aniListMediaItem` et la requête `animeDetailWithEpisodesQuery` gagnent `idMal` ; `AnimeCatalogItem` gagne
`MalID int \`json:"mal_id,omitempty"\``. Les domaines de cache `detail:v2` restent valides (un champ absent vaut
0 → pas d'appel AniSkip pour ces entrées jusqu'à leur rafraîchissement).

### Client

Nouveau fichier `internal/metadata/aniskip.go`, méthode de `AnimeCatalogService` (même schéma que les autres
caches du service : cache local créé par `NewAnimeCatalogService`, recréé sur Redis par `SetRedis` ; appels AniSkip
faits avec le même `http.Client` que les appels AniList ; nouveau champ non exporté `aniskipBaseURL`, par défaut
`https://api.aniskip.com`, que les tests du paquet `metadata` pointent sur un serveur `httptest`) :

```go
func (s *AnimeCatalogService) SkipTimes(ctx context.Context, malID, episode int, duration float64) ([]SkipSegment, error)
```

Le serveur API détient déjà un `*metadata.AnimeCatalogService` concret : aucune interface à modifier. Les tests
du handler suivent `internal/api/catalog_handlers_test.go` : un faux `http.Transport` (comme `catalogTransport`)
répond aux requêtes AniList et AniSkip selon l'hôte.

- Requête : `GET {baseURL}/v2/skip-times/{malID}/{episode}?types=op&types=ed&episodeLength={duration}`
  (`baseURL` par défaut `https://api.aniskip.com`, surchargeable pour les tests). Timeout 4 s.
- Réponse `{"found":bool,"results":[{"interval":{"startTime":f,"endTime":f},"skipType":"op"|"ed","episodeLength":f}]}`.
  404 avec `found:false` = « rien trouvé » (pas une erreur).
- Validation de chaque résultat : `|episodeLength − duration| ≤ 3 s`, `0 ≤ startTime < endTime ≤ duration + 1`,
  longueur entre 20 et 180 s ; sinon ignoré. Au plus un segment par type (le premier valide).
  `op` → `opening`, `ed` → `ending`, `Source = "aniskip"`.
- Cache kv (domaine `aniskip`), clé `{malID}:{episode}:{durée arrondie à la seconde}` : 7 jours si au moins un
  segment, 1 jour si rien trouvé. Les erreurs réseau / 5xx / JSON invalide ne sont pas mises en cache et sont
  renvoyées.
- `malID ≤ 0`, `episode ≤ 0` ou `duration ≤ 0` : aucun appel, résultat vide.

### Endpoint

`GET /api/v1/catalog/seasons/{season}/episodes/{ep}/skip-times?duration={secondes}` dans le groupe `cat` de
`internal/api/router.go`, handler dans un nouveau fichier `internal/api/skip_handlers.go`.

- 400 si `season`, `ep` ou `duration` invalides (non numériques, ≤ 0, NaN/Inf, `duration` > 6 h).
- Récupère la saison par `GetAnimeDetailsWithEpisodes(season)` pour obtenir `MalID`.
- 200 `{"segments":[…]}` dans tous les cas où la requête est valide, y compris liste vide quand la saison est
  introuvable, sans `MalID`, ou quand AniSkip échoue (l'échec est journalisé en Warn).
- `Cache-Control: private, max-age=3600` quand la liste vient d'une réponse AniSkip réussie, `no-store` sinon.

## 4. Lecteur

### Logique pure

Nouveau module `web/src/lib/skip-segments.ts` :

```ts
export interface SkipSegment { kind: 'opening' | 'ending'; start: number; end: number; source: 'chapters' | 'aniskip' }
export function mergeSkipSegments(fromChapters: SkipSegment[], fromAniSkip: SkipSegment[]): SkipSegment[]
export function activeSkipSegment(segments: SkipSegment[], position: number): SkipSegment | null
export function needsAniSkip(fromChapters: SkipSegment[]): boolean
```

- `mergeSkipSegments` : par type, le segment issu des chapitres l'emporte ; AniSkip ne complète que les types
  absents ; résultat trié par `start`.
- `activeSkipSegment` : segment tel que `start ≤ position < end − 1` (le bouton disparaît dans la dernière
  seconde) ; `null` sinon.
- `needsAniSkip` : vrai si l'un des deux types manque.

### Intégration dans `web/src/components/VideoPlayerModal.tsx`

- `VideoMetadata` (`web/src/types/api.ts`) gagne `chapters?` et `skip_segments?`.
- Quand les métadonnées vidéo sont connues (`videoMeta`, durée > 0), si `needsAniSkip(skip_segments)` et que
  `seasonId > 0` et `episodeNumber` sont connus, appeler une fois l'endpoint `skip-times` avec la durée
  (fonction `getSkipTimes` dans le client API existant du frontend). Erreur → ignorée.
- Position comparée : la même position absolue que celle passée à `handleSeek` et à `onProgress`
  (`playbackOffset + currentTime`). L'implémentation doit vérifier qu'en mode HLS cette position est bien sur
  la timeline du fichier (chapitres) et, sinon, appliquer le décalage `timeline_origin` reçu de la session.
- Nouveau composant `web/src/components/SkipSegmentButton.tsx`, affiché tant qu'un segment est actif, même
  quand les contrôles sont masqués, en bas à droite au-dessus de la barre de contrôles. Libellés : « Passer
  l'opening », « Passer l'ending ». Style plat (pas de dégradé), cohérent avec les `player-pill` existants.
- Clic : `handleSeek(segment.end)`. Exception : pour un ending dont `end ≥ durée totale − 5` et quand
  `onNextEpisode` existe, le libellé devient « Épisode suivant » et le clic appelle `onNextEpisode`.
- Raccourci clavier `S` (dans le gestionnaire clavier existant) : même action que le clic quand un segment est
  actif.
- i18n : les trois libellés ajoutés au dictionnaire de `web/src/lib/i18n.ts` (« Skip opening », « Skip
  ending », « Next episode »).

## 5. Gestion des erreurs

| Situation | Comportement |
|---|---|
| Pas de chapitres / chapitres illisibles | `skip_segments` vide, appel AniSkip |
| Lecture des chapitres lente (pièces manquantes) | abandon après 1 s (AniSkip prend le relais), comme ci-dessus |
| AniSkip en panne, timeout, JSON invalide | 200 avec liste vide, rien en cache, pas de bouton |
| AniSkip connaît un autre encodage | résultat rejeté (écart de durée > 3 s) |
| Saison sans `idMal` | liste vide, pas d'appel externe |

## 6. Tests

Backend (`go test ./internal/metadata/ ./internal/api/`) :

- `matroska_chapters_test.go` : constructeur EBML de test produisant de vrais MKV synthétiques. Cas : chapitres
  dans l'en-tête ; chapitres en fin de fichier atteints via la SeekHead ; aucun `Chapters` ; chapitre caché ;
  chapitre désactivé ; deux éditions dont une par défaut ; édition ordonnée ignorée ; `ChapterSegmentUID`
  ignoré ; titre `eng` préféré à `jpn` ; `End` manquant ; élément tronqué → erreur ; élément `Chapters` trop
  gros → erreur ; arrêt au premier `Cluster` ; entrée non Matroska → pas de chapitres ; contexte annulé.
- `skip_segments_test.go` : table de dispositions réalistes — titres OP/ED explicites (anglais, français,
  japonais, `OP1`) ; « Episode » ne déclenche pas « ED » ; chapitres génériques avec cold open puis OP de 90 s ;
  OP à 0 s ; ED suivi d'un aperçu de 15 s ; pas d'ED ; seulement « Part A / Part B » ; chapitre de 90 s au
  milieu de l'épisode ignoré ; film de 2 h ; fichier de 3 min ; chapitre couvrant tout le fichier ; titre
  ambigu ; segment de titre trop court / trop long ; opening après l'ending ; `End` manquant du dernier
  chapitre ; durée nulle.
- `aniskip_test.go` (serveur `httptest`) : trouvé ; 404 `found:false` ; écart de durée rejeté ; intervalle
  invalide rejeté ; premier segment valide par type ; timeout ; 500 ; JSON invalide ; cache des succès et des
  « rien trouvé », non-cache des erreurs ; paramètres invalides sans appel ; URL et paramètres de requête
  exacts.
- Probe : test que `ProbeReader` (ou une fonction d'aide extraite et testable sans ffprobe) attache chapitres
  et segments et rembobine le lecteur.
- `skip_handlers_test.go` : paramètres invalides → 400 ; saison sans MalID → liste vide ; succès ; échec
  AniSkip → 200 liste vide ; en-têtes de cache.

Frontend :

- `web/scripts/skip-segments.test.mjs` (`node --test`, import direct du module `.ts` comme
  `episode-file.test.mjs`) : fusion par type, priorité aux chapitres, tri, bornes de `activeSkipSegment`
  (début inclus, dernière seconde exclue), `needsAniSkip`. Script npm `test:skip`.
- `npx tsc --noEmit` et `npm run lint` propres.

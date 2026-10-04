# Bibliothèque d'épisodes AV1 — design

Date : 2026-10-04

## Objectif

Ne plus solliciter les torrents à chaque visionnage. Le premier visionnage d'un épisode le télécharge en entier ; il est ensuite réencodé en AV1 et conservé sur les disques du serveur. Les visionnages suivants lisent le fichier local : démarrage immédiat, indépendant des seeders.

### Critères de réussite

- Un épisode déjà vu se lance depuis le disque local, sans requête aux indexeurs ni au swarm.
- Après réencodage, un épisode occupe environ 25–35 % de sa taille d'origine (benchmark : −67 à −83 % sur le flux vidéo, SSIM ≥ 0,99).
- Le cache n'empêche jamais une lecture : en cas de doute (copie absente, illisible, codec non supporté, langue différente), le lecteur repasse par le torrent comme aujourd'hui.
- Un disque étiqueté `GAZES*` branché sur le serveur est utilisé sans intervention.
- Le disque système garde toujours au moins sa réserve libre.

### Hors périmètre

- Stockage froid distant (Drive, SMB, rclone) : écarté.
- Compression générique (zip, zstd, xz) : gain mesuré < 0,1 % sur la vidéo, inutile.
- Encodage matériel (iGPU Intel, NVENC) : l'image actuelle n'a pas les pilotes ; évolution possible.
- Pré-téléchargement d'épisodes non regardés.

## Décisions et mesures

### Benchmark (2026-10-04, hôte de prod)

i7-13620H, encodage limité à 8 threads, `nice 19`, prod en marche. Deux épisodes 1080p H.264 WEB-DL (919 Mo et 770 Mo), extraits de 60 s, vidéo seule.

| Méthode | Vitesse | Épisode 24 min | Gain | SSIM |
|---|---|---|---|---|
| gzip / zstd / xz (fichier entier) | — | — | 0,02–0,04 % | — |
| SVT-AV1 preset 8, CRF 30, 10 bits | 4× temps réel | ~6 min | 67–76 % | 0,994–0,995 |
| SVT-AV1 preset 6, CRF 30 | 2× | ~11 min | 69–79 % | 0,995 |
| x265 medium, CRF 23 | 2× | ~12 min | 75–83 % | 0,993–0,994 |

Choix : **SVT-AV1 preset 8, CRF 30, 10 bits** (meilleur rapport vitesse/gain, ~80 épisodes par nuit de 8 h). Aucun épisode source HEVC 10 bits n'a été mesuré.

### Écart assumé avec AGENTS.md

L'invariant « remux plutôt que transcode » vise la lecture en direct. Ici, le réencodage est un traitement de fond, hors chemin de lecture, explicitement demandé pour économiser l'espace disque. La lecture reste un remux (`-c:v copy`) du fichier AV1. L'invariant « buffers éphémères » prévoit l'exception d'un cache persistant explicite : c'est ce module.

### Compatibilité navigateur

AV1 dans fMP4/MSE : Chrome, Edge, Firefox ; Safari seulement sur matériel Apple avec décodage AV1 (M3, A17 Pro et plus récents). Un client qui ne décode pas l'AV1 utilise le torrent.

## Architecture

Nouveau package `internal/library`, intégré au serveur (approche « backend intégré »). Composants :

| Composant | Rôle |
|---|---|
| `Store` | Index SQLite `library.sqlite` : une ligne par copie d'épisode, transitions d'état atomiques. |
| `Pool` | Découverte des disques (dossiers avec repère `.gazes-library`), espace libre, réservations, choix du disque. |
| `Acquirer` | Télécharge un fichier d'épisode en entier via le moteur torrent, le vérifie et le copie dans le pool. |
| `Encoder` | Worker de fond : réencode `ORIGINAL` → `AV1`, vérifie, supprime l'original. |
| `Janitor` | Libère de la place (épisodes vus le moins récemment d'abord), réconcilie l'index avec les disques. |
| `Source` | Interface commune « octets d'un fichier vidéo » : implémentée par le lecteur torrent existant et par un fichier local. |
| API HTTP | Enregistrement et consultation des copies, lecture. |
| `cmd/library` | Commande d'administration `gazes-library` (état, liste, suppression). |

### Identité

Une copie est identifiée par `(season_id, episode, lang)` :

- `season_id` : id AniList du média de la saison (même clé que les sources et les aperçus) ;
- `episode` : numéro d'épisode dans la saison ;
- `lang` : `vostfr` ou `vf`, déduit de la source choisie (`language_tag` / `is_french`).

Une seule copie par clé. Deux lectures simultanées du même épisode ne déclenchent qu'un téléchargement.

### Cycle de vie

```
            lecture lancée              fichier complet,
 (rien) ──────────────▶ DOWNLOADING ── vérifié, copié ──▶ ORIGINAL ──▶ ENCODING ── vérifié ──▶ AV1
                            │ bloqué 24 h / échec              ▲             │ échec                │
                            ▼                                  └─────────────┘ (max 3 essais)       │ place insuffisante :
                          (rien)                                                                    ▼ vus le moins récemment d'abord
                                                                                                  (rien)
```

États supplémentaires : `UNAVAILABLE` (disque absent ; redevient l'état précédent quand le disque revient), `ORIGINAL` avec `encode_skipped = not_smaller` (l'AV1 n'était pas plus petit, l'original est conservé définitivement) ou `encode_failed` (3 échecs).

Reprise au démarrage : `ENCODING` → `ORIGINAL` (le `.tmp` est supprimé), `DOWNLOADING` → relance du téléchargement complet si le torrent est encore connu, sinon suppression de l'entrée.

### Schéma `library.sqlite`

Table `episodes` :

| Colonne | Contenu |
|---|---|
| `season_id`, `episode`, `lang` | clé primaire |
| `anime_id`, `title` | affichage admin |
| `state` | `DOWNLOADING`, `ORIGINAL`, `ENCODING`, `AV1`, `UNAVAILABLE` |
| `prev_state` | état à restaurer quand un disque revient |
| `info_hash`, `file_index`, `release_name` | source d'origine |
| `disk_id`, `rel_path` | emplacement (`<season_id>/<episode>-<lang>.mkv`) |
| `size_bytes`, `original_size_bytes`, `sha256` | intégrité et statistiques |
| `duration_ms`, `video_codec`, `audio_tracks`, `subtitle_tracks` | résultats ffprobe |
| `reserved_bytes` | réservation en cours |
| `attempts`, `last_error`, `encode_skipped` | suivi des échecs |
| `created_at`, `updated_at`, `last_access_at` | dates |

L'index vit sur le volume `gazes_library-index` (`/app/library-index`), WAL activé, comme `accounts.sqlite`.

### Groupe de disques (`Pool`)

- Racine : `LIBRARY_POOL_DIR` (`/app/library-pool` dans le conteneur, bind de `/mnt/gazes` sur l'hôte avec `bind.propagation: rshared`, pour que les montages faits après le démarrage soient visibles).
- Un disque = un sous-dossier direct contenant `.gazes-library`. Le repère contient un `disk_id` stable (UUID généré à la création) : un disque déplacé ou remonté sous un autre nom garde ses épisodes.
- Rescan toutes les 60 s : nouveaux disques ajoutés, disques disparus → leurs entrées passent `UNAVAILABLE`, disques revenus → état précédent restauré.
- Réserve par disque : `max(LIBRARY_RESERVE_PERCENT × capacité, LIBRARY_RESERVE_BYTES)`, par défaut `max(10 %, 50 Go)`.
- Placement : le disque avec le plus d'espace « libre − réserve − réservations en cours ». Refus si aucun disque n'a la place requise après tentative de libération.
- Réservations : téléchargement = taille du fichier ; encodage = 0,5 × taille d'origine en plus (original et AV1 coexistent). Libérées à la fin de l'étape.

### Mise en cache (`Acquirer`)

1. Le lecteur, une fois le fichier d'épisode connu (torrent chargé, `selectedFileIdx` choisi), appelle `POST /api/v1/library/episodes/{season}/{ep}/{lang}` avec `info_hash`, `file_index`, `release_name`, `anime_id`, `title`.
2. Si une copie existe déjà (hors `UNAVAILABLE`), la requête met seulement à jour `last_access_at`.
3. Sinon : réservation de place, entrée `DOWNLOADING`, puis le moteur torrent passe toutes les pièces du fichier en priorité normale (sans toucher aux autres fichiers d'un pack). Le torrent est épinglé : l'éviction LRU existante (`internal/torrent/evict.go`) l'ignore tant qu'il est épinglé.
4. Fichier complet : vérification des pièces (hash), copie vers `<disque>/<rel_path>.tmp`, `fsync`, renommage, ffprobe, entrée `ORIGINAL`, désépinglage.
5. Abandon si aucune progression pendant `LIBRARY_STALL_TIMEOUT` (24 h) : entrée supprimée, réservation libérée.

Le moteur torrent expose deux opérations nouvelles : `DownloadFile(infoHash, fileIndex)` (priorité normale sur toutes les pièces du fichier, survit à la fin de lecture) et `Pin/Unpin(infoHash)` (exclusion de l'éviction).

### Lecture (`Source`)

- `GET /api/v1/library/episodes/{season}/{ep}` renvoie les copies prêtes : `[{lang, state, video_codec, duration_ms, audio_tracks, subtitle_tracks, stream_id}]`. `state` ∈ `ORIGINAL`, `AV1` uniquement.
- Le lecteur, avant la recherche de sources :
  1. choisit la copie de la langue préférée de l'utilisateur ;
  2. si `video_codec = av1`, vérifie `MediaSource.isTypeSupported('video/mp4; codecs="av01.0.08M.10"')` ;
  3. si une copie convient, lit via `stream_id` ; sinon, flux torrent habituel (recherche de sources inchangée).
- Côté serveur, les routes de lecture existantes (`/stream`, `/stream/raw`, `/subtitles`, `/metadata`, sessions HLS) acceptent un `library=<stream_id>` à la place de `ih` + `file_idx`. Elles obtiennent un `Source` : fichier local (`os.File`, `io.ReadSeeker`, Range natif) ou lecteur torrent. Le reste du pipeline (remux fMP4, HLS, piste audio, sous-titres) est inchangé.
- Le tag fMP4 de la vidéo AV1 est `av01` ; ffmpeg l'écrit nativement en `-c:v copy`.
- Chaque ouverture met à jour `last_access_at` et incrémente un compteur de lectures actives (utilisé par l'encodeur et le janitor).

### Encodage (`Encoder`)

- Un seul encodage à la fois ; file ordonnée par `updated_at` croissant des entrées `ORIGINAL` (hors `encode_skipped`).
- Commande (valeurs configurables) :

```
nice -n 19 ffmpeg -nostdin -y -i <original> -map 0 \
  -c:v libsvtav1 -preset 8 -crf 30 -pix_fmt yuv420p10le -svtav1-params lp=8 \
  -c:a <copy si AAC/Opus, sinon libopus 128k stéréo / 256k 5.1> \
  -c:s copy -c:t copy -map_chapters 0 -map_metadata 0 \
  <disque>/<rel_path>.av1.tmp.mkv
```

- Plage horaire : `LIBRARY_ENCODE_WINDOW` (`HH:MM-HH:MM`, vide = en continu). Hors plage, l'encodage en cours est suspendu (`SIGSTOP`) puis repris (`SIGCONT`) à la plage suivante.
- Pause quand le serveur est chargé : si les lectures actives ≥ `LIBRARY_ENCODE_PAUSE_STREAMS` (3), `SIGSTOP` ; reprise quand elles redescendent.
- Vérification avant validation :
  - durée à ±1 s de l'original ;
  - même nombre de pistes audio et de sous-titres ;
  - décodage réussi de 2 s au début, au milieu et à la fin (`ffmpeg -ss … -t 2 -f null -`) ;
  - taille > 0.
- Si l'AV1 n'est pas plus petit que l'original : `.tmp` supprimé, original gardé, `encode_skipped = not_smaller`.
- Succès : renommage atomique en `<rel_path>` (extension `.mkv`), original supprimé, `state = AV1`, statistiques mises à jour.
- Échec : `.tmp` supprimé, `attempts++`, `last_error` ; après 3 échecs `encode_skipped = encode_failed`. L'original n'est jamais supprimé avant une vérification réussie.
- Arrêt du serveur : l'ffmpeg en cours est tué, son `.tmp` supprimé au démarrage suivant.

### Libération de place (`Janitor`)

- Toutes les 5 minutes et avant chaque réservation refusée.
- Pour chaque disque sous sa réserve : supprimer les copies `ORIGINAL`/`AV1` de ce disque par `last_access_at` croissant jusqu'à repasser au-dessus de la réserve.
- Jamais supprimées : copies en lecture, en encodage, ou dont `last_access_at` date de moins de 1 h.
- Réconciliation au démarrage et toutes les heures : fichiers présents sur disque sans entrée → supprimés ; entrées dont le fichier manque sur un disque présent → supprimées ; `.tmp` orphelins → supprimés.

### Sécurité

Les routes `/api/*` du backend sont publiques via le frontend. Pour qu'un tiers ne puisse pas remplir les disques avec des torrents arbitraires :

- `POST /api/v1/library/episodes/...` exige un utilisateur connecté (session du module `internal/auth` existant) ; sinon 401, et la lecture continue par torrent sans mise en cache.
- Le couple `info_hash` + `file_index` doit correspondre à un torrent déjà chargé par le moteur et à un fichier vidéo de ce torrent ; sinon 422.
- `season_id` doit exister dans le catalogue, `episode` doit être dans les bornes de la saison, `lang` ∈ {`vostfr`, `vf`} ; sinon 422.
- Limite : au plus 20 nouvelles copies `DOWNLOADING` par utilisateur et par heure, et au plus 4 téléchargements complets simultanés au total ; au-delà, 429 (la lecture continue par torrent).
- `stream_id` est un identifiant opaque (HMAC de la clé avec un secret serveur) : les routes de lecture ne lisent que des fichiers référencés par l'index, jamais un chemin fourni par le client.

### Erreurs

| Situation | Comportement |
|---|---|
| Fichier local absent ou illisible à la lecture | Entrée supprimée, réponse 404 `library_miss` ; le lecteur repasse au torrent. |
| Disque débranché | Entrées `UNAVAILABLE`, absentes de `GET /library/...` ; lecture par torrent. |
| Swarm mort pendant le téléchargement complet | Abandon après 24 h sans progression. |
| Place insuffisante après libération | Pas de mise en cache ; la lecture torrent continue normalement ; événement de diagnostic. |
| ffmpeg plante ou vérification échoue | Original conservé, réessai (max 3). |
| Index SQLite corrompu ou absent | Module désactivé au démarrage avec une erreur claire ; la lecture torrent fonctionne. |

Chaque transition émet un événement `diagnostics.Log` (`library.state`, `library.encode`, `library.evict`, `library.disk`) avec la clé d'épisode, le disque et la durée.

### Administration

Le projet n'a ni rôle ni interface d'administration, et le frontend relaie toutes les routes `/api/*` du backend vers l'extérieur : l'administration ne passe donc pas par HTTP. Une commande `gazes-library` (nouveau `cmd/library`, incluse dans l'image comme `gazes-server`), lancée avec `docker compose exec backend gazes-library <commande>`, lit et modifie `library.sqlite` et le pool :

- `status` : nombre de copies par état ; par disque, étiquette, `disk_id`, capacité, libre, réserve, utilisé par la bibliothèque, présent ou non ; file d'encodage (en cours avec progression, en attente) ; dernières erreurs ; espace gagné par l'AV1 (`Σ original_size_bytes − Σ size_bytes` des copies `AV1`) ;
- `list [--state S] [--season ID]` : copies avec leur état, disque, taille et dernier accès ;
- `delete <season> <ep> <lang>` : supprime une copie.

Les écritures concurrentes avec le serveur passent par SQLite (WAL, transactions courtes) ; la progression de l'encodage en cours est lue dans une ligne `encoder_status` mise à jour par le serveur toutes les 10 s.

## Configuration

| Variable | Défaut | Rôle |
|---|---|---|
| `LIBRARY_ENABLED` | `true` | Active le module. |
| `LIBRARY_POOL_DIR` | `/app/library-pool` | Racine du groupe de disques. |
| `LIBRARY_INDEX_DIR` | `/app/library-index` | Emplacement de `library.sqlite`. |
| `LIBRARY_RESERVE_PERCENT` | `10` | Réserve libre par disque (%). |
| `LIBRARY_RESERVE_BYTES` | `50GB` | Réserve libre minimale par disque. |
| `LIBRARY_STALL_TIMEOUT` | `24h` | Abandon d'un téléchargement sans progression. |
| `LIBRARY_ENCODE_PRESET` | `8` | Preset SVT-AV1. |
| `LIBRARY_ENCODE_CRF` | `30` | CRF SVT-AV1. |
| `LIBRARY_ENCODE_THREADS` | `8` | Threads de l'encodeur. |
| `LIBRARY_ENCODE_WINDOW` | vide | Plage horaire d'encodage. |
| `LIBRARY_ENCODE_PAUSE_STREAMS` | `3` | Lectures actives qui suspendent l'encodage. |

## Déploiement

- `compose.yaml`, service `backend` :
  - bind `/mnt/gazes:/app/library-pool` avec `bind.propagation: rshared` et `create_host_path: true` ;
  - volume nommé `library-index:/app/library-index`.
- L'utilisateur du conteneur (uid 10001) doit pouvoir écrire dans chaque disque : le montage et le script hôte appliquent `uid=10001,gid=10001` (exFAT/NTFS) ou un `chown` du dossier racine (ext4).
- Disque principal : le script hôte crée `/mnt/gazes/internal` avec son repère.
- `deploy/library/install-host.sh` (une fois, `sudo`) :
  - règle udev `99-gazes-library.rules` : un bloc dont `ID_FS_LABEL` commence par `GAZES` déclenche l'unité `gazes-library-mount@<label>.service` ;
  - l'unité monte la partition sur `/mnt/gazes/<label>` (options `nofail`, `noatime`, uid/gid selon le système de fichiers), crée le repère s'il manque, et démonte proprement à la déconnexion ;
  - `/mnt/gazes` est rendu `--make-rshared` au démarrage (unité `gazes-library-shared.service`).
- `make library-label-disk DEV=/dev/sdX1 LABEL=GAZES-1` : pose l'étiquette sans formater ; propose le formatage ext4 seulement avec confirmation explicite.
- L'ffmpeg 8.0.1 de l'image contient déjà `libsvtav1` (vérifié) : aucun changement d'image.

## Tests

- **Unitaires (`internal/library`)**
  - transitions d'état, unicité de la clé, reprise après redémarrage ;
  - `Pool` : choix du disque le plus libre, réserve, réservations concurrentes, disque absent puis revenu (même `disk_id`) ;
  - `Janitor` : ordre de suppression, copies protégées (lecture, encodage, < 1 h) ;
  - vérification d'encodage : durée, pistes, cas « pas plus petit » ;
  - plage horaire et pause/reprise (horloge et signal injectés).
- **Intégration**
  - encodage AV1 réel d'une vidéo de test générée (5 s, 2 pistes audio dont une AC3, sous-titres ASS avec police jointe) ; vérification des pistes et de la lecture ;
  - routes `/stream` et HLS avec `library=` : `206 Partial Content`, `Content-Range`, `Accept-Ranges: bytes` ;
  - `Acquirer` avec un faux moteur torrent : téléchargement complet, épinglage, copie, abandon sur blocage ;
  - pool simulé : dossiers temporaires avec ou sans repère, ajout et retrait pendant l'exécution.
- **Frontend (`web/scripts/*.test.mjs`)**
  - copie AV1 disponible et supportée → lecture locale ;
  - AV1 non supporté, langue différente ou `library_miss` → flux torrent ;
  - enregistrement `POST /library/...` après choix du fichier.

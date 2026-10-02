# Diagnostic VF et filtrage — Tensura, épisode 1

Cas étudié le 2 octobre 2026 :
`http://192.168.1.46:8081/anime/101280/seasons/101280/episodes/1?debug=true`.

Les corrections sont chargées sur le backend **de développement** de cette instance.

## Contre-exemples trouvés après élargissement externe

Une [annonce EXT de la saison 1, pack FoX VFF](https://ext.to/that-time-i-got-reincarnated-as-a-slime-s01-1080p-bluray-multi-vff-pcm-ac3-x264-fox-20134617/)
contient bien un fichier S01E01. Elle constitue une annonce de VF hors du
périmètre de la capture Nyaa ; ses pistes n'ont pas été sondées par Gazes.
Le compteur de la page indexée indique zéro seeder et reste une observation
ancienne, sans preuve de disponibilité actuelle.

La [liste EXT de la saison 1](https://ext.to/browse/?q=That+Time+I+Got+Reincarnated+as+a+Slime+S01)
annonce aussi un pack SHiNiGAMi comprenant la saison et cinq OAV, marqué MULTI.
Sa langue française reste à confirmer. Son intitulé `S01 + 5 OAV` était rejeté
par le filtre avant correction ; ce cas réel est désormais testé avec le pack
FoX. Les OAV seuls, les autres saisons et les mauvais épisodes restent rejetés.

Sur l'instance de développement, seuls AniDex et The Pirate Bay sont configurés
dans Prowlarr, en plus du connecteur Nyaa direct. EXT n'est pas configuré et sa
définition n'est pas proposée par les schémas de cette installation. L'accès
direct à sa page a renvoyé HTTP 403 le 2 octobre 2026. Aucun magnet ni infohash
de ces deux annonces n'a été récupéré : elles ne sont donc pas injectées comme
sources lisibles, ni incluses dans le bilan des sondes audio.

Le provisionnement exporte maintenant aussi les autres indexeurs torrent déjà
activés dans Prowlarr. Gazes charge tous ces gateways privés, y compris EXT s'il
est explicitement configuré, avec priorité conservée aux désactivations manuelles.
La version du cache de découverte passe à v3 pour ne pas conserver les rejets
de packs avec bonus numérotés. Ces corrections ne rendent pas EXT accessible.
Après rechargement des deux gateways existants et redémarrage du backend de
développement, le service est sain et l'API de cet épisode répond HTTP 200 :
16 candidats, réponse partielle, aucune annonce VF. Ce contrôle valide le
fonctionnement du service ; il ne valide pas la récupération des packs EXT.

## Résultats mesurés

| Recherche | Sources candidates | VF annoncées | État | Durée |
| --- | ---: | ---: | --- | ---: |
| Ancienne recherche complète | 26 | 0 | partielle | 20,122 s |
| Nouvelle première réponse | 8 | 0 | partielle | 3,035 s |
| Nouvelle recherche complète | 37 | 0 | partielle | 20,075 s |

Les quatre sources comptées comme françaises par la recherche complète sont
des **VOSTFR**, pas des VF. Les tags MULTI et Dual Audio restent non confirmés.
Les captures ne prouvent pas que la VF est absente de tous les fournisseurs.
Nyaa et AniDex ont notamment renvoyé des HTTP 429 pendant la campagne.

La vérification audio distingue : `confirmed` (piste française), `absent`
(sonde complète du bon épisode, sans piste française), `unknown` (langue
indéterminée), `inaccessible` (sonde indisponible), et `episode_unverified`
(fichier absent ou ambigu). Un timeout ne prouve jamais une absence de VF.
Les résultats détaillés et le bilan des sources distinctes sont dans
[track-summary.json](track-summary.json), [tracks.json](tracks.json),
[verified-files/tracks.json](verified-files/tracks.json) et
[verified-trix/tracks.json](verified-trix/tracks.json).
Les premiers essais précèdent la correction de sélection des fichiers ; les
essais `verified-files` montrent ensuite la sélection correcte de LostYears
et EMBER. Une sonde ancienne complète reste une preuve pour le même infohash
et le même fichier, même si une nouvelle tentative expire.

## Problèmes corrigés

- Le RSS Nyaa ignore le tri par seeders et répète la première page. La recherche
  de sources utilise maintenant la liste HTML triée et paginée. Les magnets,
  compteurs et URL directes de métadonnées sont lus dans la même réponse.
- Les recherches VF couvrent toutes les catégories anime. Les titres alternatifs
  et les recherches sans tag de langue restent présents. Le plan de cet anime
  passe de **162 à 34 requêtes**, avant pagination ; la première phase en prévoit
  huit avec un budget de trois secondes. Les délais globaux et les protections
  de cache restent bornés.
- Les packs `S01+S02` et `Season One + OADs` sont reconnus. Les sorties sans
  numéro d'épisode restent des candidats dont le fichier doit être vérifié.
  Les incompatibilités explicites de série, saison ou épisode restent rejetées.
- Les OVA/OAD au pluriel restent des extras. `French Subs` ne devient plus VF ;
  VFF, VFQ et VF2 restent des annonces de doublage, à vérifier par les pistes.
- `10-bit` ne crée plus un faux numéro d'épisode. Les dossiers préfixés comme
  `02. OVA`, les tags `S01OAD03` et les épisodes décimaux ne deviennent plus
  des épisodes TV ordinaires.
- Les clés des caches de découverte sont versionnées pour éviter de réutiliser
  les résultats de l'ancien RSS et des anciens filtres.

## Outils réutilisables

Depuis la racine du dépôt, afficher le plan sans accès réseau :

```sh
go run ./cmd/source-audit -identity docs/tensura-filter-audit/identity.json -plan
```

Rejouer **tous** les candidats capturés, y compris les torrents rejetés :

```sh
go run ./cmd/source-audit -replay internal/indexer/testdata/tensura-nyaa.json -out /tmp/tensura-replay.json
```

Capturer une recherche avec les mêmes fournisseurs, le cache Redis et les
délais de l'instance de développement :

```sh
docker exec -w /src gazes-dev-backend-1 go run ./cmd/source-audit -identity /src/docs/tensura-filter-audit/identity.json -out /tmp/tensura-capture.json
docker cp gazes-dev-backend-1:/tmp/tensura-capture.json /tmp/tensura-capture.json
```

La capture conserve l'identité, les candidats bruts, les requêtes, leurs durées,
les erreurs par fournisseur et chaque décision du filtre. `-fast` teste la
première phase ; `-nyaa-only` isole Nyaa. Hors du conteneur, renseigner
`REDIS_URL` pour partager le cache ; sans cette variable, la capture utilise
un cache local au processus. Privilégier le rejeu lors du réglage des filtres.

Vérifier les pistes audio du bon épisode, sans lancer de lecture :

```sh
node --experimental-strip-types web/scripts/source-audit.mjs \
  --url 'http://192.168.1.46:8081/anime/101280/seasons/101280/episodes/1' \
  --capture docs/tensura-filter-audit/final-full.json \
  --out /tmp/tensura-tracks --limit 8 --source-ms 25000
```

Sans `--capture`, l'outil interroge l'API de sources. Il sauvegarde également
les listes de fichiers pour rejouer leur sélection. Les essais sont séquentiels,
bornés et limités ; ils récupèrent les métadonnées et les morceaux nécessaires
à ffprobe, sans téléchargement volontaire des vidéos entières.

## Validation et performances

Suite Go complète réussie avec `-tags=nosqlite`, tests de concurrence des
indexeurs/API réussis, TypeScript réussi, tests des preuves VF et du classement
réussis. Les 22 tests de fichiers comprennent chacun des 24 épisodes de Tensura
sur deux packs réels et les 220 épisodes TV Naruto sur leur pack réel.
La lecture vidéo complète et tous les fournisseurs n'ont pas été validés.

Le corpus de régression réunit 531 candidats distincts des captures Nyaa et
de l'ancienne réponse : 33 restent candidats après rejeu. Ce nombre diffère de
la nouvelle recherche live, qui peut découvrir d'autres torrents.

[benchmark.txt](benchmark.txt) compare la préparation des règles par candidat
à leur préparation une seule fois : environ **552 ms contre 15 ms** pour le
corpus, avec **133 Mo contre 1,3 Mo** d'allocations cumulées. Cette mesure porte
sur le filtre ; elle ne mesure ni les accès réseau ni le temps de démarrage vidéo.

Captures finales : [final-fast.json](final-fast.json),
[final-full.json](final-full.json), [plan.json](plan.json),
[replay.json](replay.json) et [final-capture.json](final-capture.json).
La dernière capture brute, effectuée avec le cache partagé, compte 43 appels
avec pagination, 878 candidats distincts et 51 candidats retenus, sans annonce
de VF pour la saison 1. Elle reste partielle : certaines réponses combinent
des résultats utilisables avec des erreurs d'autres fournisseurs.
Les autres captures documentent les étapes de
diagnostic et ne constituent pas une référence de découverte exhaustive.

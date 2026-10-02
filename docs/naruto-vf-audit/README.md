# Naruto saison 1 : validation VF

Dernière campagne : 2026-10-02T18:44:56.832Z → 2026-10-02T18:54:50.135Z. **5/5 VF confirmées** sur le bon fichier.

| Épisode | Source confirmée | Fichier / index | Durée vidéo | Piste VF | Temps total de diagnostic |
| ---: | --- | --- | ---: | --- | ---: |
| 1 | DVDRIP | `Naruto 001.mkv` / 24 | 22:50 | `fre`, audio 0, mp3 | 83.9 s |
| 5 | Judas | `[Judas] Naruto S1/[Judas] Naruto - S01E05 (005).mkv` / 4 | 22:52 | `fre`, audio 2, opus | 139.6 s |
| 10 | Judas | `[Judas] Naruto S1/[Judas] Naruto - S01E10 (010).mkv` / 9 | 22:55 | `fre`, audio 2, opus | 133.9 s |
| 15 | Judas | `[Judas] Naruto S1/[Judas] Naruto - S01E15 (015).mkv` / 14 | 22:56 | `fre`, audio 2, opus | 117.4 s |
| 20 | Judas | `[Judas] Naruto S1/[Judas] Naruto - S01E20 (020).mkv` / 19 | 22:55 | `fre`, audio 2, opus | 118.6 s |

## Preuves et limites

[Rapport final : métadonnées, pistes, codecs, fichiers, scores, seeders, délais, erreurs et traces](report.json). Les captures des listes de sources sont dans [la troisième campagne](campaign-3/README.md). Les [première](campaign-1/README.md) et [deuxième](campaign-2/README.md) campagnes ont également confirmé 5/5.

Le diagnostic essaie VF annoncée puis VOSTFR puis langue non confirmée. Une annonce MULTI/VOSTFR peut réussir si le fichier contient effectivement une piste française. Un titre VF et des sous-titres français ne constituent pas une preuve audio. Les codes fr/fra/fre confirment la VF ; un libellé French/Français/VF n’est utilisé que si le code est absent ou indéterminé. Un code contradictoire reste indéterminé.

Les budgets sont 90 s par source, 30 s sans progression et 5 min par épisode, avec arrêt à la première confirmation. Trois campagnes successives ont été exécutées. Ce sont des sondes de métadonnées, sans écoute ni visionnage intégral.

Le pack Anime Time sondé contient anglais/japonais et aucune piste française. Le pack Judas contient jpn/eng/fre ; la VF est audio 2, après la piste japonaise. Le DVDRIP contient également une VF, mais ses pièces restent irrégulièrement accessibles : quatre essais de la dernière campagne atteignent 90 s avant un fallback. Ces timeouts restent « inaccessible », et ne prouvent pas une absence de VF. Les temps du tableau incluent la recherche et tous les échecs du diagnostic ; ils ne mesurent pas le démarrage du lecteur.

## Corrections fondées sur les tentatives

- Exclusion des openings/endings numérotés et des remontages/spinoffs ; contrôle de la série et de la numérotation. Le fichier 0 `Opening & Ending/Naruto - EndinG 1.mkv` est rejeté ; l’épisode 1 réel est le fichier 24 `Naruto 001.mkv`.
- Normalisation des accents latins pour exclure aussi Shippûden et Boruto Générations, en préservant les marques japonaises.
- Recherche partagée des packs de saison avant les requêtes d’épisode, classement par langue VF/VOSTFR puis pack et qualité. Découverte complète avec `discovery=full`, cache distinct et reprise automatique après un échec de la liste partielle.
- Correction du lien Nyaa `/download/ID.torrent` pour fournir la source HTTP de métadonnées au magnet ; maintien des résultats disponibles quand un autre indexeur dépasse son délai.
- Statuts `complete`, `timeout`, `failed` de sonde. Lecture FFprobe bornée à 64 Ko puis, si nécessaire et possible, 1 Mo dans le budget global de 4 s ; fermeture et annulation conservées. Les seuls 64 premiers Ko du fichier DVDRIP ont fourni les pistes fre/jpn lors d’un contrôle.
- Sélection de la piste française sur le fichier choisi lorsque ses métadonnées arrivent tard ; conservation des changements manuels. Préparation des deux prochaines sources en parallèle et bascule du lecteur après 15 s sans progrès au chargement/démarrage.
- La vidéo reste copiée dans le pipeline de remux ; aucune ré-encodage vidéo ajouté.

## Reproduction et tests

Depuis `web/` : `npm run diagnostic:naruto-vf`. Variables : `DIAGNOSTIC_API_BASE` et `DIAGNOSTIC_OUTPUT`. Node doit prendre en charge `--experimental-strip-types`.

Validés : suite Go complète, détecteur de courses API/indexeurs/métadonnées, 21 tests de sélection de fichiers (dont les 220 épisodes du pack réel), 5 tests de classement/fallback, 4 tests de preuve VF (dont cinq captures réelles), TypeScript sans émission, Chromium : bascule automatique, annulation, reprise, sélection manuelle, fullscreen, sélection tardive de la VF et découverte complète après échec.

Le backend de développement a été rechargé avant la troisième campagne avec les corrections finales. Les résultats restent dépendants des swarms disponibles ; 5/5 métadonnées confirmées ne garantissent pas une lecture fluide à tout instant.

## Correction du fallback constaté sur l’épisode 1

Les [événements réels du lecteur](episode-1-fallback/browser-before-fix.json) montrent une VF DVDRIP confirmée, suivie d’un `startup_timeout`, puis une bascule vers Anime Time anglais/japonais qui démarrait sans poursuivre la recherche VF. Le lecteur automatique continue désormais lorsqu’une sonde complète ne confirme pas de piste française (VF absente ou langue indéterminée). Il conserve ces candidats comme fallback après la découverte complète et respecte une source choisie manuellement. Un timeout de sonde ne devient pas une absence de VF.

Une nouvelle sonde de Judas épisode 1 confirme la piste `fre` audio 2 dans le fichier 201, durée 22:55. Le test Chromium reproduit source VF inaccessible → source anglais/japonais → pack MULTI avec français ; il vérifie aussi le fallback sans VF disponible, sans boucle. Les captures sont dans `episode-1-fallback/`.

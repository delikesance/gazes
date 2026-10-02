# Audit des sources Naruto — 2 octobre 2026

Trace étudiée : `637c4810-2a9e-444b-adc9-60bc19a9eafe`.

## Pourquoi la lecture durait 1:32

Le pack [Naruto DVDRIP VostFr/Vf](https://nyaa.si/view/1250521), infohash
`e9046a56e25fa17557bd2c585d4a8a9e6d2091fc`, contient 244 fichiers :
24 génériques et 220 épisodes TV. Les métadonnées `.torrent` ont été lues
directement depuis Nyaa, sans télécharger les vidéos.

Avant correction, l'épisode 1 avait trois candidats :

- index 0 : `Opening & Ending/Naruto - EndinG 1.mkv` ;
- un opening numéroté 1 ;
- index 24 : `Naruto 001.mkv`, le vrai épisode.

Le filtre ne reconnaissait ni les noms complets Opening/Ending, ni le dossier
combiné. Le lecteur prenait le premier candidat en cas d'ambiguïté. Les logs
confirment une demande du fichier index 0 pour cette tentative. La durée de
1:32 et la piste japonaise signalées appartiennent donc au générique.

## Résultats Nyaa et classement

Les recherches publiques `Naruto VF`, `Naruto VOSTFR` et `Naruto DVDRIP`
ont chacune renvoyé 75 entrées RSS. Une recherche textuelle peut aussi
renvoyer des RAW marqués VFR, des séries publiées par Naruto-Kun.Hu,
des films, des séries dérivées et des versions remontées.

| Source observée | Seeders | Score initial | Analyse |
| --- | ---: | ---: | --- |
| Naruto Yabai Intégrale 1080p VO/VF | 45 | 249 | Faux candidat TV : 18 films remontés |
| Naruto DVDRIP VostFr/Vf | 2 | 213 | Pack TV valable ; swarm peu fourni |
| Anime Time Naruto 001–220, Dual Audio, Eng Sub | 543 | 65 | Français non confirmé ; HEVC à vérifier selon le navigateur |
| Judas Naruto Complete, Multi-Audio/Multi-Subs | 171 | 64 | MULTI seul ne prouve pas la présence du français |

La [description de Yabai](https://nyaa.si/view/1723946) explique qu'il s'agit
d'une relecture remontée de Naruto. Les métadonnées confirment 18 fichiers,
avec des tailles de plusieurs Go. Ses numéros ne sont pas ceux des épisodes
TV. Kai et FullEdit sont également exclus de la recherche TV standard,
ainsi que Naruto SD et les packs explicitement nommés Spin-Off.

Le classement conserve la priorité **VF > VOSTFR > français non confirmé**
pour les sources avec seeders. Bonus langue : VF 200, VOSTFR 100 ; swarm :
`min(60, round(8 × log2(1 + seeders)))` ; qualité : 0 à 4 ; MULTI : 2.
Les leechers ne rapportent aucun point. Les sources sans seeder viennent
après les sources avec seeders. Le navigateur privilégie aussi les codecs
qu'il sait décoder. Le titre fournit une indication de langue, pas une
confirmation des pistes présentes dans chaque fichier.

## Corrections et vérifications

- Exclusion des génériques complets et dossiers combinés OP/ED.
- Plus de sélection arbitraire du premier fichier ambigu ; les variantes
  distinctes par résolution restent sélectionnables automatiquement.
- Exclusion des remontages et séries dérivées explicitement marqués.
- Exclusion des packs de films numérotés, tout en gardant les packs
  `001–220 + Movies` dont la plage concerne les épisodes TV.
- Vérification de chacun des 220 épisodes sur les 244 fichiers réels :
  un seul candidat correct par épisode ; épisode 221 absent.

Appels à l'API locale existante, puis rejeu des sources capturées avec le
résolveur corrigé et l'identité TV Naruto :

| Épisode | Sources capturées | Retenues au rejeu | Première source après correction |
| ---: | ---: | ---: | --- |
| 1 | 36 | 25 | DVDRIP VF/VOSTFR |
| 2 | 34 | 26 | DVDRIP VF/VOSTFR |
| 14 | 33 | 27 | DVDRIP VF/VOSTFR |
| 220 | 29 | 25 | DVDRIP VF/VOSTFR |

Les quatre recherches live indiquaient `partial: true` : ces captures ne
constituent pas un inventaire exhaustif de Nyaa. Les tests utilisent des
captures fixes ; les nombres de seeders peuvent évoluer.

Validation : suite Go complète réussie, 20 tests de sélection des fichiers
réussis (dont la boucle des 220 épisodes), 3 tests de classement frontend
réussis et vérification TypeScript réussie. Le test Chromium de lecture
automatique a effectué les bascules de sources attendues, puis échoué sur
un clic de changement de source intercepté par un panneau ; ce test complet
reste donc en échec. La lecture vidéo des 220 épisodes n'a pas été testée.

Les corrections sont dans les sources ; le backend de développement a été
redémarré pour charger le résolveur corrigé.
Un nouveau chargement de l'épisode est nécessaire pour refaire la sélection
du fichier côté frontend. Aucun réencodage vidéo n'a été ajouté.

## Suivi : lenteur de démarrage

Les logs montraient des recherches de sources à environ 20,8 secondes.
Le chemin de lecture effectue maintenant une première phase de recherches
de packs de saison avec un budget de trois secondes. Un pack VF avec
seeders permet de démarrer ; sans VF disponible, la recherche complète
continue avec le classement habituel. Les résultats anticipés sont signalés
comme partiels. Les fournisseurs dépassant le délai ne font plus perdre
les résultats déjà reçus d'un fournisseur disponible.

Les liens RSS Nyaa `/download/ID.torrent` étaient mal analysés : leur ID
incluait `.torrent`, empêchant l'ajout de l'URL de métadonnées `xs` au magnet.
Le correctif permet de récupérer les métadonnées via HTTP, avec vérification
du hash par le client torrent, plutôt que d'attendre leur échange avec les peers.
Les deux sources suivantes sont préparées en parallèle après 500 ms.
La bascule d'une source sans progrès au démarrage passe de 40 à 15 secondes ;
la simple présence d'un seeder ne prolonge plus l'attente des métadonnées.

Mesures après redémarrage du backend corrigé : premier appel Naruto épisode 1
9,893 s, incluant le chargement initial du catalogue ; épisode 2 ensuite
3,378 s. Chargement de la liste des 244 fichiers : 26 ms (cache disque existant).
La source VF n'a fourni aucun premier octet vidéo pendant un test de 15 s ;
ses statistiques indiquaient un peer/seeder et un débit de téléchargement nul.
Ces mesures prouvent l'amélioration de la découverte, pas une lecture fluide
d'un swarm incapable de fournir les pièces nécessaires.

Suite Go complète, tests du détecteur de courses sur les indexeurs, tests
de classement frontend et TypeScript réussis.

## Validation audio des cinq épisodes

La validation suivante sonde les fichiers des épisodes 1, 5, 10, 15 et 20
et confirme la VF par leurs pistes audio. Les résultats, les trois campagnes
et les tests sont dans [le rapport VF](naruto-vf-audit/README.md).
La découverte complète possède un cache distinct et le lecteur la relance
après un échec des sources partielles. Les erreurs et timeouts de sonde
sont explicitement distingués de l’absence de piste française.

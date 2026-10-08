# Moteur de lecture : legacy ou hls

Un seul réglage choisit le moteur : `PLAYBACK_ENGINE` (`legacy` par défaut, `hls`). Toute autre valeur retombe sur `legacy`. Le backend l'expose via `GET /api/v1/playback/config`, le web s'y aligne (`?player=hls|legacy` force un moteur côté navigateur pour tester ; iOS/iPadOS utilise toujours hls).

| | legacy | hls |
|---|---|---|
| Principe | un remux FFmpeg par spectateur, flux fMP4 continu | segments produits une fois, partagés entre spectateurs du même épisode/piste audio |
| Limite | 32 remuxes simultanés, 8 par client (`internal/stream/remux.go`) | 4 FFmpeg de segment en parallèle (`workers`, `internal/playback/manager.go`), cache `PLAYBACK_MEMORY_BYTES` (64 Mo) et `PLAYBACK_DISK_BYTES` (1 Go) |
| Reprise / seek | relance un remux au décalage | index de segments, seek par génération |

## Écarts de parité connus (à vérifier avant de basculer la prod)

1. Pas de bascule automatique de source en cas d'échec : `VideoPlayerModal` désactive `onPlaybackFailure` (timeouts métadonnées/démarrage/blocage) en mode hls, l'erreur est seulement affichée.
2. Pas de sélection de fichier ni d'écran d'erreur legacy en hls (`needsFileSelection`, `error` ignorés quand `hlsMode`).
3. Fenêtre de segments étroite (−10 s / +30 s autour de la position) et 25 s max par segment : une source lente à fournir des pièces échoue plus tôt qu'avec le flux continu.
4. Débit global plafonné à 4 FFmpeg simultanés, partagé par tous les épisodes : à vérifier à charge réelle (nombre d'épisodes distincts regardés en même temps).
5. Métrique « sessions actives » de l'admin : mesurée par le manager en hls, estimée via `watch_sessions` en legacy ; les chiffres changent de définition au basculement.
6. Watch party : la synchro passe par le seek hls (`HlsPlaybackController`) ; à tester à plusieurs.

## Déploiement

1. Pré-prod (`dev`, :8082) : `PLAYBACK_ENGINE=hls` dans `.env`, `docker compose up -d backend`. Lire un épisode H.264, un AV1, un MKV multi-pistes ; seek, changement de piste audio, sous-titres, watch party à 2.
2. Surveiller `/admin` (erreurs de lecture, p95 de démarrage) et les runbooks `REMUX_FAILED.md`, `STREAM_TIMEOUT.md`.
3. Prod : même réglage dans `.env`, redémarrer le backend (pas de rebuild). Le défaut du code reste `legacy`.

## Retour arrière

Retirer `PLAYBACK_ENGINE` (ou `legacy`) de `.env` et redémarrer le backend ; le web relit `/playback/config` au chargement. Les sessions en cours se terminent sur l'erreur et reprennent en legacy au rechargement. Aucune donnée à migrer (le cache `playback/` est jetable).

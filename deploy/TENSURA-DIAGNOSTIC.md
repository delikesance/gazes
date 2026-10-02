# Diagnostic Tensura S01E01 — 2 octobre 2026 UTC

## Résultat mesuré

La stack Docker finale est saine (backend, frontend et Prowlarr). Le test réel Chromium sur `http://localhost:18080/anime/101280/seasons/101280/episodes/1` a confirmé **30.14 secondes de progression vidéo pendant 30.20 secondes**, en 1920 × 1080, sans erreur JavaScript. Ce test utilise le swarm réel et aucun média simulé.

Session : `54b0ad5f-3a15-499a-b7a0-7a35d3d429b9`. Le test s'est déroulé entre `2026-10-02T11:01:10.547Z` et `2026-10-02T11:02:07.868Z`.

Fichier sélectionné : `01 - Season 1/[Trix] Tensei Shitara Slime Datta Ken - S01E01 (BD 1080p AV1).mkv`, index 0 dans un pack de 65 fichiers. Infohash : `21ff74256476e035de1de7e04fa368a0abbe9886`.

Le probe confirme de la vidéo AV1, des pistes audio japonaises et anglaises Opus, et une piste de sous-titres français ASS. **Cette source est VOSTFR ; aucune VF n'est confirmée par cette lecture.** Le remux conserve la vidéo (`video_mode=copy`). La priorité VF puis VOSTFR reste inchangée.

## Causes identifiées et corrections

Le lecteur avait perdu l'appel à `episodeFile` : la variable de correspondance restait vide et le fichier principal pouvait être choisi aveuglément. Il sélectionne désormais un fichier correspondant à l'épisode et rejette un pack absent ou ambigu avec une cause identifiable.

Le frontend Docker ne recevait pas `BACKEND_URL` au runtime : les recherches côté serveur visaient `127.0.0.1:8090` dans le conteneur frontend et échouaient avec `ECONNREFUSED`. Compose transmet désormais `http://backend:8090` ; cette erreur est absente des logs du déploiement final.

La recherche reste partielle : certains fournisseurs sont en cooldown ou indisponibles. Le bilan de cette session contient 345 résultats avant filtrage, 16 sources retenues dont 2 françaises, 24 recherches réussies et 165 recherches échouées. Nyaa fournit une source utilisable malgré ces échecs. Les erreurs individuelles sont conservées, sans changement global des délais ni du classement.

## Rapport CLI conservé

[tensura-diagnostic.jsonl](tensura-diagnostic.jsonl) contient un extrait chronologique des événements persistants (résolution, sélection, probe, remux, démarrage et swarm). L'export complet contient 2254 événements et demeure consultable dans le volume Docker :

```bash
docker compose exec -T backend gazes-logs --session 54b0ad5f-3a15-499a-b7a0-7a35d3d429b9 --limit 10000
docker compose exec -T backend gazes-logs --session 54b0ad5f-3a15-499a-b7a0-7a35d3d429b9 --limit 10000 --json > tensura-session.jsonl
docker compose exec -T backend gazes-logs --anime 101280 --season 101280 --episode 1 --follow
```

## Vérifications

- `go test -tags=nosqlite -race ./internal/... ./cmd/...` : réussi.
- TypeScript (`tsc --noEmit`) et scénarios navigateur du lecteur : réussis (packs, erreurs métadonnées, décodage, stalls, changement de source, reprise, épuisement et retry, télémétrie corrélée).
- Tests du stockage : niveau, corrélation, secrets, rotation, rétention, migration, requêtes CLI, reprise sans doublons, SQLite verrouillée, stockage indisponible, disque plein, saturation et fermeture.
- La première session `9ed763ea-e191-4e62-919a-9b9082429ddd` conserve exactement les mêmes 2 651 événements après redémarrage du backend.
- Les deux clés présentes dans la configuration d'indexeurs ne figurent ni dans les logs backend ni dans l'export de session ; aucun magnet complet n'y figure.

## Reproduction

```bash
docker compose up -d --build --wait
# Adapter TEST_BASE_URL au port exposé (8080 par défaut).
TEST_BASE_URL=http://127.0.0.1:8080 pnpm --prefix web exec node scripts/tensura-diagnostic.mjs
```

Le script nécessite Playwright et Chromium ; `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` permet de choisir le binaire. Il conserve son rapport JSON dans `/tmp/gazes-tensura-browser.json` (configurable via `DIAGNOSTIC_REPORT`). La disponibilité des swarms reste variable ; le succès documenté ici correspond au test daté ci-dessus.

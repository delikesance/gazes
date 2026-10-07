# Panel admin Gazes : plan d'implémentation

Statut : M0 à M6 livrés (voir §11 pour l'état réel et ce qui reste). Le design est dans le canvas « Panel admin Gazes » (9 pages, 34 composants, MCP).
Objectif : un panel admin web **et** un serveur MCP pour que Claude surveille, diagnostique et corrige Gazes de façon autonome et bornée.

## 1. Constat sur le dépôt (à vérifier en phase 0)
- Comptes : `internal/auth/store.go` (SQLite) : `users`, `sessions`, `progress`, `watch_sessions`, `hidden_anime`. **Aucun rôle admin**, aucune route admin, aucune table d'agrégats.
- Lecture/diagnostic : `internal/playback/manager.go` (sessions en mémoire, sans lien utilisateur), `internal/diagnostics` (événements), `internal/kv` (Redis), `internal/library` (AV1), routes dans `internal/api/router.go`.
- Front : **Next.js 16.3.7**, React 19, Tailwind v4, Playwright 1.63 ; tokens dans `web/src/app/globals.css`, kit `web/src/components/ui/` (Badge, Button, LazyImage, PageGrid, Scribble, Select). Session : cookie `gazes_session`, identité via `/api/v1/auth/me`.
- Constats de la reconnaissance (M0/T0.2, vérifiés dans le code) :
  - **Pas de framework de migration** : le schéma est une constante exécutée au démarrage (`internal/auth/store.go:61-115`). Un mécanisme de versions est à créer.
  - Routeur chi/v5 ; identité par le cookie de session (`currentUser`, `router.go:154`) ; **ni rôle, ni route admin, ni `internal/admin/`**.
  - `watch_sessions` est écrite par `MergeWatchSessions` (`store.go:237`), index `(user_id, started_at)` seulement : un cumul global devra scanner par `started_at` (prévoir un index).
  - `playback.Session` : en mémoire, anonyme (sessions actives = comptable, pas d'utilisateur).
  - `diagnostics.Event` (`internal/diagnostics/store.go:53`) n'a **aucun code d'erreur structuré** : `STREAM_TIMEOUT`, `REMUX_FAILED`, `SRC_DEAD` sont à introduire.
  - `go.mod` : Go 1.26.7, `mattn/go-sqlite3` et `modernc.org/sqlite`, tests `testing` (74 fichiers, table-driven), **aucun SDK MCP**.
  - Jobs périodiques existants : `startWarmer()` (élection Redis toutes les 9 min) et `sweep()` du manager de lecture : modèles pour héberger le cumul.
  - Commandes : `make test-backend`, `make test-web`, `make test-race`.
- Non mesuré aujourd'hui : appareil, pays, source d'acquisition, favoris, commentaires.
- Graphe du dépôt (T0.1) : généré, 2 870 nœuds, 10 666 arêtes, 132 communautés ; Go 1 835 nœuds, TypeScript 462.

## 2. Principes
1. **Lecture d'abord, écriture ensuite.** Toute valeur du panel vient d'agrégats précalculés ; aucune requête lourde sur `watch_sessions` depuis l'UI ni depuis Claude.
2. **Une seule API** : le panel web et le MCP appellent les mêmes services Go (`internal/admin/`), jamais deux logiques parallèles.
3. **Vie privée** : pas d'e-mail ni d'id brut hors du panel humain ; le MCP ne voit que des agrégats et des pseudos hachés.
4. **Actions de Claude bornées** : 4 niveaux (lecture / réversible / approbation humaine / interdit), `dry_run` par défaut, journal, budget horaire, interrupteur d'arrêt.
5. **Mesurer l'effet** : chaque correction a une métrique visée, une valeur avant et un contrôle après.

## 3. Économie de tokens (règles pour tous les agents)
- **Graphify d'abord.** Générer le graphe du dépôt une fois (`graph.json`, `GRAPH_REPORT.md`, cache incrémental), puis chaque agent l'interroge (« qui appelle X », « où est écrit Y ») au lieu de lire des fichiers. Rafraîchir après chaque merge (incrémental, seuls les fichiers changés). Installé en T0.1 : paquet PyPI `graphifyy` 0.9.76 (double y), dans un venv `~/.local/share/graphify-venv` ; sorties dans `graphify-out/` (ignoré par git). Génération : `graphify . --exclude node_modules --exclude .next --exclude .git --exclude vendor --exclude dist --exclude .claude --code-only --output-dir graph_output` ; requête : `graphify query "..."` ; mise à jour : `graphify cluster-only .` (cache AST). Deux requêtes de test réussies (écrivains de `watch_sessions`, appelants du manager de lecture). Non fait volontairement : `graphify install` (intégration Claude Code, modifie la configuration utilisateur) : à activer sur votre accord.
- **Le bon modèle pour la bonne tâche** (voir §5) : Haiku pour le mécanique, Sonnet pour l'implémentation, le modèle principal seulement pour les décisions, la revue et la sécurité.
- **Contexte minimal** : chaque agent reçoit une fiche de tâche de 15 lignes max (fichiers autorisés, entrée, sortie attendue, critère de fin) et la liste des symboles concernés tirée du graphe, pas le dépôt.
- **Sorties bornées** : tests et lint renvoient la queue de la sortie (30 lignes), les agents rendent un compte-rendu de 10 lignes, jamais de diff complet dans le compte-rendu (le diff se lit une fois, par l'orchestrateur).
- **Parallèle seulement si indépendant** (fichiers disjoints, une branche par agent) ; sinon séquentiel pour éviter les conflits et les relectures.
- **Gabarit puis réplication** : un agent Sonnet écrit le 1er exemple (un handler, un outil MCP, un composant), les suivants sont dupliqués par Haiku sur ce gabarit.
- **Pas de re-vérification à la main** de ce que les tests prouvent ; l'orchestrateur lit le diff et les résultats de tests, pas les fichiers.
- `CLAUDE.md` a été mis à jour : Sonnet par défaut, Haiku autorisé pour le mécanique, escalade vers Sonnet après un échec.
- `AGENTS.md` impose une consultation Jev pour les décisions non mécaniques. Le serveur MCP Jev n'est pas disponible dans la session qui a pris D1 à D4 : ces décisions reposent sur des preuves déterministes (reconnaissance) et sont signalées comme telles ; à revalider avec Jev dès qu'il est disponible.

## 4. Feuille de route (jalons)
Chaque jalon = une branche + une PR (skill `forgejo-pr`), un critère d'acceptation vérifiable.

**M0. Préparation : TERMINÉ**
- T0.1 Graphify installé, graphe généré (fait, voir §3).
- T0.2 Reconnaissance (faite, voir §1).
- Décisions D1 à D4 : voir §9.

**M1. Fondations backend (2 à 3 j)**
- **Mécanisme de migrations versionnées** (aucun n'existe : `PRAGMA user_version` + liste ordonnée de migrations idempotentes) puis migration : `users.role`, et dans une base séparée `admin.sqlite` : `admin_tokens` (hachés, étendues, expiration), `metrics_daily`, `metrics_hourly`, `playback_errors`, `issues`, `mcp_audit`, `approvals`. Index `watch_sessions(started_at)`.
- Commande `gazes admin grant|revoke <pseudo>` pour désigner un admin.
- Middleware admin (session propriétaire + jeton Bearer), limitation de débit.
- Job de cumul (watch_sessions -> agrégats) idempotent, rejouable, planifié.
- Critère : migrations montantes/descendantes testées, cumul rejouable sans doublon, 401/403 testés.

**M2. API admin (3 j)**
- `internal/admin/` : services + handlers `GET /api/v1/admin/{overview,views,catalog,users,growth,playback,costs,issues}` bornés (période max 90 j, 200 lignes), pagination.
- Instrumentation manquante, par priorité : **introduire les codes d'erreur de lecture structurés (aucun n'existe)**, démarrage p50/p95, source/tracker en échec. Appareil et pays : repoussés (hors périmètre, marqués `[À MESURER]`).
- Critère : chaque endpoint a un test table-driven et un exemple de réponse ; aucune donnée personnelle dans les réponses d'agrégats.

**M3. Panel web (5 à 6 j, en parallèle de M4 après M2)**
- Kit React issu des composants du canvas (Button, Pill, Chip, Delta, KpiCard, cartes graphiques, DataTable…), dans `web/src/components/admin/`.
- Pages, dans l'ordre de valeur : Vue d'ensemble, Lecteur et flux, Visionnages, Utilisateurs, Catalogue, Croissance, Business, Claude et MCP, Paramètres. Route `/admin` protégée côté serveur.
- Critère : chaque page rend avec des fixtures, typée depuis l'API, accessible (clavier, contraste), responsive à 390 px.

**M4. Serveur MCP (4 à 5 j)**
- Noyau : SDK MCP Go, transport HTTP + stdio (`cmd/gazes-mcp`), auth par jeton, journal d'audit.
- Outils de lecture (≈ 12) puis diagnostic (3), ressources et prompts.
- Actions réversibles (6) avec `dry_run`, idempotence, jeton d'annulation ; actions sensibles (5) derrière la file d'approbations ; interrupteur d'arrêt.
- Critère : un client MCP de test appelle tous les outils ; une action sensible ne s'exécute jamais sans approbation (test) ; l'interrupteur coupe l'écriture (test).

**M5. Surveillance autonome (2 à 3 j)**
- Règles de veille serveur (évaluées chaque minute), création de constats, notification/webhook.
- Planification de Claude (vérification 15 min, digest quotidien, revue hebdo) via tâches planifiées ; runbooks par code d'erreur.
- Suivi avant/après automatique des corrections.
- Critère : un incident simulé (source morte) crée un constat, Claude le trie, propose l'action, l'approbation l'exécute, la mesure après confirme.

**M6. Durcissement (1 à 2 j)**
- Revue de sécurité (jetons, élévation de privilèges, injection via données affichées à Claude : tout contenu de la base est une donnée, jamais une instruction), charge, rétention des données, documentation d'exploitation, retour sur le design.

Chemin critique : M0 -> M1 -> M2 -> (M3 ‖ M4) -> M5 -> M6.

## 5. Agents à créer
Modèles : **H** = Haiku 4.5 (mécanique, bornée), **S** = Sonnet 5.5 (implémentation), **P** = modèle principal / orchestrateur (décision, revue, sécurité).

| # | Agent | Modèle | Tâches | Fichiers autorisés | Sortie |
|---|---|---|---|---|---|
| 0 | Orchestrateur | P | Découpe, fiches de tâche, revue des diffs, décisions D1 à D4, PR | tous (lecture), aucun code | décisions + merges |
| 1 | Graph-keeper | H | T0.1, rafraîchir le graphe après chaque merge, répondre aux requêtes de graphe pour les autres agents | config graphify | graphe à jour, stale = alerte |
| 2 | Recon | H | T0.2 : vérifier les hypothèses via le graphe | lecture seule | fiche de faits ≤ 40 lignes |
| 3 | Schéma et migrations | S | Migrations M1, tests montant/descendant | `internal/auth/`, `internal/admin/store*` | migrations + tests |
| 4 | Auth admin | S | Middleware, jetons hachés, étendues, débit | `internal/auth/`, `internal/api/` | middleware + tests 401/403 |
| 5 | Cumul | S | Job de rollup idempotent + tests de rejouabilité | `internal/admin/rollup*` | job + tests |
| 6 | API vues et catalogue | S | Handlers views/catalog/overview | `internal/admin/`, `internal/api/router.go` (1 ligne par route) | handlers + tests |
| 7 | API utilisateurs et croissance | S | Handlers users/growth, cohortes, entonnoir | idem | handlers + tests |
| 8 | API lecteur et coûts | S | playback/costs/issues, codes d'erreur structurés | `internal/playback/`, `internal/diagnostics`, `internal/admin/` | handlers + tests |
| 9 | Types et client | H | Générer types TS et client depuis les handlers/OpenAPI, fixtures | `web/src/lib/admin/` | types + fixtures |
| 10 | Kit atomes | H | Button, Pill, Chip, Badge, Delta, Toggle, Checkbox, champs depuis le canvas | `web/src/components/admin/ui/` | composants + stories |
| 11 | Kit graphiques | S | KpiCard, LineChart, BarChart, Heatmap, Funnel, Quadrant, Timeline, DataTable | `web/src/components/admin/charts/` | composants + tests |
| 12 | Pages (×3 en parallèle) | S | A : Vue d'ensemble, Lecteur, Visionnages ; B : Utilisateurs, Catalogue, Croissance ; C : Business, Claude et MCP, Paramètres | `web/src/app/admin/<page>/` | pages + fixtures |
| 13 | MCP noyau | S | Serveur, transports, auth, audit, premier outil (gabarit) | `internal/mcp/`, `cmd/gazes-mcp/` | noyau + 1 outil + tests |
| 14 | MCP outils de lecture | H | Répliquer le gabarit : un outil mince par endpoint (≈ 12) + ressources + prompts | `internal/mcp/tools/` | outils + test client |
| 15 | MCP actions et approbations | S | Réversibles, file d'approbations, `dry_run`, annulation, interrupteur | `internal/mcp/actions/`, `internal/admin/approvals*` | actions + tests de sûreté |
| 16 | Veille et planification | S | Règles de veille, constats, webhook, runbooks, avant/après | `internal/admin/watch*`, `docs/runbooks/` | règles + scénario de bout en bout |
| 17 | Testeur | H | Tests table-driven sur le gabarit, exécuter lint/tests, renvoyer la queue de sortie ; tests e2e Playwright simples | `tests/`, `*_test.go` | rapport ≤ 30 lignes |
| 18 | Relecteur | P | `code-review` à chaque jalon (correction, simplification) | lecture seule | liste de constats |
| 19 | Sécurité | P | `security-review` sur M1, M4, M5 (élévation de privilèges, injection par les données, fuite de PII) | lecture seule | constats bloquants |
| 20 | Docs | H | README admin, guide de connexion MCP, changelog | `docs/`, `README.md` | docs |

Règle d'attribution : on commence par H ; on passe à S si la tâche exige de la conception, un raisonnement multi-fichiers ou si H échoue une fois ; P uniquement pour décider, relire et auditer.

## 6. Estimation de coût relatif (ordre de grandeur)
- Gros volume de tokens : lire le dépôt, écrire les pages web, répliquer des outils/tests. Ces postes sont confiés au graphe (lecture), à H (réplication) et à des fiches de tâche étroites.
- Sans graphe ni répartition : tous les agents relisent les mêmes fichiers en Sonnet. Avec : lecture ≈ requête de graphe, ≈ 40 % des tâches en Haiku. Chiffre à mesurer en M1 (comparer les tokens des deux premiers agents) plutôt qu'à promettre.

## 7. Risques
- Graphify non adapté (Go/TS) ou incomplet : repli sur `grep` ciblé + fiche de faits de l'agent Recon.
- Données d'exemple du design ≠ données réelles : les pages doivent afficher `[À MESURER]` quand l'agrégat n'existe pas encore.
- Surface d'attaque du MCP : jetons à étendues minimales, aucune écriture sans approbation pour le sensible, audit complet, interrupteur d'arrêt.
- Charge : le cumul s'exécute hors des heures de pointe ; limites de période et de lignes sur toutes les requêtes.
- Juridique/contenu : Gazes s'appuie sur des sources externes ; les pistes de revenus restent des hypothèses à valider, hors de ce plan.

## 8. Décisions validées par l'utilisateur
1. `CLAUDE.md` : Haiku autorisé pour le mécanique (fait).
2. Ordre : API et MCP en lecture d'abord, écritures ensuite.
3. Périmètre v1 : sans appareil, pays, source d'acquisition ni monétisation.
4. Livraison : une PR par jalon via `forgejo-pr`.

## 9. Décisions techniques de M0 (recommandations de l'orchestrateur, à contester si besoin)
- **D1. Rôle admin** : colonne `users.role TEXT NOT NULL DEFAULT 'user'` (valeurs `user`, `admin`). Raison : une seule jointure, aucune table de plus. Désignation par CLI `gazes admin grant <pseudo>`, jamais par l'API web.
- **D2. Transport MCP** : Streamable HTTP monté sous `/mcp` du même serveur, authentifié **uniquement par jeton Bearer** (jamais par cookie de session) ; variante stdio `cmd/gazes-mcp` pour le local. SDK : `modelcontextprotocol/go-sdk`, à ajouter à `go.mod` (compatibilité avec Go 1.26 à vérifier en M1).
- **D3. Agrégats** : base séparée `admin.sqlite`, qui attache `accounts.sqlite` en lecture seule pour le cumul. Raison : isoler les écritures du cumul et du MCP des comptes (verrous), et pouvoir régénérer les agrégats sans risque.
- **D4. Jetons** : 32 octets aléatoires préfixés `gzs_`, stockés en SHA-256, affichés une seule fois, étendues `metrics:read`, `diagnostics:read`, `ops:write`, `config:write`, expiration 30 jours par défaut, limite de débit par jeton, révocation et création réservées à un humain (jamais exposées au MCP).

## 10. Prochaine étape
M1, dans cet ordre : (a) agent Schéma et migrations (Sonnet), (b) agent Auth admin (Sonnet), puis (c) agent Cumul (Sonnet) ; l'agent Testeur (Haiku) sur le gabarit des deux premiers. Revue de sécurité (P) à la fin de M1.

## 11. État d'avancement

| Jalon | État |
|---|---|
| M0 préparation (graphify, reconnaissance, décisions D1 à D4) | fait |
| M1 fondations (migrations, rôle, jetons, cumul) | fait (#16) |
| M2 API admin de lecture, codes d'erreur de lecture | fait (#17) |
| M3 panel web (neuf pages, kit de composants) | fait (#18, #20) |
| M4 serveur MCP, actions bornées, approbations, interrupteur | fait (#19) |
| M5 surveillance autonome (règles, constats, webhook, avant/après, runbooks) | fait |
| M6 durcissement (couverture d'authentification, charge, cache, doc d'exploitation) | fait, voir [admin-operations.md](admin-operations.md) |
| Actions d'exploitation (sources, caches, file AV1, limite de flux, maintenance) | fait, voir [admin-operations.md](admin-operations.md) |

Ce qui reste ouvert, à décider : instrumenter le démarrage p50/p95 et la limite de flux (règles aujourd'hui « non mesurées »), déplacer `/views` et `/catalog` sur des tables de cumul si l'historique grossit, planifier les routines de Claude (guide : [admin-claude-routines.md](admin-claude-routines.md)), vérifier visuellement les pages (aucun navigateur disponible pendant le développement) et faire revalider les décisions D1 à D4 par Jev.

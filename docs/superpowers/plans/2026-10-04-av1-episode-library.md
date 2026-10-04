# Bibliothèque d'épisodes AV1 — plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Garder chaque épisode regardé sur les disques du serveur, réencodé en AV1, et le relire localement au lieu de repasser par le torrent.

**Architecture:** Nouveau package `internal/library` dans le serveur Go : index SQLite, groupe de disques étiquetés, acquisition complète via le moteur torrent, encodeur SVT-AV1 en tâche de fond, nettoyage par ancienneté d'accès. Les copies locales sont exposées sous un identifiant de 40 caractères hexadécimaux (`stream_id`) par un `torrent.Engine` composite : toutes les routes de lecture existantes (`/stream`, `/stream/raw`, `/subtitles`, `/metadata`, HLS) fonctionnent sans changement avec `ih=<stream_id>&file_idx=0`. Le frontend insère les copies disponibles en tête des candidats de lecture.

**Tech Stack:** Go 1.x, chi v5, `github.com/mattn/go-sqlite3`, anacrolix/torrent v1.61.0, ffmpeg 8 (`libsvtav1`, `libopus`), Next.js/TypeScript, tests `node:test`.

**Spec:** `docs/superpowers/specs/2026-10-04-av1-episode-library-design.md`

## Global Constraints

- Tests et builds Go : toujours `-tags=nosqlite` (sinon conflit de symboles SQLite entre mattn et la lib torrent). Préfixe : `GOMODCACHE=/home/workstation/go/pkg/mod`. Échecs préexistants à ignorer : `TestRanking`, `TestTargetedFrenchSources` (`internal/indexer`).
- SQLite : uniquement `mattn/go-sqlite3`, `sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")`, schéma `CREATE TABLE IF NOT EXISTS` exécuté à l'ouverture (modèle `internal/auth/store.go`).
- Identité d'une copie : `(season_id int, episode int, lang string)`, `lang ∈ {"vostfr","vf"}`. Mapping depuis une source : `VF`/`MULTI` → `vf`, `VOSTFR` → `vostfr`, autres → pas de mise en cache.
- États : `DOWNLOADING`, `ORIGINAL`, `ENCODING`, `AV1`, `UNAVAILABLE`. `encode_skipped ∈ {"", "not_smaller", "encode_failed"}`.
- Chemins : repère disque `.gazes-library` (contenu JSON `{"disk_id":"<uuid>"}`), fichier `<season_id>/<episode>-<lang>.mkv`, temporaires `<…>.tmp` et `<…>.av1.tmp.mkv`.
- Réserve par disque : `max(LIBRARY_RESERVE_PERCENT % × capacité, LIBRARY_RESERVE_BYTES)`, défauts `10` et `50000000000`.
- Réservations : téléchargement = taille du fichier ; encodage = `0,5 ×` taille d'origine.
- Encodage : `libsvtav1`, preset `8`, CRF `30`, `yuv420p10le`, `lp=8`, `nice -n 19` ; audio AAC/Opus copié, sinon `libopus` 128k (≤ 2 canaux) / 256k (> 2 canaux) ; `-map 0 -map -0:d? -c:s copy -c:t copy -map_chapters 0 -map_metadata 0`.
- Vérification : durée à ±1 s, même nombre de pistes audio et sous-titres, décodage de 2 s au début, au milieu et à la fin, taille > 0 ; AV1 ≥ original → `not_smaller`. Max 3 essais.
- Délais : rescan disques 60 s, janitor 5 min, réconciliation 1 h, blocage téléchargement 24 h, copies protégées si accédées depuis < 1 h, statut encodeur toutes les 10 s.
- Limites API : 20 nouvelles copies par utilisateur et par heure, 4 téléchargements complets simultanés.
- `stream_id` = 40 premiers caractères hex de `HMAC-SHA256(secret, "<season_id>:<episode>:<lang>")`, secret aléatoire de 32 octets stocké dans la table `meta` de l'index.
- Aucun chemin fourni par le client n'est jamais ouvert ; les routes lisent uniquement des fichiers référencés par l'index.
- Les erreurs du module ne doivent jamais empêcher une lecture torrent.

## Review Focus

1. **Pack de saison** : deux épisodes d'un même torrent mis en cache en même temps ; l'épinglage doit tenir tant qu'un fichier du torrent est en cours, et le désépinglage d'un fichier ne doit pas relâcher l'autre. → test dans la tâche 3.
2. **Redémarrage au milieu d'une copie ou d'un encodage** : `.tmp` orphelins, réservations fantômes, entrées `ENCODING`. → test de reprise dans les tâches 1 et 7.
3. **Disque retiré pendant un encodage ou une lecture** : l'encodage échoue sans supprimer l'entrée ni compter un essai ; l'entrée passe `UNAVAILABLE`. → test dans la tâche 6.
4. **Fichier source avec pièces jointes (polices) et flux de données** : `-map 0` ne doit pas échouer sur un flux data ; polices conservées. → fixture de la tâche 6.
5. **Copie AV1 relue par le pipeline HLS** : `ReadIndex` doit trouver les Cues du MKV produit par ffmpeg. → test d'intégration dans la tâche 6.

---

### Task 1: Index SQLite (`Store`)

**Files:**
- Create: `internal/library/store.go`, `internal/library/types.go`
- Test: `internal/library/store_test.go`

**Interfaces:**
- Produces (`types.go`):
  - `type State string` avec les constantes `StateDownloading`, `StateOriginal`, `StateEncoding`, `StateAV1`, `StateUnavailable`
  - `type Key struct{ SeasonID, Episode int; Lang string }`, `func (k Key) String() string` → `"<season>:<ep>:<lang>"`
  - `type Entry struct{ Key; AnimeID int; Title string; State, PrevState State; InfoHash string; FileIndex int; ReleaseName, DiskID, RelPath, SHA256, VideoCodec, LastError, EncodeSkipped string; SizeBytes, OriginalSizeBytes, ReservedBytes, DurationMS int64; AudioTracks, SubtitleTracks, Attempts int; CreatedAt, UpdatedAt, LastAccessAt time.Time }`
  - `var ErrExists = errors.New("library: entry exists")`, `var ErrNotFound = errors.New("library: entry not found")`
- Produces (`store.go`):
  - `func OpenStore(dir string) (*Store, error)` (fichier `library.sqlite`), `func (s *Store) Close() error`
  - `func (s *Store) Create(e Entry) error` (`ErrExists` si la clé existe)
  - `func (s *Store) Get(k Key) (Entry, error)`, `func (s *Store) Delete(k Key) error`
  - `func (s *Store) Update(k Key, fn func(*Entry) error) (Entry, error)` (transaction ; met à jour `UpdatedAt`)
  - `func (s *Store) List(f Filter) ([]Entry, error)` avec `type Filter struct{ States []State; DiskID string; SeasonID int; Episode int; Limit int; OrderBy string }` (`OrderBy` ∈ `"updated_at"`, `"last_access_at"`)
  - `func (s *Store) Touch(k Key, at time.Time) error` (`LastAccessAt`)
  - `func (s *Store) Recover() (RecoverReport, error)` : `ENCODING` → `ORIGINAL` ; `DOWNLOADING` laissé tel quel (traité par l'acquéreur) ; `ReservedBytes` remis à 0 partout. `type RecoverReport struct{ Encoding, Downloading int }`
  - `func (s *Store) Secret() ([]byte, error)` (créé au premier appel, table `meta`)
  - `func (s *Store) StreamID(k Key) (string, error)` et `func (s *Store) ByStreamID(id string) (Entry, error)` (colonne `stream_id` indexée, remplie à `Create`)
  - `func (s *Store) SetEncoderStatus(st EncoderStatus) error`, `func (s *Store) EncoderStatus() (EncoderStatus, error)` avec `type EncoderStatus struct{ Key *Key; Progress float64; Paused bool; UpdatedAt time.Time }`

- [ ] **Step 1: Write the failing tests**

```go
func TestCreateGetAndUniqueKey(t *testing.T)            // Create puis Get ; 2e Create même clé → ErrExists
func TestUpdateIsTransactionalAndStampsUpdatedAt(t *testing.T) // fn qui renvoie une erreur → aucune modification
func TestListFiltersAndOrdersByLastAccess(t *testing.T) // OrderBy "last_access_at" croissant, filtre States et DiskID
func TestRecoverResetsEncodingAndReservations(t *testing.T) // ENCODING→ORIGINAL, ReservedBytes=0, Downloading compté
func TestStreamIDIsStableHexAndResolvable(t *testing.T) // len 40, [0-9a-f], stable après réouverture, ByStreamID retrouve l'entrée
func TestConcurrentCreateSameKeyOnlyOneWins(t *testing.T) // 10 goroutines → 1 succès, 9 ErrExists
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(Create|Update|List|Recover|StreamID|Concurrent)' -v`
Expected: FAIL (package or symbols undefined)

- [ ] **Step 3: Implement `types.go` and `store.go`**

Table `episodes` : colonnes de la spec + `stream_id TEXT UNIQUE` + `prev_state` ; clé primaire `(season_id, episode, lang)`. Tables `meta(key TEXT PRIMARY KEY, value BLOB)` et `encoder_status(id INTEGER PRIMARY KEY CHECK(id=1), …)`. Dates en Unix ms.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=nosqlite -race ./internal/library/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library && git commit -m "Add the library episode index"
```

---

### Task 2: Groupe de disques (`Pool`)

**Files:**
- Create: `internal/library/pool.go`
- Test: `internal/library/pool_test.go`

**Interfaces:**
- Consumes: `Store.List`, `Store.Update` (tâche 1)
- Produces:
  - `type Disk struct{ ID, Label, Path string; Capacity, Free, Reserve int64; Present bool }`
  - `type StatFS func(path string) (capacity, free int64, err error)` ; `func SysStatFS(path string) (int64, int64, error)` (`syscall.Statfs`)
  - `func NewPool(root string, store *Store, statfs StatFS, reservePercent int, reserveBytes int64) *Pool`
  - `func (p *Pool) Scan() (added, removed []Disk, err error)` : sous-dossiers directs de `root` contenant `.gazes-library` ; disque disparu → entrées du disque `State=UNAVAILABLE`, `PrevState=<ancien>` ; disque revenu → `State=PrevState`
  - `func (p *Pool) Disks() []Disk`
  - `func (p *Pool) Reserve(size int64) (Reservation, error)` : disque présent avec le plus grand `Free − Reserve − réservé` ≥ `size`, sinon `ErrNoSpace`
  - `type Reservation struct{ DiskID, Path string; Size int64 }`, `func (p *Pool) Release(r Reservation)`
  - `func (p *Pool) Grow(r *Reservation, extra int64) error` (encodage)
  - `func (p *Pool) Under(diskID string) int64` : octets manquants pour revenir au-dessus de la réserve (0 si OK)
  - `func (p *Pool) Path(diskID, rel string) (string, error)` (`ErrDiskAbsent`)
  - `var ErrNoSpace, ErrDiskAbsent error`
  - `func EnsureMarker(dir string) (diskID string, err error)` (crée le repère avec un UUID v4 s'il manque)

- [ ] **Step 1: Write the failing tests**

```go
func TestScanFindsOnlyMarkedDirs(t *testing.T)          // 3 dossiers, 2 avec repère → 2 disques
func TestReservePicksMostFreeAndHonoursReserve(t *testing.T) // statfs factice : A cap 1000 free 300, B cap 1000 free 250 ; reserve 10%/0 → Reserve(150) va sur A ; Reserve(201) sur A refusé puis B ; Reserve(500) → ErrNoSpace
func TestConcurrentReservationsDoNotOverbook(t *testing.T) // 20 goroutines Reserve(10) sur 100 dispo → 10 succès
func TestRemovedDiskMarksEntriesUnavailableAndRestores(t *testing.T) // entrée AV1 sur disque X ; retrait du repère → UNAVAILABLE ; remise du même repère (même disk_id, autre nom de dossier) → AV1
func TestReserveUsesLargerOfPercentAndBytes(t *testing.T) // cap 1 000 000, 10% vs 50 000 octets fixes
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(Scan|Reserve|Concurrent|Removed)' -v`
Expected: FAIL

- [ ] **Step 3: Implement `pool.go`**

Les disques sont indexés par `disk_id` (lu dans le repère), pas par nom de dossier.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=nosqlite -race ./internal/library/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library && git commit -m "Add the library disk pool with labelled disks and reservations"
```

---

### Task 3: Téléchargement complet et épinglage dans le moteur torrent

**Files:**
- Modify: `internal/torrent/leases.go` (`pieceScheduler.apply`, ~l.49-69), `internal/torrent/client.go` (méthodes, `evictIdle` ~l.422), `internal/torrent/evict.go` (`cacheEntry`, `pickEvictions`)
- Test: `internal/torrent/fullfile_test.go`, `internal/torrent/evict_test.go`

**Interfaces:**
- Produces (méthodes de `*ClientEngine`, utilisées par la tâche 4 via l'interface `library.Fetcher`) :
  - `func (e *ClientEngine) DownloadFile(infoHash string, fileIndex int) error` : plancher de priorité `PiecePriorityNormal` sur les pièces du fichier, conservé quand les fenêtres de lecture se ferment ; épingle le fichier
  - `func (e *ClientEngine) ReleaseFile(infoHash string, fileIndex int)` : retire le plancher et l'épinglage de ce fichier seulement
  - `func (e *ClientEngine) FileProgress(infoHash string, fileIndex int) (completed, length int64, err error)` (`File.BytesCompleted()`)
  - `func (e *ClientEngine) VerifyFile(ctx context.Context, infoHash string, fileIndex int) error` : rehash de toutes les pièces du fichier (`VerifyDataContext`) ; erreur si une pièce est rejetée
- Scheduler : `func (s *pieceScheduler) setFloor(begin, end int, on bool)` ; `apply()` prend `max(fenêtres, plancher)` et ne met jamais `PiecePriorityNone` sur une pièce couverte par un plancher.
- Éviction : `cacheEntry` gagne `pinned bool` ; `pickEvictions` ignore les entrées épinglées ; un torrent est épinglé tant qu'au moins un de ses fichiers l'est.

- [ ] **Step 1: Write the failing tests**

```go
func TestPickEvictionsSkipsPinned(t *testing.T)               // entrée pinned, ancienne et grosse → jamais renvoyée
func TestSchedulerFloorSurvivesWindowRelease(t *testing.T)     // plancher [10,20) + fenêtre [12,14) High ; fermeture fenêtre → 12,13 restent Normal, pas None
func TestSchedulerFloorReleaseOnlyAffectsItsRange(t *testing.T) // planchers [0,10) et [10,20) ; retrait du 1er → [10,20) toujours Normal
func TestTorrentStaysPinnedWhileAnyFileIsPinned(t *testing.T)  // DownloadFile f1 et f2, ReleaseFile f1 → toujours pinned ; ReleaseFile f2 → plus pinned (Review Focus 1)
```

Les tests du scheduler utilisent l'abstraction de pièce déjà testable dans `leases.go` (ou une interface `pieceSetter{ SetPriority(i int, p PiecePriority) }` introduite pour l'occasion si `apply` appelle directement `s.torrent.Piece(i)`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/torrent/ -run 'Test(PickEvictionsSkipsPinned|SchedulerFloor|TorrentStaysPinned)' -v`
Expected: FAIL

- [ ] **Step 3: Implement the floor, pinning and the four methods**

Épinglage : `pinned map[string]map[int]bool` (infohash → fichiers) protégé par `e.mu`.

- [ ] **Step 4: Run all torrent tests**

Run: `go test -tags=nosqlite -race ./internal/torrent/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/torrent && git commit -m "Let the torrent engine download and pin a whole file"
```

---

### Task 4: Acquisition (`Acquirer`)

**Files:**
- Create: `internal/library/acquire.go`, `internal/library/probe.go`
- Test: `internal/library/acquire_test.go`

**Interfaces:**
- Consumes: `Store` (tâche 1), `Pool` (tâche 2), méthodes de la tâche 3
- Produces:
  - `type Fetcher interface{ GetFileStream(ctx context.Context, infoHash string, fileIndex int) (io.ReadSeekCloser, *torrent.FileInfo, error); DownloadFile(infoHash string, fileIndex int) error; ReleaseFile(infoHash string, fileIndex int); FileProgress(infoHash string, fileIndex int) (int64, int64, error); VerifyFile(ctx context.Context, infoHash string, fileIndex int) error }`
  - `type Prober interface{ Probe(ctx context.Context, path string) (MediaInfo, error) }` ; `type MediaInfo struct{ DurationMS int64; VideoCodec string; AudioCodecs []string; AudioChannels []int; SubtitleTracks, AttachmentTracks int }` ; `func FFprobe(bin string) Prober` (`ffprobe -v error -print_format json -show_format -show_streams`)
  - `type Request struct{ Key; AnimeID int; Title, InfoHash string; FileIndex int; ReleaseName string }`
  - `func NewAcquirer(store *Store, pool *Pool, f Fetcher, p Prober, clock func() time.Time, stall time.Duration, maxActive int) *Acquirer`
  - `func (a *Acquirer) Start(req Request) (Entry, bool, error)` : `bool` = nouvelle copie créée ; copie existante (hors `UNAVAILABLE`) → `Touch` et `false` ; `maxActive` (4) atteint → `ErrBusy`
  - `func (a *Acquirer) Run(ctx context.Context)` : sonde toutes les 30 s ; fichier complet → `VerifyFile`, copie via `GetFileStream` vers `<rel>.tmp`, `fsync`, `rename`, `Probe`, `SHA256`, `State=ORIGINAL`, `Release` réservation, `ReleaseFile` ; aucune progression pendant `stall` → suppression de l'entrée, `ReleaseFile`, `Release`
  - `func (a *Acquirer) Resume(ctx context.Context) error` : au démarrage, relance `DownloadFile` pour chaque `DOWNLOADING` (échec → suppression de l'entrée)
  - `var ErrBusy error`

- [ ] **Step 1: Write the failing tests** (faux `Fetcher` en mémoire, faux `Prober`, horloge injectée, pool sur `t.TempDir()`)

```go
func TestStartCreatesDownloadingAndPins(t *testing.T)       // DownloadFile appelé, Entry DOWNLOADING, réservation = taille
func TestStartExistingCopyOnlyTouches(t *testing.T)         // 2e Start même clé → false, LastAccessAt mis à jour, 1 seul DownloadFile
func TestCompletedFileIsVerifiedCopiedAndProbed(t *testing.T) // contenu identique octet pour octet, ORIGINAL, sha256, réservation libérée, ReleaseFile appelé, pas de .tmp restant
func TestVerifyFailureKeepsDownloading(t *testing.T)        // VerifyFile en erreur → reste DOWNLOADING, pas de fichier final
func TestStallAbandonsAfter24h(t *testing.T)                // progression figée 24h01 → entrée supprimée, ReleaseFile
func TestMaxActiveDownloadsReturnsBusy(t *testing.T)        // 5e Start → ErrBusy
func TestNoSpaceDoesNotCreateEntry(t *testing.T)            // Reserve → ErrNoSpace : Start renvoie l'erreur, aucune entrée
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(Start|Completed|Verify|Stall|MaxActive|NoSpace)' -v`
Expected: FAIL

- [ ] **Step 3: Implement `acquire.go` and `probe.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=nosqlite -race ./internal/library/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library && git commit -m "Download watched episodes whole into the library"
```

---

### Task 5: Moteur composite (lecture des copies locales)

**Files:**
- Create: `internal/library/engine.go`
- Test: `internal/library/engine_test.go`

**Interfaces:**
- Consumes: `torrent.Engine` (`AddTorrent`, `GetFileStream`, `GetStats`, `Close`), `Store.ByStreamID`, `Store.Touch`, `Pool.Path`
- Produces:
  - `func NewEngine(inner torrent.Engine, store *Store, pool *Pool, clock func() time.Time) *Engine` (implémente `torrent.Engine`)
  - `GetFileStream(ctx, hash, file)` : si `hash` est un `stream_id` d'une entrée `ORIGINAL`/`AV1`, et `file == 0` → `*os.File` enveloppé, `FileInfo{Index:0, Path:"<episode>-<lang>.mkv", Length:taille, IsVideo:true, MimeType:"video/x-matroska"}`, `Touch` ; fichier absent → entrée supprimée, `ErrLibraryMiss` ; sinon délégation à `inner`
  - `GetStats(hash)` pour un `stream_id` → `&torrent.SwarmStats{}` avec progression 100 % (champs existants de `SwarmStats`) ; sinon délégation
  - `func (e *Engine) ActiveStreams() int` : nombre de couples `(hash, file)` distincts ayant au moins un lecteur ouvert (torrent et local)
  - `func (e *Engine) InUse(k Key) bool`
  - `var ErrLibraryMiss error`

- [ ] **Step 1: Write the failing tests**

```go
func TestLocalCopyServedByStreamID(t *testing.T)     // lecture + Seek ; ServeRange de internal/stream renvoie 206, Content-Range, Accept-Ranges: bytes
func TestUnknownHashDelegatesToTorrent(t *testing.T) // faux inner appelé
func TestMissingFileDeletesEntryAndReturnsMiss(t *testing.T)
func TestActiveStreamsCountsDistinctFiles(t *testing.T) // 3 lecteurs sur (h,0) + 1 sur (h2,1) → 2 ; après Close de tous → 0
func TestDownloadingEntryIsNotServed(t *testing.T)   // état DOWNLOADING → délégation (pas de lecture locale)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(LocalCopy|UnknownHash|MissingFile|ActiveStreams|DownloadingEntry)' -v`
Expected: FAIL

- [ ] **Step 3: Implement `engine.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=nosqlite -race ./internal/library/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library && git commit -m "Serve library copies through the torrent engine interface"
```

---

### Task 6: Encodeur AV1

**Files:**
- Create: `internal/library/encode.go`, `internal/library/encode_args.go`
- Test: `internal/library/encode_args_test.go`, `internal/library/encode_test.go`, `internal/library/encode_integration_test.go` (tag de build `ffmpeg`)

**Interfaces:**
- Consumes: `Store`, `Pool` (`Grow`, `Release`, `Path`), `Prober`, `Engine.ActiveStreams`, `Engine.InUse`
- Produces:
  - `type EncodeSettings struct{ FFmpeg string; Preset, CRF, Threads, PauseStreams int; Window string }`
  - `func BuildArgs(in, out string, info MediaInfo, s EncodeSettings) []string` (sans `nice` ; ajouté par `Encoder`)
  - `func InWindow(window string, now time.Time) (bool, error)` (`""` → toujours vrai ; `"01:00-09:00"` ; plage qui passe minuit acceptée)
  - `func Verify(ctx context.Context, p Prober, ffmpeg, original, encoded string) error` (règles des contraintes globales)
  - `func NewEncoder(store *Store, pool *Pool, p Prober, active func() int, s EncodeSettings, clock func() time.Time) *Encoder`
  - `func (e *Encoder) Run(ctx context.Context)` : une entrée `ORIGINAL` (sans `EncodeSkipped`) à la fois, par `updated_at` croissant ; `Grow` de `0,5 ×` ; `ENCODING` ; progression via `-progress pipe:1` dans `SetEncoderStatus` toutes les 10 s ; `SIGSTOP` hors plage horaire ou si `active() >= PauseStreams`, `SIGCONT` sinon ; succès → `Verify`, `rename`, suppression de l'original, `AV1`, `RelPath` inchangé (`.mkv`), tailles mises à jour ; AV1 pas plus petit → `ORIGINAL` + `not_smaller` ; échec → `ORIGINAL`, `Attempts++`, `LastError`, `encode_failed` au 3e ; disque absent (`ErrDiskAbsent` ou fichier disparu) → `UNAVAILABLE` sans incrémenter `Attempts` ; arrêt du contexte → ffmpeg tué, `.tmp` supprimé, `ORIGINAL`

- [ ] **Step 1: Write the failing unit tests**

```go
func TestBuildArgsCopiesAACAndOpusTranscodesOthers(t *testing.T) // pistes [aac 2ch, ac3 6ch, flac 2ch] → -c:a:0 copy, -c:a:1 libopus -b:a:1 256k, -c:a:2 libopus -b:a:2 128k ; contient -c:v libsvtav1 -preset 8 -crf 30 -pix_fmt yuv420p10le -svtav1-params lp=8 -map 0 -map -0:d? -c:s copy -c:t copy -map_chapters 0 -map_metadata 0
func TestInWindow(t *testing.T)               // "" ; "01:00-09:00" à 00:59/01:00/08:59/09:00 ; "22:00-06:00" à 23:00 et 05:00
func TestVerifyRejectsDurationAndTrackMismatch(t *testing.T) // faux Prober : durée +2 s → erreur ; 1 piste audio en moins → erreur
func TestEncoderFailureKeepsOriginalAndCountsAttempts(t *testing.T) // ffmpeg factice (script qui sort 1) → ORIGINAL, Attempts 1..3, encode_failed au 3e, original intact
func TestEncoderNotSmallerKeepsOriginal(t *testing.T)
func TestEncoderPausesWhenBusyAndOutsideWindow(t *testing.T) // active()=3 → statut Paused=true ; retour à 0 → reprise
func TestDiskRemovedDuringEncodeMarksUnavailable(t *testing.T) // Review Focus 3 : Attempts inchangé, entrée UNAVAILABLE
```

Le « ffmpeg factice » est un script shell écrit dans `t.TempDir()` et passé dans `EncodeSettings.FFmpeg`.

- [ ] **Step 2: Write the failing integration test** (`//go:build ffmpeg`)

```go
func TestRealAV1EncodeKeepsTracksAndIsHLSIndexable(t *testing.T)
// fixture générée dans t.TempDir() par ffmpeg : 5 s testsrc2 1280x720 H.264,
// piste audio 1 AAC stéréo, piste audio 2 AC3 5.1, sous-titres ASS, police TTF en pièce jointe
// (`-attach` d'une police système trouvée via fc-match, sinon test sauté), flux data absent.
// Asserts : Verify OK ; ffprobe : vidéo av1, 2 audio (aac, opus), 1 subtitle ass, 1 attachment ;
// playback.ReadIndex(ctx, f, size) renvoie un index avec ≥ 1 segment (Review Focus 4 et 5).
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(BuildArgs|InWindow|Verify|Encoder|DiskRemoved)' -v` puis `go test -tags=nosqlite,ffmpeg ./internal/library/ -run TestRealAV1 -v` (dans l'image backend si ffmpeg absent de l'hôte : `docker run --rm -v "$PWD":/src -w /src golang:… ` ou `make dev-test`, au choix de l'implémenteur, à documenter dans le rapport)
Expected: FAIL

- [ ] **Step 4: Implement `encode_args.go` and `encode.go`**

- [ ] **Step 5: Run tests to verify they pass**

Run: les deux commandes de l'étape 3
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/library && git commit -m "Re-encode library copies to AV1 in the background"
```

---

### Task 7: Nettoyage et réconciliation (`Janitor`)

**Files:**
- Create: `internal/library/janitor.go`
- Test: `internal/library/janitor_test.go`

**Interfaces:**
- Consumes: `Store`, `Pool` (`Under`, `Path`, `Disks`), `Engine.InUse`
- Produces:
  - `func NewJanitor(store *Store, pool *Pool, inUse func(Key) bool, clock func() time.Time) *Janitor`
  - `func (j *Janitor) Free(diskID string) (freed int64, err error)` : supprime les copies `ORIGINAL`/`AV1` du disque par `last_access_at` croissant tant que `Under(diskID) > 0` ; ignore les copies en lecture, `ENCODING`, `DOWNLOADING`, ou accédées depuis < 1 h
  - `func (j *Janitor) Reconcile() (ReconcileReport, error)` : fichiers sans entrée, entrées sans fichier sur un disque présent, `*.tmp` et `*.av1.tmp.mkv` orphelins → supprimés. `type ReconcileReport struct{ OrphanFiles, MissingEntries, TempFiles int }`
  - `func (j *Janitor) Run(ctx context.Context)` : `Free` de chaque disque toutes les 5 min, `Reconcile` au démarrage puis toutes les heures
  - Le `Pool.Reserve` qui renvoie `ErrNoSpace` est retenté une fois par l'appelant après `Free` sur tous les disques (fait dans la tâche 8).

- [ ] **Step 1: Write the failing tests**

```go
func TestFreeDeletesLeastRecentlyAccessedFirst(t *testing.T) // 4 copies, accès J-10, J-5, J-2, J-1 ; manque 2 tailles → J-10 et J-5 supprimées
func TestFreeSkipsProtectedCopies(t *testing.T)              // en lecture, ENCODING, accédée il y a 30 min → conservées
func TestReconcileRemovesOrphansAndTemps(t *testing.T)       // Review Focus 2 : fichier sans entrée, entrée sans fichier, .tmp, .av1.tmp.mkv
func TestReconcileIgnoresAbsentDisks(t *testing.T)           // entrées d'un disque absent conservées
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/library/ -run 'Test(Free|Reconcile)' -v`
Expected: FAIL

- [ ] **Step 3: Implement `janitor.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=nosqlite -race ./internal/library/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library && git commit -m "Free library space by last access and reconcile disks"
```

---

### Task 8: Service, configuration, API HTTP et câblage

**Files:**
- Create: `internal/library/service.go`, `internal/api/library_handlers.go`
- Modify: `internal/config/config.go`, `internal/auth/service.go`, `internal/api/router.go` (`Server`, option, routes ~l.141-198), `cmd/server/main.go` (~l.72-120)
- Test: `internal/library/service_test.go`, `internal/api/library_handlers_test.go`, `internal/config/config_test.go`, `internal/auth/service_test.go`

**Interfaces:**
- Consumes: tâches 1 à 7
- Produces:
  - Config (`config.go`) : champs `LibraryEnabled bool`, `LibraryPoolDir`, `LibraryIndexDir`, `LibraryEncodeWindow string`, `LibraryReservePercent`, `LibraryEncodePreset`, `LibraryEncodeCRF`, `LibraryEncodeThreads`, `LibraryEncodePauseStreams int`, `LibraryReserveBytes int64`, `LibraryStallTimeout time.Duration` ; variables et défauts de la section Configuration de la spec (`LIBRARY_RESERVE_BYTES` en octets décimaux, défaut `50000000000`).
  - `func (s *Service) CurrentUser(r *http.Request) *User` exportée dans `internal/auth/service.go` (enveloppe `currentUser`).
  - `type Options struct{ PoolDir, IndexDir, FFmpeg, FFprobe string; ReservePercent int; ReserveBytes int64; Stall time.Duration; Encode EncodeSettings }`
  - `func Open(opts Options, inner torrent.Engine, fetcher Fetcher, logger *slog.Logger) (*Service, error)` : `OpenStore`, `Recover`, `Scan`, construit `Engine`, `Acquirer`, `Encoder`, `Janitor`
  - `func (s *Service) Engine() torrent.Engine`
  - `func (s *Service) Start(ctx context.Context)` : goroutines `Scan` (60 s), `Acquirer.Resume` + `Run`, `Encoder.Run`, `Janitor.Run`
  - `func (s *Service) Register(userID int64, req Request) (Entry, bool, error)` : limite 20 nouvelles copies par utilisateur et par heure (`ErrRateLimited`) ; sur `ErrNoSpace`, `Janitor.Free` de tous les disques puis un seul nouvel essai
  - `func (s *Service) Copies(seasonID, episode int) ([]Copy, error)` avec `type Copy struct{ Lang, State, VideoCodec, StreamID string; DurationMS int64; AudioTracks, SubtitleTracks int }` (états `ORIGINAL`/`AV1` uniquement)
  - `func (s *Service) Close() error`
  - Chaque transition journalisée par `diagnostics.Log` : `library.state`, `library.encode`, `library.evict`, `library.disk` avec `season_id`, `episode`, `lang`, `disk_id`, `duration_ms`.
  - API (`library_handlers.go`, enregistrées seulement si le service existe) :
    - `POST /api/v1/library/episodes/{season}/{ep}/{lang}`, corps `{"info_hash","file_index","release_name","anime_id","title"}` → 201 `{created:true}` / 200 `{created:false}` ; 401 sans utilisateur ; 422 si `lang` invalide, si l'épisode n'est pas dans `GetAnimeDetailsWithEpisodes(ctx, season).EpisodeList`, si `info_hash` n'est pas un torrent chargé ou si `file_index` n'est pas un fichier vidéo (`AddTorrent` interdit ici : vérifier via `GetStats` + liste des fichiers) ; 429 si `ErrRateLimited`/`ErrBusy` ; 507 si `ErrNoSpace`
    - `GET /api/v1/library/episodes/{season}/{ep}` → 200 `{"copies":[Copy…]}`
  - Câblage : `main.go` ouvre le service si `cfg.LibraryEnabled` (erreur d'ouverture → log `library.disabled` et poursuite sans bibliothèque), passe `service.Engine()` à `api.NewServer` à la place de `torrentEngine`, `api.WithLibrary(service)`, `service.Start(ctx)` avec un contexte annulé à l'arrêt, `defer service.Close()` avant `torrentEngine.Close`.
  - Pour vérifier qu'un fichier est vidéo sans `AddTorrent`, ajouter à `*ClientEngine` : `func (e *ClientEngine) Files(infoHash string) ([]FileInfo, bool)` et l'exposer à l'API via une petite interface `torrentFiles` (assertion de type, comme `PacedLimits`).

- [ ] **Step 1: Write the failing tests**

```go
// internal/config
func TestLibraryConfigDefaults(t *testing.T)      // valeurs par défaut de la spec
// internal/auth
func TestCurrentUserReadsSessionCookie(t *testing.T)
// internal/library
func TestRegisterRateLimitPerUser(t *testing.T)   // 21e nouvelle copie dans l'heure → ErrRateLimited ; même clé répétée ne compte pas
func TestRegisterRetriesAfterFreeingSpace(t *testing.T)
// internal/api (httptest + chi)
func TestLibraryPostRequiresLogin(t *testing.T)            // 401
func TestLibraryPostValidatesLangEpisodeAndFile(t *testing.T) // lang "en" → 422 ; épisode absent → 422 ; torrent inconnu → 422 ; fichier non vidéo → 422
func TestLibraryPostCreatesThenTouches(t *testing.T)       // 201 puis 200
func TestLibraryGetListsReadyCopiesOnly(t *testing.T)      // DOWNLOADING absente, AV1 présente avec stream_id de 40 hex
func TestStreamRawServesLibraryCopy(t *testing.T)          // GET /api/v1/stream/raw?ih=<stream_id>&file_idx=0 avec Range → 206
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./internal/config/ ./internal/auth/ ./internal/library/ ./internal/api/ -run 'Test(Library|CurrentUser|Register|StreamRawServesLibrary)' -v`
Expected: FAIL

- [ ] **Step 3: Implement config, `CurrentUser`, `Files`, `service.go`, handlers and wiring**

- [ ] **Step 4: Run the full backend suite**

Run: `go test -tags=nosqlite ./... && go vet -tags=nosqlite ./... && gofmt -l internal cmd`
Expected: PASS hors `TestRanking`/`TestTargetedFrenchSources` ; vet propre ; gofmt ne liste que `internal/playback/index.go` et `manager.go` (préexistants)

- [ ] **Step 5: Commit**

```bash
git add internal cmd && git commit -m "Wire the AV1 library into the server and expose its API"
```

---

### Task 9: Commande `gazes-library`

**Files:**
- Create: `cmd/library/main.go`
- Modify: `deploy/backend.Dockerfile` (build l.8, `COPY` après l.15, `mkdir` des dossiers `library-index library-pool`), `Makefile` (cible `build`, l.46-47)
- Test: `cmd/library/main_test.go`

**Interfaces:**
- Consumes: `OpenStore`, `Store.List`, `Store.EncoderStatus`, `NewPool` + `SysStatFS`, `Store.Delete` + `Pool.Path`
- Produces : binaire `gazes-library` sur le modèle de `cmd/logs/main.go` (flags, `signal.NotifyContext`, erreurs sur stderr + `os.Exit(1)`), dossiers par défaut `$LIBRARY_INDEX_DIR` et `$LIBRARY_POOL_DIR`. Sous-commandes :
  - `status [--json]` : copies par état ; par disque étiquette, `disk_id`, capacité, libre, réserve, utilisé, présent ; encodage en cours (clé, progression, en pause) et nombre en attente ; 5 dernières erreurs ; espace gagné par l'AV1
  - `list [--state S] [--season ID] [--json]`
  - `delete <season> <ep> <lang>` : refuse une copie `DOWNLOADING` ou `ENCODING` ; supprime fichier puis entrée

- [ ] **Step 1: Write the failing tests** (fonction `run(args []string, stdout, stderr io.Writer) int` testée directement)

```go
func TestStatusReportsCountsDisksAndSavings(t *testing.T) // index et pool dans t.TempDir() ; économie = Σ original − Σ taille des AV1
func TestListFiltersByState(t *testing.T)
func TestDeleteRefusesEncodingAndRemovesFile(t *testing.T)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=nosqlite ./cmd/library/ -v`
Expected: FAIL

- [ ] **Step 3: Implement `cmd/library/main.go`, Dockerfile and Makefile changes**

- [ ] **Step 4: Run tests and build the image**

Run: `go test -tags=nosqlite ./cmd/library/ -v && docker build -f deploy/backend.Dockerfile -t gazes-backend:library-test . && docker run --rm --entrypoint gazes-library gazes-backend:library-test status --json`
Expected: PASS ; build OK ; `status` affiche un JSON vide valide (aucun disque)

- [ ] **Step 5: Commit**

```bash
git add cmd/library deploy/backend.Dockerfile Makefile && git commit -m "Add the gazes-library admin command"
```

---

### Task 10: Déploiement : volumes, disques étiquetés, script hôte

**Files:**
- Modify: `compose.yaml` (service `backend`, volumes l.56-105 ; liste des volumes l.151)
- Create: `deploy/library/install-host.sh`, `deploy/library/99-gazes-library.rules`, `deploy/library/gazes-library-mount@.service`, `deploy/library/gazes-library-shared.service`, `deploy/library/mount-disk.sh`, `deploy/library/label-disk.sh`
- Modify: `Makefile` (cibles `library-install-host`, `library-label-disk`), `README.md` (section « Bibliothèque AV1 »)
- Test: `deploy/library/test.sh` (shellcheck + `bash -n` + essai à blanc)

**Interfaces:**
- `compose.yaml` backend :
  - volume long `type: bind`, `source: /mnt/gazes`, `target: /app/library-pool`, `bind: {propagation: rshared, create_host_path: true}`
  - `library-index:/app/library-index` ; ajout de `library-index` aux volumes nommés
  - environnement : `LIBRARY_POOL_DIR: /app/library-pool`, `LIBRARY_INDEX_DIR: /app/library-index`
- `install-host.sh` (sudo, idempotent, `--dry-run` qui affiche les actions sans rien écrire) :
  - crée `/mnt/gazes/internal`, `chown 10001:10001`, écrit le repère `.gazes-library` (`{"disk_id":"<uuid>"}`) s'il manque
  - installe `gazes-library-shared.service` (`mount --bind /mnt/gazes /mnt/gazes` puis `mount --make-rshared /mnt/gazes`, `Before=docker.service`) et l'active
  - installe la règle udev : `ACTION=="add", SUBSYSTEM=="block", ENV{ID_FS_LABEL}=="GAZES*", TAG+="systemd", ENV{SYSTEMD_WANTS}+="gazes-library-mount@%E{ID_FS_LABEL}.service"`
  - installe `gazes-library-mount@.service` qui appelle `mount-disk.sh %i` et, `ExecStop`, démonte
- `mount-disk.sh <LABEL>` : `/dev/disk/by-label/<LABEL>` → `/mnt/gazes/<LABEL>` ; options `nofail,noatime` + `uid=10001,gid=10001` pour exfat/ntfs/vfat ; ext4/xfs/btrfs → `chown 10001:10001` de la racine ; crée le repère s'il manque
- `label-disk.sh DEV LABEL` : refuse un `LABEL` qui ne commence pas par `GAZES` ; pose l'étiquette selon le système de fichiers (`e2label`, `exfatlabel`, `ntfslabel`) sans formater ; si la partition n'a pas de système de fichiers, propose `mkfs.ext4 -L LABEL DEV` seulement après avoir tapé `FORMAT <DEV>` en confirmation
- Makefile : `library-install-host` → `sudo deploy/library/install-host.sh` ; `library-label-disk` → `sudo deploy/library/label-disk.sh $(DEV) $(LABEL)`

- [ ] **Step 1: Write the failing check script `deploy/library/test.sh`**

`shellcheck deploy/library/*.sh` (si disponible), `bash -n` de chaque script, `install-host.sh --dry-run --root <tmpdir>` qui doit lister : création du dossier interne, du repère, des 3 unités et de la règle ; `label-disk.sh /dev/null OTHER` doit sortir en erreur « label must start with GAZES ».

- [ ] **Step 2: Run it to verify it fails**

Run: `bash deploy/library/test.sh`
Expected: FAIL (scripts absents)

- [ ] **Step 3: Write the scripts, units, rule, compose and Makefile changes, README section**

`--root` préfixe tous les chemins écrits (tests à blanc).

- [ ] **Step 4: Verify**

Run: `bash deploy/library/test.sh && docker compose -f compose.yaml config --quiet`
Expected: PASS ; compose valide

- [ ] **Step 5: Commit**

```bash
git add compose.yaml deploy/library Makefile README.md && git commit -m "Mount GAZES-labelled disks into the library pool"
```

---

### Task 11: Frontend : lecture depuis la bibliothèque et enregistrement

**Files:**
- Create: `web/src/lib/library.ts`
- Modify: `web/src/lib/api.ts` (fonctions `getLibraryCopies`, `registerLibraryCopy`), `web/src/types/api.ts` (`EpisodeSource.library?`), `web/src/components/AutoEpisodePlayer.tsx` (~l.50-65, l.114), `web/src/components/VideoPlayerModal.tsx` (~l.205, l.486-496, nouvelle prop), `web/package.json` (script `test:library`)
- Test: `web/scripts/library.test.mjs`

**Interfaces:**
- `types/api.ts` : `EpisodeSource.library?: { stream_id: string; lang: 'vf' | 'vostfr'; video_codec: string; duration_ms: number }`
- `api.ts` :
  - `getLibraryCopies(seasonId: number, episode: number, signal?: AbortSignal): Promise<LibraryCopy[]>` (`GET /library/episodes/{season}/{ep}` ; erreur réseau → `[]`)
  - `registerLibraryCopy(seasonId: number, episode: number, lang: 'vf'|'vostfr', body: {info_hash: string; file_index: number; release_name: string; anime_id: number; title: string}): Promise<void>` (erreurs ignorées)
  - `type LibraryCopy = { lang: 'vf'|'vostfr'; state: 'ORIGINAL'|'AV1'; video_codec: string; stream_id: string; duration_ms: number; audio_tracks: number; subtitle_tracks: number }`
- `library.ts` (fonctions pures) :
  - `libraryLang(source: EpisodeSource): 'vf' | 'vostfr' | null` (`VF`/`MULTI` → `vf`, `VOSTFR` → `vostfr`, sinon `null`)
  - `AV1_MIME = 'video/mp4; codecs="av01.0.08M.10"'`
  - `canPlayCopy(copy: LibraryCopy, isTypeSupported: (mime: string) => boolean): boolean` (`video_codec !== 'av1'` → `true`)
  - `librarySource(copy: LibraryCopy, base: Partial<EpisodeSource>): EpisodeSource` (`info_hash = stream_id`, `magnet_uri = ''`, `language_tag = 'VF'|'VOSTFR'`, `is_french`, `library` rempli, `seeders` élevé pour l'affichage)
  - `withLibraryCandidates(ranked: EpisodeSource[], copies: LibraryCopy[], isTypeSupported): EpisodeSource[]` : chaque copie lisible est insérée juste avant la première source classée de la même langue (`libraryLang`) ; si aucune source de cette langue, ajoutée en fin ; copies non lisibles ignorées
- `VideoPlayerModal` :
  - si `item.library` : pas de `loadTorrent`, `loadData` synthétique `{info_hash: stream_id, files: [{index:0, path:'episode.mkv', length:0, is_video:true}], main_video_index:0}`, `selectedFileIdx = 0`, pas de recherche de fichier d'épisode
  - nouvelle prop `onFileResolved?: (infoHash: string, fileIndex: number) => void`, appelée une seule fois au premier événement `playing` pour une source non-bibliothèque
  - erreur de lecture d'une source bibliothèque (`404 library_miss` ou échec média) → `reportFailure` habituel : `AutoEpisodePlayer` passe au candidat suivant (torrent)
- `AutoEpisodePlayer` :
  - au montage, `getLibraryCopies(seasonId, episodeNumber)` avec 1,5 s de délai max avant de lancer la 1re tentative ; candidats = `withLibraryCandidates(classement existant, copies, MediaSource.isTypeSupported)`
  - `onFileResolved` → si `useAuth().user` et `libraryLang(source)` non nul → `registerLibraryCopy(...)`

- [ ] **Step 1: Write the failing tests** (`node --experimental-strip-types`, `node:test`, import direct de `../src/lib/library.ts`)

```js
test('libraryLang maps VF and MULTI to vf, VOSTFR to vostfr, others to null')
test('canPlayCopy rejects av1 when unsupported, accepts original copies')
test('withLibraryCandidates puts a vf copy before the first VF torrent source')
test('withLibraryCandidates keeps a VF torrent ahead of a cached vostfr copy')   // langue différente → torrent d'abord
test('withLibraryCandidates drops unplayable av1 copies')                         // vieux Safari → torrent
test('librarySource uses stream_id as info_hash and has no magnet')
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && node --experimental-strip-types scripts/library.test.mjs`
Expected: FAIL

- [ ] **Step 3: Implement `library.ts`, API functions, types, component changes, `test:library` script**

- [ ] **Step 4: Run web checks**

Run: `cd web && npm ci && npm run test:library && npm run test:files && npm run test:sources && npx tsc --noEmit && npm run lint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web && git commit -m "Play episodes from the AV1 library before falling back to torrents"
```

---

### Task 12: Vérification de bout en bout (dev)

**Files:** aucun fichier produit (rapport uniquement)

- [ ] **Step 1: Démarrer l'environnement de dev** : `make dev` avec un dossier de pool temporaire monté (`LIBRARY_POOL_DIR` pointant sur un dossier contenant un repère créé par `EnsureMarker`) et `LIBRARY_ENCODE_THREADS=4`.
- [ ] **Step 2: Lire un épisode court** connecté avec un compte de test ; vérifier `gazes-library status` : entrée `DOWNLOADING` puis `ORIGINAL`.
- [ ] **Step 3: Attendre l'encodage** : `ENCODING` puis `AV1`, original supprimé, économie affichée.
- [ ] **Step 4: Relire l'épisode** : les logs ne montrent aucune requête indexeur ni `torrent/load` ; `GET /api/v1/stream/raw?ih=<stream_id>` répond 206 ; seek et sous-titres OK en mode legacy et HLS.
- [ ] **Step 5: Retirer le repère du disque** : copie `UNAVAILABLE`, lecture repasse par le torrent ; remettre le repère : copie de retour.
- [ ] **Step 6: Rapport** : captures des sorties `status`, temps de démarrage local vs torrent, anomalies.

package torrent

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/cache"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	anacrolixTorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// ClientEngine implements Engine using anacrolix/torrent.
type ClientEngine struct {
	cfg        EngineConfig
	client     *anacrolixTorrent.Client
	logger     *slog.Logger
	mu         sync.RWMutex
	torrents   map[string]*anacrolixTorrent.Torrent
	prevStats  map[string]statSnapshot
	schedulers map[string]*pieceScheduler
	lastUsed   map[string]time.Time
	pinned     map[string]map[int]bool // infohash -> pinned file indexes
	verified   sync.Map                // "infohash/file" -> struct{}: payload sampled once per process
	metainfo   *cache.MetainfoStore
	stop       chan struct{}
}

type statSnapshot struct {
	timestamp      time.Time
	completedBytes int64
}

// NewClientEngine initializes and starts the BitTorrent client engine.
func NewClientEngine(cfg EngineConfig, logger *slog.Logger) (*ClientEngine, error) {
	clientConfig := anacrolixTorrent.NewDefaultClientConfig()
	clientConfig.DataDir = cfg.DataDir
	clientConfig.MetainfoSourcesClient = &http.Client{Timeout: 8 * time.Second}
	// Allow uploading when EnableTitForTat or Seed is enabled so peers reciprocate with full download bandwidth
	clientConfig.NoUpload = !cfg.EnableTitForTat && !cfg.Seed
	clientConfig.DisableUTP = cfg.DisableUTP
	clientConfig.DisableTCP = cfg.DisableTCP
	clientConfig.DisablePEX = false
	clientConfig.NoDHT = false
	clientConfig.PeriodicallyAnnounceTorrentsToDht = true

	if cfg.EstablishedConnsPerTorrent > 0 {
		clientConfig.EstablishedConnsPerTorrent = cfg.EstablishedConnsPerTorrent
	}
	if cfg.HalfOpenConnsPerTorrent > 0 {
		clientConfig.HalfOpenConnsPerTorrent = cfg.HalfOpenConnsPerTorrent
	}

	// Optimize throughput and balance CPU usage
	clientConfig.TotalHalfOpenConns = 250
	clientConfig.TorrentPeersHighWater = 1000
	clientConfig.TorrentPeersLowWater = 100
	clientConfig.DisableIPv6 = false
	clientConfig.NoDefaultPortForwarding = false // UPnP / NAT-PMP when the host network allows it
	clientConfig.NominalDialTimeout = 5 * time.Second
	clientConfig.MinDialTimeout = 2 * time.Second
	clientConfig.HandshakesTimeout = 4 * time.Second
	clientConfig.DisableAcceptRateLimiting = true
	clientConfig.DialForPeerConns = true
	clientConfig.AcceptPeerConnections = true
	clientConfig.AlwaysWantConns = true
	clientConfig.PieceHashersPerTorrent = 2
	clientConfig.HeaderObfuscationPolicy = anacrolixTorrent.HeaderObfuscationPolicy{
		Preferred:        true,
		RequirePreferred: false,
	}

	// Set listen port (prefer UDPPort if ListenPort is unset)
	listenPort := cfg.ListenPort
	if listenPort == 0 && cfg.PreferUDP && cfg.UDPPort > 0 {
		listenPort = cfg.UDPPort
	}
	if listenPort > 0 {
		clientConfig.ListenPort = listenPort
	}

	client, err := anacrolixTorrent.NewClient(clientConfig)
	if err != nil && listenPort > 0 {
		// Fallback to random available port if preferred port was in use
		logger.Warn("preferred port occupied, falling back to auto-assigned port", "port", listenPort, "err", err)
		clientConfig.ListenPort = 0
		client, err = anacrolixTorrent.NewClient(clientConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create bittorrent client: %w", err)
	}

	engine := &ClientEngine{
		cfg:       cfg,
		client:    client,
		logger:    logger,
		torrents:  make(map[string]*anacrolixTorrent.Torrent),
		prevStats: make(map[string]statSnapshot),
		lastUsed:  make(map[string]time.Time),
		stop:      make(chan struct{}),
	}
	if cfg.MetainfoDir != "" {
		if store, storeErr := cache.NewMetainfoStore(cfg.MetainfoDir); storeErr != nil {
			logger.Warn("metainfo cache disabled", "err", storeErr)
		} else {
			engine.metainfo = store
			go store.Prune(30 * 24 * time.Hour)
		}
	}
	go engine.trackerStatusLoop()
	if cfg.CacheMaxBytes > 0 {
		go engine.evictionLoop()
	}

	logger.Info("bittorrent client engine initialized",
		"data_dir", cfg.DataDir,
		"readahead_mb", cfg.DefaultReadaheadBytes/(1024*1024),
		"conns_per_torrent", cfg.EstablishedConnsPerTorrent,
		"lookahead_pieces", cfg.LookaheadPieceCount,
		"tit_for_tat", cfg.EnableTitForTat,
		"listen_port", client.LocalPort(),
		"cache_max_gb", cfg.CacheMaxBytes>>30,
	)

	return engine, nil
}

// fetchMetainfo downloads the .torrent of a magnet marked "xs=gazes:<provider>" from
// that provider, and caches it so a restart does not repeat the request.
func (e *ClientEngine) fetchMetainfo(ctx context.Context, magnet metainfo.Magnet) *metainfo.MetaInfo {
	provider, ok := strings.CutPrefix(magnet.Params.Get("xs"), "gazes:")
	fetch := e.cfg.MetainfoFetchers[provider]
	if !ok || fetch == nil {
		return nil
	}
	infoHash := magnet.InfoHash.HexString()
	fctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	mi, err := fetch(fctx, infoHash)
	if err != nil || mi == nil {
		if err != nil {
			diagnostics.Logger(ctx, e.logger).Warn("torrent.metainfo_fetch_failed", "infohash", infoHash, "provider", provider, "err", diagnostics.Redact(err.Error()))
		}
		return nil
	}
	if e.metainfo != nil {
		if saveErr := e.metainfo.Save(infoHash, mi); saveErr != nil {
			e.logger.Warn("metainfo cache write failed", "infohash", infoHash, "err", saveErr)
		}
	}
	return mi
}

// AddTorrent resolves the file list without downloading video data.
// GetFileStream schedules headers only for the file selected by the viewer.
func (e *ClientEngine) AddTorrent(ctx context.Context, magnetURI string) (string, []FileInfo, error) {
	magnet, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		return "", nil, fmt.Errorf("invalid magnet uri: %w", err)
	}
	var cached *metainfo.MetaInfo
	if e.metainfo != nil {
		cached = e.metainfo.Load(magnet.InfoHash.HexString())
	}
	if cached == nil {
		cached = e.fetchMetainfo(ctx, magnet)
	}
	private := cached != nil && isPrivate(cached)
	var t *anacrolixTorrent.Torrent
	if cached != nil {
		// Known source: skip the DHT/tracker metadata exchange entirely.
		t, err = e.client.AddTorrent(cached)
	} else {
		t, err = e.client.AddMagnet(magnetURI)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid magnet uri: %w", err)
	}

	// Add fast public tracker tiers to ensure high seeder connectivity. A private
	// torrent's peers are only on its own tracker.
	if len(e.cfg.DefaultTrackers) > 0 && !private {
		trackerTiers := make([][]string, len(e.cfg.DefaultTrackers))
		for i, tr := range e.cfg.DefaultTrackers {
			trackerTiers[i] = []string{tr}
		}
		t.AddTrackers(trackerTiers)
	}

	infoHash := strings.ToLower(t.InfoHash().HexString())
	diagnostics.Logger(ctx, e.logger).Debug("torrent.metadata_wait", "infohash", infoHash)

	e.mu.Lock()
	e.torrents[infoHash] = t
	e.markUsedLocked(infoHash)
	e.mu.Unlock()

	// Wait for metadata resolution with context support
	select {
	case <-ctx.Done():
		diagnostics.Logger(ctx, e.logger).Warn("torrent.metadata_failed", "infohash", infoHash, "err", ctx.Err())
		return "", nil, ctx.Err()
	case <-t.GotInfo():
	}
	if cached == nil && e.metainfo != nil {
		mi := t.Metainfo()
		if saveErr := e.metainfo.Save(infoHash, &mi); saveErr != nil {
			e.logger.Warn("metainfo cache write failed", "infohash", infoHash, "err", saveErr)
		}
	}

	files := t.Files()
	fileInfos := make([]FileInfo, len(files))

	for i, f := range files {
		path := f.DisplayPath()
		isVideo := IsVideoFile(path)
		mime := DetectMimeType(path)

		fileInfos[i] = FileInfo{
			Index:    i,
			Path:     path,
			Length:   f.Length(),
			IsVideo:  isVideo,
			MimeType: mime,
		}

	}

	return infoHash, fileInfos, nil
}

// GetFileStream returns a responsive sequential reader for streaming a specific file.
func (e *ClientEngine) GetFileStream(ctx context.Context, infoHash string, fileIndex int) (io.ReadSeekCloser, *FileInfo, error) {
	infoHash = strings.ToLower(infoHash)
	diagnostics.Logger(ctx, e.logger).Debug("torrent.file_requested", "infohash", infoHash, "file_index", fileIndex)
	t, ok := e.getTorrent(infoHash)
	if !ok {
		return nil, nil, fmt.Errorf("torrent with infohash %s not found", infoHash)
	}
	e.touch(infoHash)

	// Ensure metadata is ready
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-t.GotInfo():
	}

	files := t.Files()
	if fileIndex < 0 || fileIndex >= len(files) {
		return nil, nil, fmt.Errorf("file index %d out of range (total %d)", fileIndex, len(files))
	}

	targetFile := files[fileIndex]
	e.verifyPayloadOnce(ctx, infoHash, fileIndex, t, targetFile)
	path := targetFile.DisplayPath()
	info := FileInfo{
		Index:    fileIndex,
		Path:     path,
		Length:   targetFile.Length(),
		IsVideo:  IsVideoFile(path),
		MimeType: DetectMimeType(path),
	}

	// Share priority leases across readers so cancellation releases only this
	// viewer's windows, including when multiple viewers watch the same pack.
	e.mu.Lock()
	if e.schedulers == nil {
		e.schedulers = make(map[string]*pieceScheduler)
	}
	scheduler := e.schedulers[infoHash]
	if scheduler == nil {
		scheduler = newPieceScheduler(t)
		e.schedulers[infoHash] = scheduler
	}
	e.mu.Unlock()
	lookahead := e.cfg.LookaheadPieceCount
	if pieceLength := t.Info().PieceLength; pieceLength > 0 && e.cfg.DefaultReadaheadBytes > 0 {
		byteWindow := int((e.cfg.DefaultReadaheadBytes + pieceLength - 1) / pieceLength)
		if lookahead <= 0 || lookahead > byteWindow {
			lookahead = byteWindow
		}
	}

	reader := NewSequentialFileReader(
		ctx, scheduler,
		targetFile,
		info,
		e.cfg.DefaultReadaheadBytes,
		lookahead,
		e.cfg.HeaderPrefetchPieces,
	)
	return reader, &info, nil
}

// GetStats returns real-time swarm telemetry.
func (e *ClientEngine) GetStats(infoHash string) (*SwarmStats, error) {
	t, ok := e.getTorrent(infoHash)
	if !ok {
		return nil, fmt.Errorf("torrent %s not found", infoHash)
	}
	e.touch(infoHash)

	completed := t.BytesCompleted()
	total := t.Length()
	var progress float64
	if total > 0 {
		progress = (float64(completed) / float64(total)) * 100.0
	}

	// Calculate instantaneous download rate
	e.mu.Lock()
	prev, exists := e.prevStats[infoHash]
	now := time.Now()
	var downloadRate int64
	if exists && now.After(prev.timestamp) {
		elapsedSec := now.Sub(prev.timestamp).Seconds()
		if elapsedSec > 0 {
			diffBytes := completed - prev.completedBytes
			if diffBytes > 0 {
				downloadRate = int64(float64(diffBytes) / elapsedSec)
			}
		}
	}
	e.prevStats[infoHash] = statSnapshot{timestamp: now, completedBytes: completed}
	e.mu.Unlock()

	stats := t.Stats()
	activePeers := stats.ActivePeers
	totalPeers := stats.TotalPeers

	title := t.Name()
	if title == "" {
		title = infoHash
	}

	return &SwarmStats{
		InfoHash:       infoHash,
		Title:          title,
		TotalBytes:     total,
		CompletedBytes: completed,
		ProgressPct:    progress,
		DownloadRate:   downloadRate,
		UploadRate:     0,
		TotalPeers:     totalPeers,
		ActiveSeeders:  activePeers,
	}, nil
}

// Close gracefully stops all active torrents and closes the client.
func (e *ClientEngine) Close() error {
	e.logger.Info("shutting down bittorrent engine")
	if e.stop != nil {
		select {
		case <-e.stop:
		default:
			close(e.stop)
		}
	}
	e.client.Close()
	return nil
}

func (e *ClientEngine) getTorrent(infoHash string) (*anacrolixTorrent.Torrent, bool) {
	e.mu.RLock()
	t, ok := e.torrents[strings.ToLower(infoHash)]
	e.mu.RUnlock()
	if ok {
		return t, true
	}

	// Fallback lookup directly in client
	h := metainfo.NewHashFromHex(infoHash)
	if ct, found := e.client.Torrent(h); found {
		e.mu.Lock()
		e.torrents[strings.ToLower(infoHash)] = ct
		e.mu.Unlock()
		return ct, true
	}

	return nil, false
}

// Verify interface compliance
var _ Engine = (*ClientEngine)(nil)

func (e *ClientEngine) touch(infoHash string) {
	e.mu.Lock()
	e.markUsedLocked(strings.ToLower(infoHash))
	e.mu.Unlock()
}

func (e *ClientEngine) markUsedLocked(infoHash string) {
	if e.lastUsed == nil {
		e.lastUsed = make(map[string]time.Time)
	}
	e.lastUsed[infoHash] = time.Now()
}

// evictionLoop keeps the on-disk payload under CacheMaxBytes by dropping the
// least recently used torrents that no viewer is reading.
func (e *ClientEngine) evictionLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.evictIdle()
		}
	}
}

func (e *ClientEngine) evictIdle() {
	e.mu.RLock()
	entries := make([]cacheEntry, 0, len(e.torrents))
	for infoHash, t := range e.torrents {
		if t.Info() == nil {
			continue
		}
		active := false
		if scheduler := e.schedulers[infoHash]; scheduler != nil {
			scheduler.mu.Lock()
			active = len(scheduler.windows) > 0
			scheduler.mu.Unlock()
		}
		entries = append(entries, cacheEntry{infoHash: infoHash, bytes: t.BytesCompleted(), lastUsed: e.lastUsed[infoHash], active: active, pinned: len(e.pinned[infoHash]) > 0})
	}
	e.mu.RUnlock()
	for _, infoHash := range pickEvictions(entries, e.cfg.CacheMaxBytes, e.cfg.CacheIdleTTL, time.Now()) {
		e.dropTorrent(infoHash)
	}
}

func (e *ClientEngine) dropTorrent(infoHash string) {
	e.mu.Lock()
	t := e.torrents[infoHash]
	delete(e.torrents, infoHash)
	delete(e.prevStats, infoHash)
	delete(e.schedulers, infoHash)
	delete(e.lastUsed, infoHash)
	delete(e.pinned, infoHash)
	e.mu.Unlock()
	if t == nil || t.Info() == nil {
		return
	}
	name := t.Info().BestName()
	// Clear completion state first so a later re-add re-downloads instead of trusting deleted data.
	for i := 0; i < t.NumPieces(); i++ {
		_ = t.Piece(i).Storage().MarkNotComplete()
	}
	t.Drop()
	if err := removeTorrentData(e.cfg.DataDir, name); err != nil && !os.IsNotExist(err) {
		e.logger.Warn("cache eviction failed", "infohash", infoHash, "err", err)
		return
	}
	e.logger.Info("cache.evicted", "infohash", infoHash, "name", name)
}

// verifyPayloadOnce guards against payloads deleted or zeroed while the completion database still
// calls their pieces complete (they would otherwise stream as zeros and break every probe).
func (e *ClientEngine) verifyPayloadOnce(ctx context.Context, infoHash string, fileIndex int, t *anacrolixTorrent.Torrent, f *anacrolixTorrent.File) {
	key := fmt.Sprintf("%s/%d", infoHash, fileIndex)
	if _, loaded := e.verified.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	vctx, cancel := context.WithTimeout(ctx, fileVerifyTimeout)
	defer cancel()
	rejected, err := verifyFilePieces(vctx, t, f)
	if err != nil {
		e.verified.Delete(key) // retry next open
		diagnostics.Logger(ctx, e.logger).Warn("torrent.payload_verify_failed", "infohash", infoHash, "file_index", fileIndex, "err", err)
		return
	}
	if rejected > 0 {
		diagnostics.Logger(ctx, e.logger).Warn("torrent.payload_invalid", "infohash", infoHash, "file_index", fileIndex, "pieces_redownloading", rejected)
		return
	}
	diagnostics.Logger(ctx, e.logger).Debug("torrent.payload_verified", "infohash", infoHash, "file_index", fileIndex)
}

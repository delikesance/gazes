package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/donations"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/api"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/nyaa"
	"github.com/gazes/gazes/internal/indexer/settings"
	"github.com/gazes/gazes/internal/kv"
	"github.com/gazes/gazes/internal/library"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
	"github.com/gazes/gazes/internal/vpn"
)

func main() {
	// 1. Load configuration
	cfg := config.Load()

	// Structured diagnostics use the same sanitized events in all sinks.
	logStore, err := diagnostics.Open(diagnostics.FromEnv(cfg.LogLevel))
	if err != nil {
		fmt.Fprintln(os.Stderr, "diagnostics initialization failed")
		os.Exit(1)
	}
	logger := slog.New(logStore.Handler())
	slog.SetDefault(logger)
	log.SetOutput(diagnostics.StandardWriter{})
	log.SetFlags(0)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = logStore.Close(ctx)
	}()

	logger.Info("starting gazes server",
		"env", cfg.AppEnv,
		"port", cfg.Port,
		"host", cfg.Host,
	)

	// 3. Ensure required directories exist
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		logger.Error("failed to create data dir", "err", err, "path", cfg.DataDir)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		logger.Error("failed to create cache dir", "err", err, "path", cfg.CacheDir)
	}

	// Nyaa remains independent of the gateway, including during Prowlarr outages.
	providers := []indexer.Provider{nyaa.NewClient(nyaa.DefaultBaseURL, nil), nyaa.NewSukebeiClient(nyaa.SukebeiBaseURL, nil)}
	extraProviders, err := settings.Providers(os.Getenv)
	if err != nil {
		logger.Error("invalid indexer configuration", "err", err)
		os.Exit(1)
	}
	providers = append(providers, extraProviders...)
	catalogIndexers := indexer.NewMultiProvider(providers...)

	// 5. Initialize BitTorrent Engine
	torrentCfg := torrent.DefaultEngineConfig(cfg.DataDir)
	torrentCfg.ListenPort = cfg.TorrentPort
	torrentCfg.Tunneled = cfg.VPNControlURL != ""
	torrentCfg.CacheMaxBytes = cfg.TorrentCacheMaxBytes
	torrentCfg.MetainfoDir = filepath.Join(cfg.CacheDir, "metainfo")
	if cfg.C411APIKey != "" {
		// C411 names a torrent by its infohash; its .torrent holds the private announce URL.
		torrentCfg.MetainfoFetchers = map[string]torrent.MetainfoFetcher{
			"c411": torrent.URLTemplateFetcher(nil, "https://c411.org/api?t=get&id={infohash}&apikey="+cfg.C411APIKey),
		}
	}
	torrentEngine, err := torrent.NewClientEngine(torrentCfg, logger)
	if err != nil {
		logger.Error("failed to initialize bittorrent engine", "err", err)
		os.Exit(1)
	}
	defer torrentEngine.Close()

	// Behind the gluetun sidecar: rotate the exit IP when AniList throttles it, and on a timer.
	if rotator := vpn.New(cfg.VPNControlURL, cfg.VPNRotateMinGap); rotator.Enabled() {
		rotate := func(ctx context.Context) error {
			err := rotator.Rotate(ctx)
			switch {
			case err == nil:
				logger.Info("vpn exit ip rotated")
			case !errors.Is(err, vpn.ErrCooldown):
				logger.Warn("vpn rotation failed", "err", err)
			}
			return err
		}
		metadata.SetAnilistRotator(rotate)
		if cfg.VPNRotateEvery > 0 {
			// A reset cuts every peer connection: postpone the timer while someone streams or downloads,
			// but not forever (maxDeferred ticks), so the IP still changes on a busy server.
			go func() {
				const maxDeferred = 4
				deferred := 0
				for range time.Tick(cfg.VPNRotateEvery) {
					if torrentEngine.ActiveReaders() > 0 && deferred < maxDeferred {
						deferred++
						continue
					}
					deferred = 0
					_ = rotate(context.Background())
				}
			}()
		}
	}

	// 6. Initialize Media Streaming Pipeline
	streamPipeline := stream.NewPipelineManager(logger)

	// Redis is required: caches, upstream rate limits and auth state are shared through it.
	if cfg.RedisURL == "" {
		logger.Error("REDIS_URL is required (see compose.redis.yaml)")
		os.Exit(1)
	}
	redisCtx, redisCancel := context.WithTimeout(context.Background(), 70*time.Second)
	redisClient, err := kv.Open(redisCtx, cfg.RedisURL, cfg.RedisNamespace, 60*time.Second)
	redisCancel()
	if err != nil {
		logger.Error("redis unavailable", "err", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	// The catalog survives Redis restarts on disk; without it the caches simply run on Redis alone.
	// Production only: the dev stack shares Redis and the AniList budget with production, so it must
	// neither run a second import nor elect itself leader of the production one.
	if cfg.AppEnv == "production" {
		if durable, err := kv.OpenDurable(filepath.Join(cfg.CatalogDir, "catalog.sqlite")); err != nil {
			logger.Error("durable catalog store unavailable, caches run on Redis only", "err", err, "dir", cfg.CatalogDir)
		} else {
			defer durable.Close()
			redisClient.SetDurable(durable)
		}
	}
	catalogIndexers.SetRedis(redisClient)

	// Every AniList answer is kept on disk for good: the catalog grows into our own database.
	if err := metadata.OpenAnilistStore(filepath.Join(cfg.AccountsDir, "anilist.sqlite")); err != nil {
		logger.Warn("anilist store unavailable, answers are only cached in Redis", "err", err)
	} else {
		defer metadata.CloseAnilistStore()
	}

	// 7. Initialize API server
	proxy := auth.NewProxyTrust(cfg.TrustProxy, cfg.TrustedProxies)
	if proxy.Open() {
		// Any peer that can reach the backend can then forge X-Forwarded-For (its rate-limit identity)
		// or omit it (counted as the web server's own render).
		logger.Warn("TRUST_PROXY is set without TRUSTED_PROXIES: forwarded headers are believed from any peer; set TRUSTED_PROXIES to the web service")
	}
	accounts, err := auth.New(auth.Options{Dir: cfg.AccountsDir, Production: cfg.AppEnv == "production", Proxy: proxy, Getenv: os.Getenv, State: auth.NewRedisState(redisClient)})
	if err != nil {
		logger.Error("failed to initialize accounts", "err", err)
		os.Exit(1)
	}
	defer accounts.Close()

	// The AV1 library wraps the torrent engine; any failure to open it leaves the plain engine in place.
	var engine torrent.Engine = torrentEngine
	serverOpts := []api.Option{api.WithAuth(accounts), api.WithRedis(redisClient), api.WithProxyTrust(proxy)}
	libraryCtx, libraryCancel := context.WithCancel(context.Background())
	defer libraryCancel()
	if cfg.LibraryEnabled {
		libraryService, err := library.Open(library.Options{
			PoolDir:        cfg.LibraryPoolDir,
			IndexDir:       cfg.LibraryIndexDir,
			FFmpeg:         "ffmpeg",
			FFprobe:        "ffprobe",
			ReservePercent: cfg.LibraryReservePercent,
			ReserveBytes:   cfg.LibraryReserveBytes,
			Stall:          cfg.LibraryStallTimeout,
			MinViewers:     cfg.LibraryMinViewers,
			Encode: library.EncodeSettings{
				Preset:       cfg.LibraryEncodePreset,
				CRF:          cfg.LibraryEncodeCRF,
				Threads:      cfg.LibraryEncodeThreads,
				PauseStreams: cfg.LibraryEncodePauseStreams,
				Window:       cfg.LibraryEncodeWindow,
			},
		}, torrentEngine, torrentEngine, logger)
		if err != nil {
			diagnostics.Log(libraryCtx, slog.LevelError, "library.disabled", "error", err.Error())
		} else {
			// Stop the workers before the torrent engine they read from closes.
			defer libraryService.Close()
			libraryService.Start(libraryCtx)
			engine = libraryService.Engine()
			serverOpts = append(serverOpts, api.WithLibrary(libraryService))
		}
	}

	// The admin panel is optional: any failure here leaves streaming untouched.
	adminSvc, stopAdmin := startAdmin(cfg, logger, redisClient)
	if adminSvc != nil {
		defer stopAdmin()
		serverOpts = append(serverOpts, api.WithAdmin(adminSvc))
		serverOpts = append(serverOpts, api.WithMCP())
		// Best-effort playback error recording (bounded queue, never blocks a playback).
		errorRecorder := admin.NewErrorRecorder(adminSvc.Store())
		defer errorRecorder.Close()
		serverOpts = append(serverOpts, api.WithErrorSink(errorRecorder))
	}

	// Donations are optional too: without their database the page is simply off.
	if donationStore, err := donations.Open(cfg.AccountsDir); err != nil {
		logger.Error("donations disabled: cannot open donations database", "err", err)
	} else {
		defer donationStore.Close()
		accounts.SetOnAccountDeleted(func(ctx context.Context, userID int64) {
			if err := donationStore.UnlinkUser(ctx, userID); err != nil {
				logger.Warn("unlink donations of a deleted account", "err", err)
			}
		})
		goal := int64(0)
		if cfg.DonationGoalEUR != nil {
			goal = int64(*cfg.DonationGoalEUR * 100)
		}
		serverOpts = append(serverOpts, api.WithDonations(donations.NewService(donationStore, donations.Config{
			SiteURL: cfg.SiteURL, BTCPayURL: cfg.BTCPayURL, BTCPayStoreID: cfg.BTCPayStoreID, BTCPayAPIKey: cfg.BTCPayAPIKey,
			BTCPayWebhookSecret: cfg.BTCPayWebhookSecret, KofiURL: cfg.KofiURL, KofiToken: cfg.KofiToken, GoalCents: goal,
		})))
		if adminSvc != nil {
			adminSvc.SetDonations(donationStore)
		}
	}

	server := api.NewServer(cfg, logger, catalogIndexers, engine, streamPipeline, serverOpts...)
	defer server.ClosePlayback()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.StreamTimeout, // Allow long streaming sessions
		IdleTimeout:  60 * time.Second,
	}

	// 8. Run server in background goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// 9. Graceful shutdown on SIGINT / SIGTERM
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("server error encountered", "err", err)
	case sig := <-shutdown:
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("error during server shutdown", "err", err)
			_ = httpServer.Close()
		}
		logger.Info("gazes server gracefully stopped")
	}
}

// startAdmin opens the admin database, builds the admin service and runs the metrics rollup
// (Redis-elected so one replica per period does the work) until the returned stop func is
// called. It returns a nil service when the admin database cannot be opened.
func startAdmin(cfg *config.Config, logger *slog.Logger, redis *kv.Client) (*admin.Service, func()) {
	store, err := admin.Open(cfg.AdminDBPath)
	if err != nil {
		logger.Error("admin disabled: cannot open admin database", "err", err, "path", cfg.AdminDBPath)
		return nil, nil
	}
	svc, err := admin.NewService(store, filepath.Join(cfg.AccountsDir, "accounts.sqlite"))
	if err != nil {
		logger.Error("admin disabled: cannot open accounts database read-only", "err", err)
		store.Close()
		return nil, nil
	}
	elect := func(name string, period time.Duration) func(context.Context) bool {
		if redis == nil {
			return nil
		}
		return func(ctx context.Context) bool { return redis.Elect(ctx, name, period) }
	}
	svc.SetWatchConfig(admin.WatchConfig{WebhookURL: cfg.WatchWebhookURL, WebhookSecret: cfg.WatchWebhookSecret, DiskPath: cfg.WatchDiskPath})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		svc.RunWatch(ctx, admin.DefaultWatchInterval, elect("admin-watch", admin.DefaultWatchInterval-10*time.Second))
	}()
	go func() {
		defer close(done)
		if e := elect("admin-backfill", time.Hour); e == nil || e(ctx) {
			if err := svc.BackfillIfEmpty(ctx, 30); err != nil && ctx.Err() == nil {
				logger.Error("admin initial backfill failed", "err", err)
			}
		}
		svc.Rollup().RunElected(ctx, admin.DefaultRollupInterval, nil, elect("admin-rollup", admin.DefaultRollupInterval-time.Minute))
	}()
	return svc, func() {
		cancel()
		<-done
		<-watchDone
		svc.Close()
		store.Close()
	}
}

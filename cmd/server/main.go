package main

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gazes/gazes/internal/api"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/nyaa"
	"github.com/gazes/gazes/internal/indexer/settings"
	"github.com/gazes/gazes/internal/kv"
	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
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
	providers := []indexer.Provider{nyaa.NewClient(nyaa.DefaultBaseURL, nil)}
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
	catalogIndexers.SetRedis(redisClient)

	// 7. Initialize API server
	accounts, err := auth.New(auth.Options{Dir: cfg.AccountsDir, Production: cfg.AppEnv == "production", TrustProxy: cfg.TrustProxy, Getenv: os.Getenv, State: auth.NewRedisState(redisClient)})
	if err != nil {
		logger.Error("failed to initialize accounts", "err", err)
		os.Exit(1)
	}
	defer accounts.Close()

	server := api.NewServer(cfg, logger, catalogIndexers, torrentEngine, streamPipeline, api.WithAuth(accounts), api.WithRedis(redisClient))
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

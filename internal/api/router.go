package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server represents the API server.
type Server struct {
	cfg             *config.Config
	logger          *slog.Logger
	indexer         indexer.Provider
	torrentEngine   torrent.Engine
	streamPipeline  stream.Pipeline
	analyzer        metadata.Analyzer
	animeService    *metadata.AnimeService
	catalogService  *metadata.AnimeCatalogService
	episodeResolver *indexer.EpisodeResolver
	router          chi.Router
	diagnosticRate  diagnosticLimiter
	auth            *auth.Service
}

// Option customises a Server.
type Option func(*Server)

// WithAuth enables the account routes.
func WithAuth(svc *auth.Service) Option { return func(s *Server) { s.auth = svc } }

// NewServer initializes a new Server instance.
func NewServer(
	cfg *config.Config,
	logger *slog.Logger,
	idx indexer.Provider,
	torEngine torrent.Engine,
	pipeline stream.Pipeline,
	opts ...Option,
) *Server {
	s := &Server{
		cfg:             cfg,
		logger:          logger,
		indexer:         idx,
		torrentEngine:   torEngine,
		streamPipeline:  pipeline,
		analyzer:        metadata.NewFFprobeAnalyzer(logger),
		animeService:    metadata.NewAnimeService(nil),
		catalogService:  metadata.NewAnimeCatalogService(nil),
		episodeResolver: indexer.NewEpisodeResolver(idx),
	}

	for _, opt := range opts {
		opt(s)
	}
	s.setupRoutes()
	return s
}

// Router returns the initialized http.Handler.
func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Middlewares
	r.Use(middleware.RequestID)
	r.Use(s.diagnosticContext)
	r.Use(s.diagnosticRecover)

	// CORS Configuration for P2P and web clients
	if s.cfg.EnableCORS {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Range", "Origin", "X-Playback-Session-ID", "X-Playback-Attempt-ID", "X-Request-ID", "X-Playback-Anime-ID", "X-Playback-Season-ID", "X-Playback-Episode"},
			ExposedHeaders:   []string{"Content-Length", "Content-Range", "Accept-Ranges", "Content-Type", "X-Request-ID", "X-Playback-Session-ID"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// Liveness Probe
	r.Get("/healthz", s.HandleHealthz)

	// API v1 Routes
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", s.HandleHealth)
		api.Post("/diagnostics/events", s.HandleDiagnosticEvents)
		api.Get("/search", s.HandleSearch)
		api.Get("/latest", s.HandleLatest)

		if s.auth != nil {
			api.Get("/auth/kem", s.auth.Kem)
			api.Get("/auth/captcha", s.auth.CaptchaChallenge)
			api.Post("/auth/register", s.auth.Register)
			api.Post("/auth/login", s.auth.Login)
			api.Post("/auth/logout", s.auth.Logout)
			api.Get("/auth/me", s.auth.Me)
			api.Get("/me/progress", s.auth.GetProgress)
			api.Put("/me/progress", s.auth.PutProgress)
		}

		// Catalog & Episode Discovery
		api.Route("/catalog", func(cat chi.Router) {
			cat.Get("/trending", s.HandleCatalogTrending)
			cat.Get("/seasonal", s.HandleCatalogSeasonal)
			cat.Get("/popular", s.HandleCatalogPopular)
			cat.Get("/schedule", s.HandleCatalogSchedule)
			cat.Get("/search", s.HandleCatalogSearch)
			cat.Get("/anime/{id}/franchise", s.HandleFranchise)
			cat.Get("/anime/{id}/seasons/{season}", s.HandleSeason)
			cat.Get("/anime/{id}/seasons/{season}/episodes/{ep}/sources", s.HandleSeasonSources)
			cat.Get("/anime/{id}", s.HandleCatalogAnimeDetail)
			cat.Get("/anime/{id}/episodes/{ep}/sources", s.HandleEpisodeSources)
		})

		// Torrent Engine Routes
		api.Post("/torrent/load", s.HandleLoadTorrent)
		api.Get("/torrent/stats", s.HandleTorrentStats)
		api.Get("/metadata", s.HandleMetadata)

		// Video Streaming & Subtitles Routes
		api.Get("/stream", s.HandleStream)
		api.Get("/stream/raw", s.HandleStreamRaw)
		api.Get("/subtitles", s.HandleSubtitles)
	})

	s.router = r
}

func contextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

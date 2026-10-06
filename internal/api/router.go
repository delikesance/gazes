package api

import (
	"context"
	"github.com/gazes/gazes/internal/kv"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/library"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/playback"
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
	party           *partyHub
	episodeResolver indexer.EpisodeSourceResolver
	router          chi.Router
	diagnosticRate  rateLimiter
	routeLimits     rateLimiter
	proxy           *auth.ProxyTrust
	auth            *auth.Service
	kv              *kv.Client
	sourceCache     *sourceCache
	sourceCacheOnce sync.Once
	previewStore    *previewStore
	previewOnce     sync.Once
	playback        *playback.Manager
	playbackOnce    sync.Once
	subtitles       subtitleJobs
	library         *library.Service
	libraryUser     func(*http.Request) (int64, bool)
	admin           *admin.Service
	mcpEnabled      bool
	mcpOrigins      []string
	errorSink       admin.ErrorSink
}

// Option customises a Server.
type Option func(*Server)

type unavailableEpisodeResolver struct{ err error }

func (r unavailableEpisodeResolver) ResolveSeasonSources(context.Context, indexer.EpisodeIdentity) (*indexer.EpisodeSourcesResponse, error) {
	return nil, r.err
}
func (r unavailableEpisodeResolver) ResolvePlaybackSources(context.Context, indexer.EpisodeIdentity) (*indexer.EpisodeSourcesResponse, error) {
	return nil, r.err
}

// WithProxyTrust makes per-client rate limits read the client IP from X-Forwarded-For when the
// request comes from a trusted proxy (the web server, relaying the edge's header).
func WithProxyTrust(p *auth.ProxyTrust) Option { return func(s *Server) { s.proxy = p } }

// WithAuth enables the account routes.
func WithAuth(svc *auth.Service) Option { return func(s *Server) { s.auth = svc } }

// WithLibrary enables the AV1 episode library routes.
func WithLibrary(svc *library.Service) Option { return func(s *Server) { s.library = svc } }

// WithLibraryUser overrides how the library routes identify the caller (default: the auth session). Tests only.
func WithLibraryUser(fn func(*http.Request) (int64, bool)) Option {
	return func(s *Server) { s.libraryUser = fn }
}

// WithRedis moves every shared cache, upstream rate limit and single-flight to Redis, so all
// instances behave like one polite client of AniList and the indexers.
func WithRedis(c *kv.Client) Option {
	return func(s *Server) {
		s.kv = c
		s.catalogService.SetRedis(c)
		s.animeService.SetRedis(c)
		s.startWarmer()
	}
}

// NewServer initializes a new Server instance.
func NewServer(
	cfg *config.Config,
	logger *slog.Logger,
	idx indexer.Provider,
	torEngine torrent.Engine,
	pipeline stream.Pipeline,
	opts ...Option,
) *Server {
	fallbackResolver := indexer.NewEpisodeResolver(idx)
	fallbackResolver.SetFastPhaseTimeout(cfg.ResolverFastPhaseTimeout)
	var episodeResolver indexer.EpisodeSourceResolver = fallbackResolver
	if cfg.ArrAuthoritative {
		resolver, err := indexer.NewArrEpisodeResolver(cfg.SonarrURL, cfg.SonarrAPIKey, cfg.SonarrSeriesMap, cfg.RadarrURL, cfg.RadarrAPIKey, cfg.RadarrMovieMap, nil)
		if err != nil {
			episodeResolver = unavailableEpisodeResolver{err: err}
		} else {
			episodeResolver = resolver
		}
	}
	s := &Server{
		cfg:             cfg,
		logger:          logger,
		indexer:         idx,
		torrentEngine:   torEngine,
		streamPipeline:  pipeline,
		analyzer:        metadata.NewFFprobeAnalyzer(logger),
		animeService:    metadata.NewAnimeService(nil),
		catalogService:  metadata.NewAnimeCatalogService(nil),
		party:           newPartyHub(),
		episodeResolver: episodeResolver,
	}

	for _, opt := range opts {
		opt(s)
	}
	s.party.clientIP = s.clientKey
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

	// Off by default: the web app is same-origin. Opt in with ENABLE_CORS, ideally with CORS_ALLOWED_ORIGINS.
	if s.cfg.EnableCORS {
		origins := s.cfg.CORSAllowedOrigins
		if len(origins) == 0 {
			origins = []string{"*"}
		}
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Range", "Origin", "X-Playback-Session-ID", "X-Playback-Attempt-ID", "X-Request-ID", "X-Playback-Anime-ID", "X-Playback-Season-ID", "X-Playback-Episode"},
			ExposedHeaders:   []string{"Content-Length", "Content-Range", "Accept-Ranges", "Content-Type", "X-Request-ID", "X-Playback-Session-ID"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// Liveness Probe
	r.Get("/healthz", s.HandleHealthz)

	s.mountAdmin(r)
	s.mountMCP(r)

	// API v1 Routes
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", s.HandleHealth)
		api.Post("/diagnostics/events", s.HandleDiagnosticEvents)
		api.Get("/diagnostics/cache", s.HandleCacheDiagnostics)
		api.With(s.rateLimit("search", 60, time.Minute)).Get("/search", s.HandleSearch)
		api.Get("/latest", s.HandleLatest)
		api.Get("/party/{room}/events", s.party.handleEvents)
		api.With(s.rateLimit("party", 240, time.Minute)).Post("/party/{room}/events", s.party.handlePost)

		if s.auth != nil {
			api.Get("/auth/kem", s.auth.Kem)
			api.Get("/auth/captcha", s.auth.CaptchaChallenge)
			api.Post("/auth/register", s.auth.Register)
			api.Post("/auth/login", s.auth.Login)
			api.Post("/auth/logout", s.auth.Logout)
			api.Get("/auth/me", s.auth.Me)
			api.Get("/me/progress", s.auth.GetProgress)
			api.Put("/me/progress", s.auth.PutProgress)
			api.Get("/me/history", s.auth.GetHistory)
			api.Put("/me/history", s.auth.PutHistory)
			api.Delete("/me/history", s.auth.DeleteHistory)
			api.Get("/me/hidden", s.auth.GetHidden)
			api.Put("/me/hidden", s.auth.PutHidden)
			api.Get("/me/watchlist", s.auth.GetWatchlist)
			api.Put("/me/watchlist", s.auth.PutWatchlist)
		}

		// Catalog & Episode Discovery
		api.Route("/catalog", func(cat chi.Router) {
			cat.Get("/trending", s.HandleCatalogTrending)
			cat.Get("/seasonal", s.HandleCatalogSeasonal)
			cat.Get("/popular", s.HandleCatalogPopular)
			cat.With(s.rateLimit("foryou", 30, time.Minute)).Get("/foryou", s.HandleCatalogForYou)
			cat.With(s.rateLimit("foryou", 30, time.Minute)).Post("/foryou", s.HandleCatalogForYou)
			cat.Get("/schedule", s.HandleCatalogSchedule)
			cat.With(s.rateLimit("catalog-search", 60, time.Minute)).Get("/search", s.HandleCatalogSearch)
			cat.Get("/anime/{id}/franchise", s.HandleFranchise)
			cat.Get("/anime/{id}/seasons/{season}", s.HandleSeason)
			cat.Get("/anime/{id}/seasons/{season}/episodes/{ep}/sources", s.HandleSeasonSources)
			cat.Get("/seasons/{season}/episodes/{ep}/preview", s.HandleEpisodePreview)
			cat.Post("/seasons/{season}/episodes/{ep}/preview", s.HandleEpisodePreviewCreate)
			cat.Get("/seasons/{season}/episodes/{ep}/skip-times", s.HandleSkipTimes)
			cat.Get("/anime/{id}", s.HandleCatalogAnimeDetail)
			cat.Get("/anime/{id}/episodes/{ep}/sources", s.HandleEpisodeSources)
		})

		if s.library != nil {
			api.Post("/library/episodes/{season}/{ep}/{lang}", s.HandleLibraryRegister)
			api.Get("/library/episodes/{season}/{ep}", s.HandleLibraryCopies)
		}

		// Torrent Engine Routes
		api.With(s.rateLimit("torrent-load", 30, time.Minute)).Post("/torrent/load", s.HandleLoadTorrent)
		api.Get("/torrent/stats", s.HandleTorrentStats)
		api.Get("/metadata", s.HandleMetadata)

		// Video Streaming & Subtitles Routes
		api.Get("/stream", s.HandleStream)
		api.With(s.rateLimit("stream-raw", 120, time.Minute)).Get("/stream/raw", s.HandleStreamRaw)
		api.With(s.rateLimit("subtitles", 60, time.Minute)).Get("/subtitles", s.HandleSubtitles)
		api.Get("/playback/config", s.HandlePlaybackConfig)
		api.Post("/playback/sessions", s.HandlePlaybackCreate)
		api.Put("/playback/sessions/{session}", s.HandlePlaybackUpdate)
		api.Delete("/playback/sessions/{session}", s.HandlePlaybackDelete)
		api.Get("/playback/sessions/{session}/index.m3u8", s.HandlePlaybackPlaylist)
		api.Get("/playback/sessions/{session}/{segment}/{asset}", s.HandlePlaybackMedia)
	})

	s.router = r
}

func contextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

// sources returns the episode-findings cache, creating it for Servers built without NewServer.
func (s *Server) sources() *sourceCache {
	s.sourceCacheOnce.Do(func() {
		if s.sourceCache == nil {
			s.sourceCache = newSourceCache(s.kv)
		}
	})
	return s.sourceCache
}

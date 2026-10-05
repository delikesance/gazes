package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration options for Gazes.
type Config struct {
	PlaybackEngine       string        `json:"playback_engine"`
	PlaybackMemoryBytes  int64         `json:"playback_memory_bytes"`
	PlaybackDiskBytes    int64         `json:"playback_disk_bytes"`
	AppEnv               string        `json:"app_env"`
	Host                 string        `json:"host"`
	Port                 int           `json:"port"`
	DataDir              string        `json:"data_dir"`
	CacheDir             string        `json:"cache_dir"`
	LogLevel             string        `json:"log_level"`
	EnableCORS           bool          `json:"enable_cors"`
	StreamTimeout        time.Duration `json:"stream_timeout"`
	MaxMemoryCache       int64         `json:"max_memory_cache_bytes"`
	TorrentPort          int           `json:"torrent_port"`
	TorrentCacheMaxBytes int64         `json:"torrent_cache_max_bytes"`
	AccountsDir          string        `json:"accounts_dir"`
	AdminDBPath          string        `json:"admin_db_path"`
	TrustProxy           bool          `json:"trust_proxy"`
	// In authoritative mode, explicit AniList -> *Arr bindings replace all local
	// title matching. API keys and bindings are intentionally never serialized.
	ArrAuthoritative bool   `json:"arr_authoritative"`
	SonarrURL        string `json:"sonarr_url"`
	SonarrAPIKey     string `json:"-"`
	SonarrSeriesMap  string `json:"-"`
	RadarrURL        string `json:"radarr_url"`
	RadarrAPIKey     string `json:"-"`
	// C411APIKey lets the backend fetch C411 .torrent files for the VF fallback.
	C411APIKey     string `json:"-"`
	RadarrMovieMap string `json:"-"`
	// RedisURL is required: it holds the caches, upstream rate limits and auth state shared by every instance.
	RedisURL       string `json:"-"`
	RedisNamespace string `json:"redis_namespace"`
	// ResolverFastPhaseTimeout bounds the first (shared season-pack) round of a playback resolve.
	ResolverFastPhaseTimeout time.Duration `json:"resolver_fast_phase_timeout"`

	// AV1 episode library (see docs/superpowers/specs/2026-10-04-av1-episode-library-design.md).
	LibraryEnabled            bool          `json:"library_enabled"`
	LibraryPoolDir            string        `json:"library_pool_dir"`
	LibraryIndexDir           string        `json:"library_index_dir"`
	LibraryEncodeWindow       string        `json:"library_encode_window"`
	LibraryReservePercent     int           `json:"library_reserve_percent"`
	LibraryEncodePreset       int           `json:"library_encode_preset"`
	LibraryEncodeCRF          int           `json:"library_encode_crf"`
	LibraryEncodeThreads      int           `json:"library_encode_threads"`
	LibraryEncodePauseStreams int           `json:"library_encode_pause_streams"`
	LibraryReserveBytes       int64         `json:"library_reserve_bytes"`
	LibraryStallTimeout       time.Duration `json:"library_stall_timeout"`
}

// Load loads configuration from environment variables with fallback defaults.
func Load() *Config {
	return &Config{
		PlaybackEngine:           getEnv("PLAYBACK_ENGINE", "legacy"),
		PlaybackMemoryBytes:      getEnvInt64("PLAYBACK_MEMORY_BYTES", 64<<20),
		PlaybackDiskBytes:        getEnvInt64("PLAYBACK_DISK_BYTES", 1<<30),
		AppEnv:                   getEnv("APP_ENV", "development"),
		Host:                     getEnv("HOST", "0.0.0.0"),
		Port:                     getEnvInt("PORT", 8090),
		DataDir:                  getEnv("DATA_DIR", "./data"),
		CacheDir:                 getEnv("CACHE_DIR", "./cache"),
		LogLevel:                 getEnv("LOG_LEVEL", "debug"),
		EnableCORS:               getEnvBool("ENABLE_CORS", true),
		StreamTimeout:            getEnvDuration("STREAM_TIMEOUT", 30*time.Minute),
		MaxMemoryCache:           getEnvInt64("MAX_MEMORY_CACHE_BYTES", 256*1024*1024), // 256MB default
		TorrentPort:              getEnvInt("TORRENT_PORT", 42069),                     // publish this TCP+UDP port for inbound peers
		TorrentCacheMaxBytes:     getEnvInt64("TORRENT_CACHE_MAX_BYTES", 40<<30),       // 40 GiB of resident payload, LRU-evicted
		AccountsDir:              getEnv("ACCOUNTS_DIR", "./accounts"),
		AdminDBPath:              getEnv("GAZES_ADMIN_DB", filepath.Join(getEnv("ACCOUNTS_DIR", "./accounts"), "admin.sqlite")),
		TrustProxy:               getEnvBool("TRUST_PROXY", false), // honour X-Forwarded-* from the edge proxy
		ArrAuthoritative:         getEnvBool("ARR_AUTHORITATIVE", false),
		SonarrURL:                getEnv("SONARR_URL", ""),
		SonarrAPIKey:             getEnvOrFile("SONARR_API_KEY", "SONARR_API_KEY_FILE"),
		SonarrSeriesMap:          getEnv("SONARR_SERIES_MAP", ""),
		RadarrURL:                getEnv("RADARR_URL", ""),
		RadarrAPIKey:             getEnvOrFile("RADARR_API_KEY", "RADARR_API_KEY_FILE"),
		C411APIKey:               getEnvOrFile("C411_API_KEY", "C411_API_KEY_FILE"),
		RadarrMovieMap:           getEnv("RADARR_MOVIE_MAP", ""),
		RedisURL:                 getEnv("REDIS_URL", ""),
		RedisNamespace:           getEnv("REDIS_NAMESPACE", "gazes"), // isolates per-stack state (auth) on a shared Redis
		ResolverFastPhaseTimeout: getEnvDuration("RESOLVER_FAST_PHASE_TIMEOUT", 3*time.Second),

		LibraryEnabled:            getEnvBool("LIBRARY_ENABLED", true),
		LibraryPoolDir:            getEnv("LIBRARY_POOL_DIR", "/app/library-pool"),
		LibraryIndexDir:           getEnv("LIBRARY_INDEX_DIR", "/app/library-index"),
		LibraryEncodeWindow:       getEnv("LIBRARY_ENCODE_WINDOW", ""),
		LibraryReservePercent:     getEnvInt("LIBRARY_RESERVE_PERCENT", 10),
		LibraryReserveBytes:       getEnvInt64("LIBRARY_RESERVE_BYTES", 50_000_000_000),
		LibraryStallTimeout:       getEnvDuration("LIBRARY_STALL_TIMEOUT", 24*time.Hour),
		LibraryEncodePreset:       getEnvInt("LIBRARY_ENCODE_PRESET", 8),
		LibraryEncodeCRF:          getEnvInt("LIBRARY_ENCODE_CRF", 30),
		LibraryEncodeThreads:      getEnvInt("LIBRARY_ENCODE_THREADS", 8),
		LibraryEncodePauseStreams: getEnvInt("LIBRARY_ENCODE_PAUSE_STREAMS", 3),
	}
}

func getEnvOrFile(key, fileKey string) string {
	if value := getEnv(key, ""); value != "" {
		return value
	}
	path := getEnv(fileKey, "")
	if path == "" {
		return ""
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

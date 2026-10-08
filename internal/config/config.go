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
	CatalogDir           string        `json:"catalog_dir"` // CATALOG_DIR: durable copy of the AniList catalog (SQLite), kept across deploys
	LogLevel             string        `json:"log_level"`
	EnableCORS           bool          `json:"enable_cors"`
	CORSAllowedOrigins   []string      `json:"cors_allowed_origins"` // empty with EnableCORS = any origin
	StreamTimeout        time.Duration `json:"stream_timeout"`
	MaxMemoryCache       int64         `json:"max_memory_cache_bytes"`
	TorrentPort          int           `json:"torrent_port"`
	TorrentCacheMaxBytes int64         `json:"torrent_cache_max_bytes"`
	AccountsDir          string        `json:"accounts_dir"`
	AdminDBPath          string        `json:"admin_db_path"`
	WatchWebhookURL      string        `json:"-"`                         // GAZES_WATCH_WEBHOOK_URL: where watch events are POSTed (optional)
	WatchWebhookSecret   string        `json:"-"`                         // GAZES_WATCH_WEBHOOK_SECRET: HMAC key of the X-Gazes-Signature header (optional)
	WatchDiskPath        string        `json:"watch_disk_path,omitempty"` // GAZES_WATCH_DISK_PATH: directory whose volume the disk_pct rule measures (optional)
	VPNControlURL        string        `json:"-"`                         // VPN_CONTROL_URL: gluetun control server; empty = no VPN
	VPNRotateEvery       time.Duration `json:"-"`                         // VPN_ROTATE_EVERY: periodic exit-IP rotation; 0 disables
	VPNRotateMinGap      time.Duration `json:"-"`                         // VPN_ROTATE_MIN_GAP: shortest time between two rotations
	TrustProxy           bool          `json:"trust_proxy"`
	TrustedProxies       []string      `json:"trusted_proxies"` // with TrustProxy: the only peers believed; empty = any
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
	LibraryMinViewers         int           `json:"library_min_viewers"`

	// Optional cost inputs for the admin panel (nil = not provided, never defaulted).
	CostServerMonth       *float64 `json:"cost_server_month,omitempty"`         // GAZES_COST_SERVER_MONTH: server cost per month
	CostBandwidthPerGB    *float64 `json:"cost_bandwidth_per_gb,omitempty"`     // GAZES_COST_BANDWIDTH_PER_GB
	CostStoragePerGBMonth *float64 `json:"cost_storage_per_gb_month,omitempty"` // GAZES_COST_STORAGE_PER_GB_MONTH
	GBPerWatchHour        *float64 `json:"gb_per_watch_hour,omitempty"`         // GAZES_GB_PER_WATCH_HOUR: data served per hour watched

	// Donations (all optional; a provider with missing fields is off). Secrets also read GAZES_*_FILE.
	SiteURL             string   `json:"-"` // GAZES_SITE_URL: public origin, for the post-payment redirect
	BTCPayURL           string   `json:"-"` // GAZES_BTCPAY_URL
	BTCPayStoreID       string   `json:"-"` // GAZES_BTCPAY_STORE_ID
	BTCPayAPIKey        string   `json:"-"` // GAZES_BTCPAY_API_KEY
	BTCPayWebhookSecret string   `json:"-"` // GAZES_BTCPAY_WEBHOOK_SECRET
	KofiURL             string   `json:"-"` // GAZES_KOFI_URL: public Ko-fi page
	KofiToken           string   `json:"-"` // GAZES_KOFI_TOKEN: webhook verification token
	DonationGoalEUR     *float64 `json:"-"` // GAZES_DONATION_GOAL_EUR: monthly goal shown publicly
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
		CatalogDir:               getEnv("CATALOG_DIR", "./catalog"),
		LogLevel:                 getEnv("LOG_LEVEL", "debug"),
		EnableCORS:               getEnvBool("ENABLE_CORS", false), // the web app is same-origin through the Next rewrite
		CORSAllowedOrigins:       splitList(getEnv("CORS_ALLOWED_ORIGINS", "")),
		StreamTimeout:            getEnvDuration("STREAM_TIMEOUT", 30*time.Minute),
		MaxMemoryCache:           getEnvInt64("MAX_MEMORY_CACHE_BYTES", 256*1024*1024), // 256MB default
		TorrentPort:              getEnvInt("TORRENT_PORT", 42069),                     // publish this TCP+UDP port for inbound peers
		TorrentCacheMaxBytes:     getEnvInt64("TORRENT_CACHE_MAX_BYTES", 40<<30),       // 40 GiB of resident payload, LRU-evicted
		AccountsDir:              getEnv("ACCOUNTS_DIR", "./accounts"),
		AdminDBPath:              getEnv("GAZES_ADMIN_DB", filepath.Join(getEnv("ACCOUNTS_DIR", "./accounts"), "admin.sqlite")),
		WatchWebhookURL:          getEnv("GAZES_WATCH_WEBHOOK_URL", ""),
		WatchWebhookSecret:       getEnv("GAZES_WATCH_WEBHOOK_SECRET", ""),
		WatchDiskPath:            getEnv("GAZES_WATCH_DISK_PATH", ""),
		VPNControlURL:            getEnv("VPN_CONTROL_URL", ""),
		VPNRotateEvery:           getEnvDuration("VPN_ROTATE_EVERY", 30*time.Minute),
		VPNRotateMinGap:          getEnvDuration("VPN_ROTATE_MIN_GAP", 10*time.Minute),
		TrustProxy:               getEnvBool("TRUST_PROXY", false), // honour X-Forwarded-* from the edge proxy
		TrustedProxies:           splitList(getEnv("TRUSTED_PROXIES", "")),
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

		LibraryEnabled:        getEnvBool("LIBRARY_ENABLED", true),
		LibraryPoolDir:        getEnv("LIBRARY_POOL_DIR", "/app/library-pool"),
		LibraryIndexDir:       getEnv("LIBRARY_INDEX_DIR", "/app/library-index"),
		LibraryEncodeWindow:   getEnv("LIBRARY_ENCODE_WINDOW", ""),
		LibraryReservePercent: getEnvInt("LIBRARY_RESERVE_PERCENT", 10),
		LibraryReserveBytes:   getEnvInt64("LIBRARY_RESERVE_BYTES", 50_000_000_000),
		LibraryStallTimeout:   getEnvDuration("LIBRARY_STALL_TIMEOUT", 24*time.Hour),
		// Distinct users who must watch an episode before it is downloaded and AV1-encoded (1 = the first viewer).
		LibraryMinViewers:         getEnvInt("LIBRARY_MIN_VIEWERS", 2),
		LibraryEncodePreset:       getEnvInt("LIBRARY_ENCODE_PRESET", 8),
		LibraryEncodeCRF:          getEnvInt("LIBRARY_ENCODE_CRF", 30),
		LibraryEncodeThreads:      getEnvInt("LIBRARY_ENCODE_THREADS", 8),
		LibraryEncodePauseStreams: getEnvInt("LIBRARY_ENCODE_PAUSE_STREAMS", 3),

		CostServerMonth:       getEnvOptFloat("GAZES_COST_SERVER_MONTH"),
		CostBandwidthPerGB:    getEnvOptFloat("GAZES_COST_BANDWIDTH_PER_GB"),
		CostStoragePerGBMonth: getEnvOptFloat("GAZES_COST_STORAGE_PER_GB_MONTH"),
		GBPerWatchHour:        getEnvOptFloat("GAZES_GB_PER_WATCH_HOUR"),

		SiteURL:             getEnv("GAZES_SITE_URL", ""),
		BTCPayURL:           getEnv("GAZES_BTCPAY_URL", ""),
		BTCPayStoreID:       getEnv("GAZES_BTCPAY_STORE_ID", ""),
		BTCPayAPIKey:        getEnvOrFile("GAZES_BTCPAY_API_KEY", "GAZES_BTCPAY_API_KEY_FILE"),
		BTCPayWebhookSecret: getEnvOrFile("GAZES_BTCPAY_WEBHOOK_SECRET", "GAZES_BTCPAY_WEBHOOK_SECRET_FILE"),
		KofiURL:             getEnv("GAZES_KOFI_URL", ""),
		KofiToken:           getEnvOrFile("GAZES_KOFI_TOKEN", "GAZES_KOFI_TOKEN_FILE"),
		DonationGoalEUR:     getEnvOptFloat("GAZES_DONATION_GOAL_EUR"),
	}
}

// getEnvOptFloat reads an optional non-negative number; absent, empty, invalid or negative is nil.
func getEnvOptFloat(key string) *float64 {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil || f < 0 || f != f || f > 1e12 {
		return nil
	}
	return &f
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

// splitList splits a comma-separated env value, dropping blanks.
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

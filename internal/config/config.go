package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration options for Gazes.
type Config struct {
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
}

// Load loads configuration from environment variables with fallback defaults.
func Load() *Config {
	return &Config{
		AppEnv:               getEnv("APP_ENV", "development"),
		Host:                 getEnv("HOST", "0.0.0.0"),
		Port:                 getEnvInt("PORT", 8090),
		DataDir:              getEnv("DATA_DIR", "./data"),
		CacheDir:             getEnv("CACHE_DIR", "./cache"),
		LogLevel:             getEnv("LOG_LEVEL", "debug"),
		EnableCORS:           getEnvBool("ENABLE_CORS", true),
		StreamTimeout:        getEnvDuration("STREAM_TIMEOUT", 30*time.Minute),
		MaxMemoryCache:       getEnvInt64("MAX_MEMORY_CACHE_BYTES", 256*1024*1024), // 256MB default
		TorrentPort:          getEnvInt("TORRENT_PORT", 42069),                     // publish this TCP+UDP port for inbound peers
		TorrentCacheMaxBytes: getEnvInt64("TORRENT_CACHE_MAX_BYTES", 40<<30),       // 40 GiB of resident payload, LRU-evicted
	}
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

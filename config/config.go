package config

import (
	"log/slog"
	"os"
	"strconv"
)

const defaultDataPath = "data/ebpf_spans.json"

// Config holds application configuration.
type Config struct {
	DataPath    string
	HTTPPort    int    // REST API port, default 8080, env HTTP_PORT
	MetricsPort int    // Prometheus metrics port, default 9090, env METRICS_PORT
	LogLevel    string // "debug", "info", "warn", "error"; default "info", env LOG_LEVEL
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		DataPath:    defaultDataPath,
		HTTPPort:    8080,
		MetricsPort: 9090,
		LogLevel:    "info",
	}
}

// LoadConfig reads configuration from environment variables, falling back to defaults.
func LoadConfig() Config {
	cfg := DefaultConfig()
	if v := os.Getenv("DATA_PATH"); v != "" {
		cfg.DataPath = v
	}
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}
	if v := os.Getenv("METRICS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.MetricsPort = port
		}
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	return cfg
}

// SlogLevel converts the string log level to slog.Level.
func (c Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

package config

import "os"

const defaultDataPath = "data/ebpf_spans.json"

// Config holds application configuration.
type Config struct {
	DataPath string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{DataPath: defaultDataPath}
}

// LoadConfig reads configuration from environment variables, falling back to defaults.
func LoadConfig() Config {
	cfg := DefaultConfig()
	if v := os.Getenv("DATA_PATH"); v != "" {
		cfg.DataPath = v
	}
	return cfg
}

package config

import (
	"log/slog"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DataPath != "data/ebpf_spans.json" {
		t.Errorf("DataPath = %q, want %q", cfg.DataPath, "data/ebpf_spans.json")
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.MetricsPort != 9090 {
		t.Errorf("MetricsPort = %d, want 9090", cfg.MetricsPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	t.Setenv("DATA_PATH", "/tmp/spans.json")
	t.Setenv("HTTP_PORT", "3000")
	t.Setenv("METRICS_PORT", "9191")
	t.Setenv("LOG_LEVEL", "debug")

	cfg := LoadConfig()

	if cfg.DataPath != "/tmp/spans.json" {
		t.Errorf("DataPath = %q, want %q", cfg.DataPath, "/tmp/spans.json")
	}
	if cfg.HTTPPort != 3000 {
		t.Errorf("HTTPPort = %d, want 3000", cfg.HTTPPort)
	}
	if cfg.MetricsPort != 9191 {
		t.Errorf("MetricsPort = %d, want 9191", cfg.MetricsPort)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv("HTTP_PORT", "notanumber")
	cfg := LoadConfig()
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080 (default on invalid input)", cfg.HTTPPort)
	}
}

func TestSlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		cfg := Config{LogLevel: tt.level}
		if got := cfg.SlogLevel(); got != tt.want {
			t.Errorf("SlogLevel(%q) = %v, want %v", tt.level, got, tt.want)
		}
	}
}

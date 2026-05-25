package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const envPrefix = "FORZA_"

type Config struct {
	Log     LogConfig     `koanf:"log"`
	API     APIConfig     `koanf:"api"`
	Ingest  IngestConfig  `koanf:"ingest"`
	Storage StorageConfig `koanf:"storage"`
	Stream  StreamConfig  `koanf:"stream"`
}

type LogConfig struct {
	Level string `koanf:"level"`
}

type APIConfig struct {
	Addr string `koanf:"addr"`
}

type IngestConfig struct {
	Addr           string `koanf:"addr"`
	FH6CaptureLog  string `koanf:"fh6_capture_log"`
}

type StorageConfig struct {
	DataDir string `koanf:"data_dir"`
}

type StreamConfig struct {
	RingSize int `koanf:"ring_size"`
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(c.Log.Level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Load(explicitPath string) (*Config, error) {
	k := koanf.New(".")

	path := explicitPath
	if path == "" {
		var err error
		path, err = xdgPath("XDG_CONFIG_HOME", ".config", "forza-telemetry/config.toml")
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat config %s: %w", path, err)
	}

	envProvider := env.Provider(envPrefix, ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, envPrefix)), "__", ".")
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if err := applyDefaults(cfg); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.Storage.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	return cfg, nil
}

func applyDefaults(c *Config) error {
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.API.Addr == "" {
		c.API.Addr = ":8080"
	}
	if c.Ingest.Addr == "" {
		c.Ingest.Addr = ":7100"
	}
	if c.Storage.DataDir == "" {
		dir, err := xdgPath("XDG_DATA_HOME", ".local/share", "forza-telemetry")
		if err != nil {
			return err
		}
		c.Storage.DataDir = dir
	}
	if c.Ingest.FH6CaptureLog == "" {
		c.Ingest.FH6CaptureLog = filepath.Join(c.Storage.DataDir, "captures", "fh6.log")
	}
	if c.Stream.RingSize <= 0 {
		c.Stream.RingSize = 3600
	}
	return nil
}

func xdgPath(envVar, defaultRel, suffix string) (string, error) {
	base := os.Getenv(envVar)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		base = filepath.Join(home, defaultRel)
	}
	return filepath.Join(base, suffix), nil
}

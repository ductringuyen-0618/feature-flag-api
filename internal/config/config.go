package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	ReloadInterval time.Duration
	OverrideTTL    time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       getenv("REDIS_URL", "redis://127.0.0.1:6379/0"),
		ReloadInterval: 10 * time.Second,
		OverrideTTL:    time.Minute,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if v := os.Getenv("RELOAD_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("RELOAD_INTERVAL: %w", err)
		}
		cfg.ReloadInterval = d
	}
	if v := os.Getenv("OVERRIDE_CACHE_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("OVERRIDE_CACHE_TTL: %w", err)
		}
		cfg.OverrideTTL = d
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func MustInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	ReloadInterval time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       os.Getenv("REDIS_URL"),
		ReloadInterval: 10 * time.Second,
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
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

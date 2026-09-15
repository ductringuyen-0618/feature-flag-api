package config

import (
	"os"
	"testing"
)

func TestLoadUnsetRedisURLEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://flags:flags@127.0.0.1:5432/flags?sslmode=disable")
	orig, had := os.LookupEnv("REDIS_URL")
	os.Unsetenv("REDIS_URL")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("REDIS_URL", orig)
			return
		}
		_ = os.Unsetenv("REDIS_URL")
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisURL != "" {
		t.Fatalf("RedisURL=%q, want empty when REDIS_URL unset", cfg.RedisURL)
	}
}

func TestLoadExplicitEmptyRedisURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://flags:flags@127.0.0.1:5432/flags?sslmode=disable")
	t.Setenv("REDIS_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisURL != "" {
		t.Fatalf("RedisURL=%q, want empty when REDIS_URL is explicitly empty", cfg.RedisURL)
	}
}

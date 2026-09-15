package config

import (
	"os"
	"testing"
)

func TestLoadUnsetRedisURLIsEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://flags:flags@127.0.0.1:5432/flags?sslmode=disable")
	prev, had := os.LookupEnv("REDIS_URL")
	if err := os.Unsetenv("REDIS_URL"); err != nil {
		t.Fatalf("Unsetenv REDIS_URL: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("REDIS_URL", prev)
		} else {
			_ = os.Unsetenv("REDIS_URL")
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RedisURL != "" {
		t.Fatalf("RedisURL=%q, want empty when REDIS_URL is unset", cfg.RedisURL)
	}
}

func TestLoadEmptyRedisURLStaysEmpty(t *testing.T) {
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

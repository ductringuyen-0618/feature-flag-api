package cached

import (
	"context"
	"testing"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/store/memory"
)

func TestHealthOkWithoutRedis(t *testing.T) {
	svc := New(memory.New(), nil, time.Hour, false)
	if got := svc.Health(context.Background()); got != "ok" {
		t.Fatalf("Health=%q", got)
	}
}

func TestHealthDegradedWhenRedisWantedButMissing(t *testing.T) {
	svc := New(memory.New(), nil, time.Hour, true)
	if got := svc.Health(context.Background()); got != "degraded" {
		t.Fatalf("Health=%q", got)
	}
}

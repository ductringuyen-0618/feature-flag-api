package cached

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/memory"
)

type getFlagStub struct {
	store.FlagStore
	mu   sync.Mutex
	flag flag.Flag
}

func (s *getFlagStub) GetFlag(ctx context.Context, name string) (flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flag.Name == name {
		return s.flag, nil
	}
	return s.FlagStore.GetFlag(ctx, name)
}

func TestApplyInvalidationDoesNotStompNewerWriteThrough(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	stub := &getFlagStub{FlagStore: db}
	svc := New(stub, nil, time.Hour, false)
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	created, err := svc.CreateFlag(ctx, flag.Flag{Name: "kill-switch", Enabled: true, RolloutPercent: 100})
	if err != nil {
		t.Fatal(err)
	}

	off := false
	updated, err := svc.UpdateFlag(ctx, "kill-switch", &off, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatal("write-through left kill switch on")
	}

	stale := created
	stale.Enabled = true
	stub.mu.Lock()
	stub.flag = stale
	stub.mu.Unlock()

	svc.applyInvalidation(ctx, "kill-switch")

	got, err := svc.GetFlag(ctx, "kill-switch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("stale invalidation stomped kill switch: %+v", got)
	}
}

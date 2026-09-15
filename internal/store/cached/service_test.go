package cached_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/cached"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/memory"
)

func newService(t *testing.T, db store.FlagStore) *cached.Service {
	t.Helper()
	if db == nil {
		db = memory.New()
	}
	svc := cached.New(db, nil, time.Hour)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc
}

type listStub struct {
	store.FlagStore
	emptyList bool
}

func (s *listStub) ListFlags(ctx context.Context) ([]flag.Flag, error) {
	if s.emptyList {
		return nil, nil
	}
	return s.FlagStore.ListFlags(ctx)
}

func TestReloadKeepsSnapshotOnEmptyList(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	stub := &listStub{FlagStore: db}
	svc := newService(t, stub)

	if _, err := svc.CreateFlag(ctx, flag.Flag{Name: "keep-me", Enabled: true, RolloutPercent: 100}); err != nil {
		t.Fatal(err)
	}

	stub.emptyList = true
	if err := svc.Reload(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ListFlags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "keep-me" {
		t.Fatalf("snapshot after empty reload = %+v", got)
	}
}

func TestOverrideReadsFromStore(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	svc := newService(t, db)

	created, err := svc.CreateFlag(ctx, flag.Flag{Name: "checkout", Enabled: false, RolloutPercent: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetOverride(ctx, flag.Override{FlagName: created.Name, UserID: "alice", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetOverride(ctx, "checkout", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.UserID != "alice" {
		t.Fatalf("store override = %+v", got)
	}

	res, err := svc.Evaluate(ctx, "checkout", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Enabled || res.Reason != flag.ReasonOverride {
		t.Fatalf("eval = %+v", res)
	}

	if err := svc.DeleteOverride(ctx, "checkout", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetOverride(ctx, "checkout", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted override err = %v", err)
	}

	res, err = svc.Evaluate(ctx, "checkout", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if res.Enabled || res.Reason != flag.ReasonFlagDisabled {
		t.Fatalf("after delete eval = %+v", res)
	}
}

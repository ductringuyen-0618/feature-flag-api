package cached_test

import (
	"context"
	"errors"
	"sync"
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
	svc := cached.New(db, nil, time.Hour, false)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc
}

type listStub struct {
	store.FlagStore
	emptyList bool
	fixedList []flag.Flag
	useFixed  bool
}

func (s *listStub) ListFlags(ctx context.Context) ([]flag.Flag, error) {
	if s.useFixed {
		out := make([]flag.Flag, len(s.fixedList))
		copy(out, s.fixedList)
		return out, nil
	}
	if s.emptyList {
		return nil, nil
	}
	return s.FlagStore.ListFlags(ctx)
}

type getFlagStub struct {
	store.FlagStore
	fixed     flag.Flag
	useFixed  bool
	notFound  bool
	mu        sync.Mutex
}

func (s *getFlagStub) GetFlag(ctx context.Context, name string) (flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notFound {
		return flag.Flag{}, store.ErrNotFound
	}
	if s.useFixed {
		return s.fixed, nil
	}
	return s.FlagStore.GetFlag(ctx, name)
}

type blockingOverrideStub struct {
	store.FlagStore
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingOverrideStub) GetOverride(ctx context.Context, flagName, userID string) (flag.Override, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
		return flag.Override{FlagName: flagName, UserID: userID, Enabled: true}, nil
	case <-ctx.Done():
		return flag.Override{}, ctx.Err()
	}
}

func TestReloadClearsSnapshotOnEmptyList(t *testing.T) {
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
	if len(got) != 0 {
		t.Fatalf("snapshot after empty SoT reload = %+v, want cleared", got)
	}
}

func TestReloadDoesNotStompNewerWriteThrough(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	stub := &listStub{FlagStore: db}
	svc := newService(t, stub)

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := svc.CreateFlag(ctx, flag.Flag{
		Name:           "kill-switch",
		Enabled:        true,
		RolloutPercent: 100,
		CreatedAt:      old,
		UpdatedAt:      old,
	})
	if err != nil {
		t.Fatal(err)
	}

	off := false
	updated, err := svc.UpdateFlag(ctx, created.Name, &off, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatal("write-through should leave kill switch off")
	}

	// Late ListFlags result from before the kill-switch write.
	stale := created
	stale.Enabled = true
	stale.UpdatedAt = old
	stub.useFixed = true
	stub.fixedList = []flag.Flag{stale}

	if err := svc.Reload(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetFlag(ctx, "kill-switch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("stale reload stomped write-through: %+v", got)
	}

	res, err := svc.Evaluate(ctx, "kill-switch", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if res.Enabled {
		t.Fatalf("evaluate after stale reload = %+v, want kill switch off", res)
	}
}

func TestApplyInvalidationDoesNotStompNewerWriteThrough(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	stub := &getFlagStub{FlagStore: db}
	svc := cached.New(stub, nil, time.Hour, false)
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := svc.CreateFlag(ctx, flag.Flag{
		Name:           "kill-switch",
		Enabled:        true,
		RolloutPercent: 100,
		CreatedAt:      old,
		UpdatedAt:      old,
	})
	if err != nil {
		t.Fatal(err)
	}

	off := false
	if _, err := svc.UpdateFlag(ctx, created.Name, &off, nil, nil); err != nil {
		t.Fatal(err)
	}

	stale := created
	stale.Enabled = true
	stale.UpdatedAt = old
	stub.mu.Lock()
	stub.useFixed = true
	stub.fixed = stale
	stub.mu.Unlock()

	svc.ApplyInvalidationForTest(ctx, "kill-switch")

	got, err := svc.GetFlag(ctx, "kill-switch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("stale applyInvalidation stomped write-through: %+v", got)
	}
}

func TestGetOverrideLeaderCancelDoesNotFailFollower(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	stub := &blockingOverrideStub{
		FlagStore: db,
		started:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	svc := newService(t, stub)

	if _, err := svc.CreateFlag(ctx, flag.Flag{Name: "checkout", Enabled: true, RolloutPercent: 100}); err != nil {
		t.Fatal(err)
	}

	leaderCtx, cancel := context.WithCancel(context.Background())
	followerDone := make(chan error, 1)
	leaderDone := make(chan error, 1)

	go func() {
		_, err := svc.Evaluate(leaderCtx, "checkout", "alice")
		leaderDone <- err
	}()

	<-stub.started

	go func() {
		_, err := svc.Evaluate(context.Background(), "checkout", "alice")
		followerDone <- err
	}()

	// Give the follower time to join the singleflight wait.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-leaderDone:
	case <-time.After(2 * time.Second):
		t.Fatal("leader evaluate did not return after cancel")
	}

	close(stub.release)

	select {
	case err := <-followerDone:
		if err != nil {
			t.Fatalf("follower evaluate after leader cancel = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower evaluate timed out")
	}
}

func TestEvaluateMissesSnapshotWithoutDBFill(t *testing.T) {
	ctx := context.Background()
	db := memory.New()
	svc := cached.New(db, nil, time.Hour, false)

	if _, err := db.CreateFlag(ctx, flag.Flag{Name: "only-in-db", Enabled: true, RolloutPercent: 100}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Evaluate(ctx, "only-in-db", "alice")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("evaluate missing snapshot = %v", err)
	}

	got, err := svc.GetFlag(ctx, "only-in-db")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "only-in-db" {
		t.Fatalf("admin get = %+v", got)
	}

	_, err = svc.Evaluate(ctx, "only-in-db", "alice")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("evaluate after admin get = %v", err)
	}

	list, err := svc.ListFlags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("snapshot filled = %+v", list)
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

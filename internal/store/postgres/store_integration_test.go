//go:build integration

// Run: DATABASE_URL=postgres://... go test -tags=integration ./internal/store/postgres/...
package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store/postgres"
)

var testStore *postgres.Store

func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		os.Stderr.WriteString("skip: DATABASE_URL unset (integration tests need Postgres)\n")
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := postgres.New(ctx, url)
	if err != nil {
		os.Stderr.WriteString("postgres.New: " + err.Error() + "\n")
		os.Exit(1)
	}
	testStore = s
	code := m.Run()
	_ = s.Close()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	flags, err := testStore.ListFlags(ctx)
	if err != nil {
		t.Fatalf("ListFlags: %v", err)
	}
	for _, f := range flags {
		if err := testStore.DeleteFlag(ctx, f.Name); err != nil {
			t.Fatalf("DeleteFlag(%q): %v", f.Name, err)
		}
	}
}

func TestCreateGetListFlag(t *testing.T) {
	reset(t)
	ctx := context.Background()

	created, err := testStore.CreateFlag(ctx, flag.Flag{
		Name:           "checkout",
		Description:    "checkout flow",
		Enabled:        true,
		RolloutPercent: 25,
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	if created.Name != "checkout" || created.Description != "checkout flow" || !created.Enabled || created.RolloutPercent != 25 {
		t.Fatalf("CreateFlag row = %+v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("CreateFlag timestamps zero: created=%v updated=%v", created.CreatedAt, created.UpdatedAt)
	}

	got, err := testStore.GetFlag(ctx, "checkout")
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if got.Name != "checkout" || got.Description != "checkout flow" || !got.Enabled || got.RolloutPercent != 25 {
		t.Fatalf("GetFlag = %+v", got)
	}

	if _, err := testStore.CreateFlag(ctx, flag.Flag{Name: "alpha", Enabled: false, RolloutPercent: 0}); err != nil {
		t.Fatalf("CreateFlag alpha: %v", err)
	}
	list, err := testStore.ListFlags(ctx)
	if err != nil {
		t.Fatalf("ListFlags: %v", err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "checkout" {
		t.Fatalf("ListFlags order/contents = %+v", list)
	}
}

func TestCreateDuplicateReturnsAlreadyExists(t *testing.T) {
	reset(t)
	ctx := context.Background()
	in := flag.Flag{Name: "dup", Enabled: true, RolloutPercent: 100}
	if _, err := testStore.CreateFlag(ctx, in); err != nil {
		t.Fatalf("first CreateFlag: %v", err)
	}
	_, err := testStore.CreateFlag(ctx, in)
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate CreateFlag error = %v, want ErrAlreadyExists", err)
	}
}

func TestUpdateFlagCoalesce(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if _, err := testStore.CreateFlag(ctx, flag.Flag{
		Name:           "partial",
		Description:    "orig",
		Enabled:        false,
		RolloutPercent: 10,
	}); err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}

	enabled := true
	updated, err := testStore.UpdateFlag(ctx, "partial", &enabled, nil, nil)
	if err != nil {
		t.Fatalf("UpdateFlag enabled only: %v", err)
	}
	if !updated.Enabled || updated.Description != "orig" || updated.RolloutPercent != 10 {
		t.Fatalf("after enabled patch = %+v", updated)
	}

	desc := "next"
	updated, err = testStore.UpdateFlag(ctx, "partial", nil, &desc, nil)
	if err != nil {
		t.Fatalf("UpdateFlag description only: %v", err)
	}
	if !updated.Enabled || updated.Description != "next" || updated.RolloutPercent != 10 {
		t.Fatalf("after description patch = %+v", updated)
	}

	rollout := 80
	updated, err = testStore.UpdateFlag(ctx, "partial", nil, nil, &rollout)
	if err != nil {
		t.Fatalf("UpdateFlag rollout only: %v", err)
	}
	if !updated.Enabled || updated.Description != "next" || updated.RolloutPercent != 80 {
		t.Fatalf("after rollout patch = %+v", updated)
	}
}

func TestDeleteFlagCascadesOverrides(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if _, err := testStore.CreateFlag(ctx, flag.Flag{Name: "gone", Enabled: true, RolloutPercent: 100}); err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	if err := testStore.SetOverride(ctx, flag.Override{FlagName: "gone", UserID: "u1", Enabled: true}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := testStore.DeleteFlag(ctx, "gone"); err != nil {
		t.Fatalf("DeleteFlag: %v", err)
	}
	if _, err := testStore.GetFlag(ctx, "gone"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetFlag after delete = %v, want ErrNotFound", err)
	}
	if _, err := testStore.GetOverride(ctx, "gone", "u1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetOverride after cascade = %v, want ErrNotFound", err)
	}
}

func TestSetOverrideUnknownFlagReturnsNotFound(t *testing.T) {
	reset(t)
	ctx := context.Background()
	err := testStore.SetOverride(ctx, flag.Override{FlagName: "missing", UserID: "u1", Enabled: true})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetOverride unknown flag = %v, want ErrNotFound", err)
	}
}

func TestGetAndDeleteOverride(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if _, err := testStore.CreateFlag(ctx, flag.Flag{Name: "ov", Enabled: false, RolloutPercent: 0}); err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	if err := testStore.SetOverride(ctx, flag.Override{FlagName: "ov", UserID: "alice", Enabled: true}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	got, err := testStore.GetOverride(ctx, "ov", "alice")
	if err != nil {
		t.Fatalf("GetOverride: %v", err)
	}
	if got.FlagName != "ov" || got.UserID != "alice" || !got.Enabled {
		t.Fatalf("GetOverride = %+v", got)
	}
	if err := testStore.DeleteOverride(ctx, "ov", "alice"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	if _, err := testStore.GetOverride(ctx, "ov", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetOverride after delete = %v, want ErrNotFound", err)
	}
	if err := testStore.DeleteOverride(ctx, "ov", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteOverride missing = %v, want ErrNotFound", err)
	}
}

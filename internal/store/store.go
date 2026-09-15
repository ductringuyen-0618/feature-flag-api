package store

import (
	"context"
	"errors"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// FlagStore is the durable source of truth for flags and overrides.
type FlagStore interface {
	CreateFlag(ctx context.Context, f flag.Flag) (flag.Flag, error)
	GetFlag(ctx context.Context, name string) (flag.Flag, error)
	ListFlags(ctx context.Context) ([]flag.Flag, error)
	UpdateFlag(ctx context.Context, name string, enabled *bool, description *string, rollout *int) (flag.Flag, error)
	DeleteFlag(ctx context.Context, name string) error

	SetOverride(ctx context.Context, o flag.Override) error
	GetOverride(ctx context.Context, flagName, userID string) (flag.Override, error)
	DeleteOverride(ctx context.Context, flagName, userID string) error

	Ping(ctx context.Context) error
	Close() error
}

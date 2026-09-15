package memory

import (
	"context"
	"sync"
	"time"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
)

type Store struct {
	mu        sync.Mutex
	flags     map[string]flag.Flag
	overrides map[string]flag.Override // flag\x00user
}

func New() *Store {
	return &Store{
		flags:     map[string]flag.Flag{},
		overrides: map[string]flag.Override{},
	}
}

func key(flagName, userID string) string { return flagName + "\x00" + userID }

func (s *Store) Ping(context.Context) error { return nil }
func (s *Store) Close() error               { return nil }

func (s *Store) CreateFlag(_ context.Context, f flag.Flag) (flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.flags[f.Name]; ok {
		return flag.Flag{}, store.ErrAlreadyExists
	}
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	s.flags[f.Name] = f
	return f, nil
}

func (s *Store) GetFlag(_ context.Context, name string) (flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flags[name]
	if !ok {
		return flag.Flag{}, store.ErrNotFound
	}
	return f, nil
}

func (s *Store) ListFlags(context.Context) ([]flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]flag.Flag, 0, len(s.flags))
	for _, f := range s.flags {
		out = append(out, f)
	}
	return out, nil
}

func (s *Store) UpdateFlag(_ context.Context, name string, enabled *bool, description *string, rollout *int) (flag.Flag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flags[name]
	if !ok {
		return flag.Flag{}, store.ErrNotFound
	}
	if enabled != nil {
		f.Enabled = *enabled
	}
	if description != nil {
		f.Description = *description
	}
	if rollout != nil {
		f.RolloutPercent = *rollout
	}
	f.UpdatedAt = time.Now().UTC()
	s.flags[name] = f
	return f, nil
}

func (s *Store) DeleteFlag(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.flags[name]; !ok {
		return store.ErrNotFound
	}
	delete(s.flags, name)
	prefix := name + "\x00"
	for k := range s.overrides {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(s.overrides, k)
		}
	}
	return nil
}

func (s *Store) SetOverride(_ context.Context, o flag.Override) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.flags[o.FlagName]; !ok {
		return store.ErrNotFound
	}
	s.overrides[key(o.FlagName, o.UserID)] = o
	return nil
}

func (s *Store) GetOverride(_ context.Context, flagName, userID string) (flag.Override, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.overrides[key(flagName, userID)]
	if !ok {
		return flag.Override{}, store.ErrNotFound
	}
	return o, nil
}

func (s *Store) DeleteOverride(_ context.Context, flagName, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(flagName, userID)
	if _, ok := s.overrides[k]; !ok {
		return store.ErrNotFound
	}
	delete(s.overrides, k)
	return nil
}

var _ store.FlagStore = (*Store)(nil)

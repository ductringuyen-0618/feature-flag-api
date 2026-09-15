package cached

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/ductringuyen-0618/feature-flag-api/internal/flag"
	"github.com/ductringuyen-0618/feature-flag-api/internal/store"
	redissync "github.com/ductringuyen-0618/feature-flag-api/internal/sync/redis"
)

// Service is the application façade: Postgres SoT, in-memory flag snapshot,
// Redis pub/sub. Overrides are durable in Postgres only.
type Service struct {
	db    store.FlagStore
	sync  *redissync.Sync // may be nil
	reload time.Duration

	mu    sync.RWMutex
	flags map[string]flag.Flag

	sf singleflight.Group

	redisOK bool
}

func New(db store.FlagStore, syncClient *redissync.Sync, reload time.Duration) *Service {
	if reload <= 0 {
		reload = 10 * time.Second
	}
	return &Service{
		db:     db,
		sync:   syncClient,
		reload: reload,
		flags:  map[string]flag.Flag{},
	}
}

func (s *Service) Start(ctx context.Context) error {
	if err := s.Reload(ctx); err != nil {
		return err
	}
	if s.sync != nil {
		s.redisOK = true
		_ = s.sync.Subscribe(ctx, func(name string) {
			s.applyInvalidation(context.Background(), name)
		})
	}
	go s.poll(ctx)
	return nil
}

func (s *Service) Health(ctx context.Context) string {
	if s.sync == nil {
		return "ok"
	}
	cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if err := s.sync.Ping(cctx); err != nil {
		s.redisOK = false
		return "degraded"
	}
	s.redisOK = true
	return "ok"
}

func (s *Service) Ready(ctx context.Context) error {
	return s.db.Ping(ctx)
}

func (s *Service) Reload(ctx context.Context) error {
	list, err := s.db.ListFlags(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]flag.Flag, len(list))
	for _, f := range list {
		next[f.Name] = f
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// An empty ListFlags is more often a blip than a true wipe of a warm snapshot.
	if len(next) == 0 && len(s.flags) > 0 {
		return nil
	}
	s.flags = next
	return nil
}

func (s *Service) applyInvalidation(ctx context.Context, name string) {
	if name == "" || name == "*" {
		_ = s.Reload(ctx)
		return
	}
	f, err := s.db.GetFlag(ctx, name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if errors.Is(err, store.ErrNotFound) {
		delete(s.flags, name)
		return
	}
	if err != nil {
		return
	}
	s.flags[name] = f
}

func (s *Service) poll(ctx context.Context) {
	t := time.NewTicker(s.reload)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Reload(ctx)
		}
	}
}

func (s *Service) publish(ctx context.Context, name string) {
	if s.sync == nil {
		return
	}
	_ = s.sync.PublishChanged(ctx, name)
}

func (s *Service) CreateFlag(ctx context.Context, f flag.Flag) (flag.Flag, error) {
	out, err := s.db.CreateFlag(ctx, f)
	if err != nil {
		return flag.Flag{}, err
	}
	s.mu.Lock()
	s.flags[out.Name] = out
	s.mu.Unlock()
	s.publish(ctx, out.Name)
	return out, nil
}

func (s *Service) GetFlag(ctx context.Context, name string) (flag.Flag, error) {
	s.mu.RLock()
	f, ok := s.flags[name]
	s.mu.RUnlock()
	if ok {
		return f, nil
	}
	return s.db.GetFlag(ctx, name)
}

func (s *Service) ListFlags(ctx context.Context) ([]flag.Flag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]flag.Flag, 0, len(s.flags))
	for _, f := range s.flags {
		out = append(out, f)
	}
	return out, nil
}

func (s *Service) UpdateFlag(ctx context.Context, name string, enabled *bool, description *string, rollout *int) (flag.Flag, error) {
	out, err := s.db.UpdateFlag(ctx, name, enabled, description, rollout)
	if err != nil {
		return flag.Flag{}, err
	}
	s.mu.Lock()
	s.flags[out.Name] = out
	s.mu.Unlock()
	s.publish(ctx, out.Name)
	return out, nil
}

func (s *Service) DeleteFlag(ctx context.Context, name string) error {
	if err := s.db.DeleteFlag(ctx, name); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.flags, name)
	s.mu.Unlock()
	s.publish(ctx, name)
	return nil
}

func (s *Service) SetOverride(ctx context.Context, o flag.Override) error {
	if _, err := s.GetFlag(ctx, o.FlagName); err != nil {
		return err
	}
	return s.db.SetOverride(ctx, o)
}

func (s *Service) DeleteOverride(ctx context.Context, flagName, userID string) error {
	return s.db.DeleteOverride(ctx, flagName, userID)
}

func (s *Service) getOverride(ctx context.Context, flagName, userID string) (*flag.Override, error) {
	v, err, _ := s.sf.Do(flagName+"\x00"+userID, func() (any, error) {
		o, err := s.db.GetOverride(ctx, flagName, userID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &o, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*flag.Override), nil
}

// Evaluate loads the flag from the snapshot and the override from Postgres.
func (s *Service) Evaluate(ctx context.Context, flagName, userID string) (flag.Result, error) {
	s.mu.RLock()
	f, ok := s.flags[flagName]
	s.mu.RUnlock()
	if !ok {
		return flag.Result{}, store.ErrNotFound
	}
	ovr, err := s.getOverride(ctx, flagName, userID)
	if err != nil {
		return flag.Result{}, err
	}
	return flag.Evaluate(&f, userID, ovr), nil
}

// EvaluateBulk evaluates many flags for one user.
func (s *Service) EvaluateBulk(ctx context.Context, userID string, names []string) (map[string]flag.Result, error) {
	out := make(map[string]flag.Result, len(names))
	for _, name := range names {
		res, err := s.Evaluate(ctx, name, userID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[name] = res
	}
	return out, nil
}

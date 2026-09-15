package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const changedChannel = "flags:changed"

// Sync handles pub/sub invalidation and short-TTL override caching.
// Redis is optional: failures degrade to Postgres-backed paths.
type Sync struct {
	client *goredis.Client
	ttl    time.Duration
}

func New(ctx context.Context, redisURL string, overrideTTL time.Duration) (*Sync, error) {
	opt, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := goredis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	if overrideTTL <= 0 {
		overrideTTL = time.Minute
	}
	return &Sync{client: client, ttl: overrideTTL}, nil
}

func (s *Sync) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Sync) Close() error {
	return s.client.Close()
}

func (s *Sync) PublishChanged(ctx context.Context, flagName string) error {
	return s.client.Publish(ctx, changedChannel, flagName).Err()
}

func (s *Sync) Subscribe(ctx context.Context, fn func(flagName string)) error {
	pubsub := s.client.Subscribe(ctx, changedChannel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return err
	}
	ch := pubsub.Channel()
	go func() {
		defer pubsub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if fn != nil {
					fn(msg.Payload)
				}
			}
		}
	}()
	return nil
}

type overrideCacheVal struct {
	Found   bool `json:"found"`
	Enabled bool `json:"enabled"`
}

func overrideKey(flagName, userID string) string {
	return fmt.Sprintf("override:%s:%s", flagName, userID)
}

func (s *Sync) GetOverride(ctx context.Context, flagName, userID string) (enabled bool, found bool, ok bool) {
	raw, err := s.client.Get(ctx, overrideKey(flagName, userID)).Result()
	if err != nil {
		return false, false, false
	}
	var v overrideCacheVal
	if json.Unmarshal([]byte(raw), &v) != nil {
		return false, false, false
	}
	return v.Enabled, v.Found, true
}

func (s *Sync) SetOverride(ctx context.Context, flagName, userID string, found, enabled bool) {
	raw, err := json.Marshal(overrideCacheVal{Found: found, Enabled: enabled})
	if err != nil {
		return
	}
	_ = s.client.Set(ctx, overrideKey(flagName, userID), raw, s.ttl).Err()
}

func (s *Sync) InvalidateOverride(ctx context.Context, flagName, userID string) {
	_ = s.client.Del(ctx, overrideKey(flagName, userID)).Err()
}

func (s *Sync) InvalidateFlagOverrides(ctx context.Context, flagName string) {
	// Best-effort pattern delete; interview-scale.
	iter := s.client.Scan(ctx, 0, "override:"+flagName+":*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		_ = s.client.Del(ctx, keys...).Err()
	}
}

// BumpVersion is reserved for clients that want versioned keys; kept for docs/tests.
func (s *Sync) BumpVersion(ctx context.Context, flagName string) (int64, error) {
	return s.client.Incr(ctx, "flag:version:"+flagName).Result()
}

package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

const changedChannel = "flags:changed"

// Sync handles pub/sub invalidation for flag snapshot refresh.
// Redis is optional: failures degrade to local reload polling.
type Sync struct {
	client *goredis.Client
}

func New(ctx context.Context, redisURL string) (*Sync, error) {
	opt, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := goredis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Sync{client: client}, nil
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

// Package lockx implements token-checked Redis leases.
package lockx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

type Locker struct{ client redis.UniversalClient }

func NewLocker(client redis.UniversalClient) *Locker { return &Locker{client: client} }

// Acquire returns a nil release function if another owner holds the lease.
func (l *Locker) Acquire(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(nonce[:])
	acquired, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !acquired {
		return nil, err
	}
	return func(ctx context.Context) error {
		const release = "if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end"
		return l.client.Eval(ctx, release, []string{key}, token).Err()
	}, nil
}

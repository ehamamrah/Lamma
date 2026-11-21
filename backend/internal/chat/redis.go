package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const historySize = 100

// RedisBuffer stores recent messages in a capped list that expires with the meeting.
type RedisBuffer struct {
	rdb *redis.Client
}

// NewRedisBuffer creates a RedisBuffer.
func NewRedisBuffer(rdb *redis.Client) *RedisBuffer { return &RedisBuffer{rdb: rdb} }

func key(meetingID uuid.UUID) string { return "chat:" + meetingID.String() }

// Append pushes payload, trims to historySize and sets the expiry, atomically.
func (b *RedisBuffer) Append(ctx context.Context, meetingID uuid.UUID, payload []byte, until time.Time) error {
	k := key(meetingID)
	_, err := b.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.LPush(ctx, k, payload)
		p.LTrim(ctx, k, 0, historySize-1)
		p.ExpireAt(ctx, k, until)
		return nil
	})
	return err
}

// Recent returns up to historySize payloads, newest first.
func (b *RedisBuffer) Recent(ctx context.Context, meetingID uuid.UUID) ([][]byte, error) {
	vals, err := b.rdb.LRange(ctx, key(meetingID), 0, historySize-1).Result()
	out := make([][]byte, len(vals))
	for i, v := range vals {
		out[i] = []byte(v)
	}
	return out, err
}

// Clear deletes the history.
func (b *RedisBuffer) Clear(ctx context.Context, meetingID uuid.UUID) error {
	return b.rdb.Del(ctx, key(meetingID)).Err()
}

package limiter

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisLimiter_ContextCancellation(t *testing.T) {
	client := redisTestClient(t)
	limiter, err := NewRedisLimiter(client, WithPrefix(redisTestPrefix(t, client)))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	limit := Limit{
		Rate:   100,
		Burst:  100,
		Period: time.Second,
	}
	id := Identity{Namespace: "test", Key: "user_cancel"}

	_, err = limiter.Allow(ctx, id, limit)

	if err == nil {
		t.Fatal("Expected an error due to cancelled context, but got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected error to be context.Canceled, but got: %v", err)
	}
}

func TestRedisLimiter_Deadline(t *testing.T) {
	client := redisTestClient(t)
	limiter, err := NewRedisLimiter(client, WithPrefix(redisTestPrefix(t, client)))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	limit := Limit{
		Rate:   100,
		Burst:  100,
		Period: time.Second,
	}
	id := Identity{Namespace: "test", Key: "user_deadline"}

	_, err = limiter.Allow(ctx, id, limit)

	if err == nil {
		t.Fatal("Expected timeout error, but got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected error to be context.DeadlineExceeded, but got: %v", err)
	}
}

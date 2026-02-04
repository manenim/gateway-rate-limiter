package limiter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_Options(t *testing.T) {
	opts := &redis.Options{Addr: "localhost:6379"}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping integration test: Redis not available (%v)", err)
	}
	defer client.Close()

	t.Run("WithPrefix", func(t *testing.T) {
		prefix := "custom_app:"
		key := fmt.Sprintf("opt_test_%d", time.Now().UnixNano())
		id := Identity{Namespace: "options", Key: key}
		limit := Limit{Rate: 1, Period: time.Second, Burst: 1}

		limiter, err := NewRedisLimiter(client, WithPrefix(prefix))
		if err != nil {
			t.Fatalf("Failed to create limiter: %v", err)
		}

		_, err = limiter.Allow(ctx, id, limit)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}

		// Verify the key uses the custom prefix
		expectedKey := prefix + string(id.Namespace) + ":" + id.Key
		exists, err := client.Exists(ctx, expectedKey).Result()
		if err != nil {
			t.Fatalf("Redis Exists failed: %v", err)
		}
		if exists == 0 {
			t.Errorf("Expected key %s to exist, but it does not", expectedKey)
		}
	})

	t.Run("WithTimeout", func(t *testing.T) {
		// Hard to test timeout without mocking network latency or setting extremely small timeout.
		// We can check if NewRedisLimiter succeeds with valid timeout.
		_, err := NewRedisLimiter(client, WithTimeout(10*time.Millisecond))
		if err != nil {
			t.Errorf("WithTimeout should not cause error on valid client: %v", err)
		}
	})
}

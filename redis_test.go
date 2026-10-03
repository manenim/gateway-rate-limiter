package limiter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestClient makes integration tests optional locally and mandatory in CI.
// Use a dedicated Redis instance: tests create isolated keys, never FLUSHDB.
func redisTestClient(t testing.TB) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{
		Addr: addr, MaxRetries: -1,
		DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
		ContextTimeoutEnabled: true,
	})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		if os.Getenv("REQUIRE_REDIS") == "1" {
			t.Fatalf("Redis required at %s: %v", addr, err)
		}
		t.Skipf("Redis unavailable at %s (set REQUIRE_REDIS=1 to fail): %v", addr, err)
	}
	return client
}

var redisTestSequence atomic.Uint64

func redisTestPrefix(t testing.TB, client *redis.Client) string {
	t.Helper()
	prefix := fmt.Sprintf("limiter-test:%d:%d:%s:", time.Now().UnixNano(), redisTestSequence.Add(1), t.Name())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Prefixes are unique to this test; cleanup never touches other data.
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
			if err != nil {
				return
			}
			if len(keys) > 0 {
				_ = client.Del(ctx, keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	})
	return prefix
}

func TestRedisLimiter_Integration(t *testing.T) {
	client := redisTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limiter, err := NewRedisLimiter(client, WithPrefix(redisTestPrefix(t, client)))
	if err != nil {
		t.Fatalf("Failed to create RedisLimiter: %v", err)
	}

	t.Run("BasicFlow", func(t *testing.T) {
		key := fmt.Sprintf("it_test_%d", time.Now().UnixNano())
		id := Identity{Namespace: "integration", Key: key}
		limit := Limit{
			Rate:   10,
			Period: time.Second,
			Burst:  2,
		}

		dec, err := limiter.Allow(ctx, id, limit)
		if err != nil {
			t.Fatalf("Redis error: %v", err)
		}
		if !dec.Allow {
			t.Error("Expected first request to be Allowed")
		}
		if dec.Remaining != 1 {
			t.Errorf("Expected 1 remaining, got %d", dec.Remaining)
		}

		dec, err = limiter.Allow(ctx, id, limit)
		if err != nil {
			t.Fatal(err)
		}
		if !dec.Allow {
			t.Error("Expected second request to be Allowed")
		}

		dec, err = limiter.Allow(ctx, id, limit)
		if err != nil {
			t.Fatal(err)
		}
		if dec.Allow {
			t.Error("Expected third request to be Denied")
		}
		if dec.RetryAfter <= 0 {
			t.Error("Expected positive RetryAfter on denial")
		}
	})

	t.Run("DistributedState", func(t *testing.T) {
		key := fmt.Sprintf("dist_test_%d", time.Now().UnixNano())
		id := Identity{Namespace: "integration", Key: key}
		limit := Limit{Rate: 1, Period: time.Second, Burst: 1}

		prefix := redisTestPrefix(t, client)
		limiterA, err := NewRedisLimiter(client, WithPrefix(prefix))
		if err != nil {
			t.Fatal(err)
		}
		first, err := limiterA.Allow(ctx, id, limit)
		if err != nil || !first.Allow {
			t.Fatalf("first decision = %+v, err = %v", first, err)
		}

		limiterB, err := NewRedisLimiter(client, WithPrefix(prefix))
		if err != nil {
			t.Fatal(err)
		}
		dec, err := limiterB.Allow(ctx, id, limit)

		if err != nil {
			t.Fatal(err)
		}
		if dec.Allow {
			t.Error("Instance B should see the token consumed by Instance A")
		}
	})
}

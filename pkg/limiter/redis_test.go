package limiter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_Integration(t *testing.T) {
	opts := &redis.Options{
		Addr: "localhost:6379",
	}
	client := redis.NewClient(opts)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping integration test: Redis not available (%v)", err)
	}

	limiter, err := NewRedisLimiter(client)
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
		if err != nil { t.Fatal(err) }
		if !dec.Allow { t.Error("Expected second request to be Allowed") }

		dec, err = limiter.Allow(ctx, id, limit)
		if err != nil { t.Fatal(err) }
		if dec.Allow { t.Error("Expected third request to be Denied") }
		if dec.RetryAfter <= 0 {
			t.Error("Expected positive RetryAfter on denial")
		}
	})

	t.Run("DistributedState", func(t *testing.T) {
		key := fmt.Sprintf("dist_test_%d", time.Now().UnixNano())
		id := Identity{Namespace: "integration", Key: key}
		limit := Limit{Rate: 1, Period: time.Second, Burst: 1}

		// Instance A consumes the token
		limiterA, _ := NewRedisLimiter(client) // Simulate Node A
		limiterA.Allow(ctx, id, limit)

		// Instance B tries to consume same token
		limiterB, _ := NewRedisLimiter(client) // Simulate Node B
		dec, err := limiterB.Allow(ctx, id, limit)
		
		if err != nil { t.Fatal(err) }
		if dec.Allow {
			t.Error("Instance B should see the token consumed by Instance A")
		}
	})
}

package limiter

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_Allow_Basics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   10,
		Period: time.Second,
		Burst:  10,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	decision, _ := limiter.Allow(ctx, id, limit)

	if !decision.Allow {
		t.Error("Expected request to be allowed, but got denied!.")
	}

	if decision.Remaining != 9 {
		t.Errorf("Expected 9 remaining tokens got %d instead!", decision.Remaining)
	}

}

func TestMemoryLimiter_Exhaustion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   1,
		Period: time.Second,
		Burst:  5,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	for i := 0; i < 5; i++ {
		dec, _ := limiter.Allow(ctx, id, limit)
		if !dec.Allow {
			t.Fatalf("Request %d was unexpectedly denied", i)
		}
	}

	dec, _ := limiter.Allow(ctx, id, limit)
	if dec.Allow {
		t.Errorf("The 6th request should have been denied (Burst=5), but was allowed")
	}
}

func TestMemoryLimiter_Refill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   10,
		Period: time.Second,
		Burst:  1,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	limiter.Allow(ctx, id, limit)

	dec, _ := limiter.Allow(ctx, id, limit)

	if dec.Allow {
		t.Fatal("Should be denied immediately")
	}

	time.Sleep(150 * time.Millisecond)

	dec, err := limiter.Allow(ctx, id, limit)
	if err != nil || !dec.Allow {
		t.Fatalf("Expected allowance after refill, got %+v, err = %v", dec, err)
	}
}

// Race Test
func TestMemoryLimiter_ThreadSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   1,
		Burst:  100,
		Period: 24 * time.Hour,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	var wg sync.WaitGroup

	wg.Add(100)
	for range 100 {
		go func() {
			defer wg.Done()
			limiter.Allow(ctx, id, limit)
		}()
	}
	wg.Wait()

	dec, _ := limiter.Allow(ctx, id, limit)
	if dec.Allow {
		t.Errorf("Expected bucket to be exhausted after 100 concurrent requests, but 101st was allowed")
	}
}

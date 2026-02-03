package limiter

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_Allow_Basics(t *testing.T) {
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   10,
		Period: time.Second,
		Burst:  10,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	decision := limiter.Allow(id, limit)

	if !decision.Allow {
		t.Error("Expected request to be allowed, but got denied!.")
	}

	if decision.Remaining != 9 {
		t.Logf("Expected 9 remianing tokens got %d instead!", decision.Remaining)
	}

}

func TestMemoryLimiter_Exhaustion(t *testing.T) {
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   1,
		Period: time.Second,
		Burst:  5,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	for i := 0; i < 5; i++ {
		dec := limiter.Allow(id, limit)
		if !dec.Allow {
			t.Fatalf("Request %d was unexpectedly denied", i)
		}
	}

	dec := limiter.Allow(id, limit)
	if dec.Allow {
		t.Errorf("The 6th request should have been denied (Burst=5), but was allowed")
	}
}

func TestMemoryLimiter_Refill(t *testing.T) {
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   10,
		Period: time.Second,
		Burst:  1,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	limiter.Allow(id, limit)

	if limiter.Allow(id, limit).Allow {
		t.Fatal("Should be denied immediately")
	}

	time.Sleep(150 * time.Millisecond)

	dec := limiter.Allow(id, limit)
	if !dec.Allow {
		t.Errorf("Refill failed! Waited 150ms for a 100ms token but was denied.")
	}
}

// Race Test
func TestMemoryLimiter_ThreadSafety(t *testing.T) {
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   100,
		Burst:  100,
		Period: time.Second,
	}

	id := Identity{Namespace: "test", Key: "user_1"}

	var wg sync.WaitGroup

	wg.Add(100)
	for range 100 {
		go func() {
			defer wg.Done()
			limiter.Allow(id, limit)
		}()
	}

	wg.Wait()

	dec := limiter.Allow(id, limit)
	if dec.Allow {
		t.Errorf("Expected bucket to be exhausted after 100 concurrent requests, but 101st was allowed")
	}
}

func BenchmarkMemoryLimiter_Allow(b *testing.B) {
	limiter := NewMemoryLimiter()

	limit := Limit{
		Rate:   1000,
		Burst:  100000,
		Period: time.Second,
	}
	id := Identity{Namespace: "test", Key: "user_1"}

	for b.Loop() {
		limiter.Allow(id, limit)
	}
}

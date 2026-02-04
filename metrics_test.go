package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// MockRecorder captures metrics in memory for assertion
type MockRecorder struct {
	Counters map[string]float64
	Timings  map[string][]float64
}

func NewMockRecorder() *MockRecorder {
	return &MockRecorder{
		Counters: make(map[string]float64),
		Timings:  make(map[string][]float64),
	}
}

func (m *MockRecorder) Add(name string, value float64, tags map[string]string) {
	m.Counters[name] += value
}

func (m *MockRecorder) Observe(name string, value float64, tags map[string]string) {
	m.Timings[name] = append(m.Timings[name], value)
}

func TestRedisLimiter_Metrics(t *testing.T) {
	// 1. Setup Redis (Reuse integration test pattern)
	opts := &redis.Options{Addr: "localhost:6379"}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping metrics test: Redis not available (%v)", err)
	}
	defer client.Close()

	// 3. Inject Mock Recorder
	mock := NewMockRecorder()

	// 2. Create Limiter with Recorder
	limiter, err := NewRedisLimiter(client, WithRecorder(mock))
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// 4. Perform Action
	id := Identity{Namespace: "metrics_test", Key: "user_1"}
	limit := Limit{Rate: 10, Period: time.Second, Burst: 10}

	_, err = limiter.Allow(context.Background(), id, limit)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	// 5. Assertions

	// Check "ratelimit.call" Counter
	if val, ok := mock.Counters["ratelimit.call"]; !ok || val != 1 {
		t.Errorf("Expected 'ratelimit.call' counter to be 1, got %v", val)
	}

	// Check "ratelimit.latency" Histogram
	if timings, ok := mock.Timings["ratelimit.latency"]; !ok || len(timings) != 1 {
		t.Error("Expected 1 latency observation")
	} else if timings[0] <= 0 {
		t.Errorf("Expected positive latency, got %v", timings[0])
	}
}

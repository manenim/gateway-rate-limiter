package limiter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Separate clients model replicas with independent connection pools. A one-day
// refill period prevents additional whole tokens during this bounded test.
func TestRedisLimiter_ConcurrentSharedBucket(t *testing.T) {
	const replicas, requests, burst = 8, 256, 37
	client := redisTestClient(t)
	prefix := redisTestPrefix(t, client)
	limiters := make([]*RedisLimiter, replicas)
	for i := range limiters {
		var err error
		limiters[i], err = NewRedisLimiter(redisTestClient(t), WithPrefix(prefix))
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	limit := Limit{Rate: 1, Period: 24 * time.Hour, Burst: burst}
	id := Identity{Namespace: "contention", Key: "shared"}
	type result struct {
		decision Decision
		err      error
	}
	results := make(chan result, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			decision, err := limiters[i%replicas].Allow(ctx, id, limit)
			results <- result{decision, err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	allowed := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("Allow: %v", result.err)
		}
		dec := result.decision
		if dec.Remaining < 0 || dec.Remaining >= burst {
			t.Fatalf("invalid remaining: %+v", dec)
		}
		if dec.Allow {
			allowed++
		} else if dec.RetryAfter <= 0 {
			t.Fatalf("missing retry hint: %+v", dec)
		}
	}
	if allowed != burst {
		t.Fatalf("allowed %d/%d requests across %d clients, want exactly burst=%d", allowed, requests, replicas, burst)
	}
	t.Logf("shared bucket: %d allowed, %d denied across %d independent clients", allowed, requests-allowed, replicas)
}

type metricEvent struct {
	name  string
	value float64
	tags  map[string]string
}

type eventRecorder struct {
	mu                sync.Mutex
	counters, timings []metricEvent
}

func (r *eventRecorder) Add(name string, value float64, tags map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = append(r.counters, metricEvent{name, value, tags})
}
func (r *eventRecorder) Observe(name string, value float64, tags map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timings = append(r.timings, metricEvent{name, value, tags})
}

func TestRedisLimiter_DependencyFailures(t *testing.T) {
	for _, failure := range []string{"closed_client", "wrong_key_type", "missing_script"} {
		t.Run(failure, func(t *testing.T) {
			client := redisTestClient(t)
			prefix := redisTestPrefix(t, redisTestClient(t))
			recorder := &eventRecorder{}
			limiter, err := NewRedisLimiter(client, WithPrefix(prefix), WithRecorder(recorder))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			id := Identity{Namespace: "failure", Key: failure}
			limit := Limit{Rate: 1, Period: 24 * time.Hour, Burst: 1}
			// Prove the initialized limiter works before introducing a failure.
			if dec, err := limiter.Allow(ctx, id, limit); err != nil || !dec.Allow {
				t.Fatalf("healthy decision = %+v, err = %v", dec, err)
			}
			switch failure {
			case "closed_client":
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
			case "wrong_key_type":
				key := prefix + string(id.Namespace) + ":" + id.Key
				if err := client.Set(ctx, key, "not a hash", time.Minute).Err(); err != nil {
					t.Fatal(err)
				}
			case "missing_script":
				// Model a cache miss without flushing scripts used by other tests.
				limiter.scriptSHA = "0000000000000000000000000000000000000000"
			}
			dec, err := limiter.Allow(ctx, id, limit)
			if err == nil || dec != (Decision{}) {
				t.Fatalf("failure decision = %+v, err = %v; want zero decision and error", dec, err)
			}
			if len(recorder.counters) != 2 || len(recorder.timings) != 2 {
				t.Fatalf("metrics = %+v", recorder)
			}
			counter := recorder.counters[1]
			if counter.name != "ratelimit.errors" || counter.value != 1 || counter.tags["namespace"] != "failure" || counter.tags["type"] != "redis_eval" {
				t.Fatalf("error counter = %+v", counter)
			}
			timing := recorder.timings[1]
			if timing.name != "ratelimit.latency" || timing.value <= 0 || timing.tags["status"] != "error" {
				t.Fatalf("error latency = %+v", timing)
			}
			t.Logf("%s returned %v and emitted error metrics", failure, err)
		})
	}
}

// This initialization failure is deterministic and requires no running backend.
func TestRedisLimiter_UnreachableDependency(t *testing.T) {
	unreachable := errors.New("test: dependency unreachable")
	client := redis.NewClient(&redis.Options{
		Addr: "unused:6379", MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) { return nil, unreachable },
	})
	defer client.Close()
	limiter, err := NewRedisLimiter(client, WithTimeout(time.Second))
	if limiter != nil || !errors.Is(err, unreachable) {
		t.Fatalf("NewRedisLimiter = %v, %v; want dependency error", limiter, err)
	}
}

func TestRedisLimiter_MetricDecisionTags(t *testing.T) {
	client := redisTestClient(t)
	recorder := &eventRecorder{}
	limiter, err := NewRedisLimiter(client, WithPrefix(redisTestPrefix(t, client)), WithRecorder(recorder))
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"allowed", "denied"} {
		dec, err := limiter.Allow(context.Background(), Identity{Namespace: "tags", Key: "same"}, Limit{Rate: 1, Period: 24 * time.Hour, Burst: 1})
		if err != nil || dec.Allow != (i == 0) {
			t.Fatalf("%s: decision=%+v, err=%v", status, dec, err)
		}
		counter, timing := recorder.counters[i], recorder.timings[i]
		if counter.name != "ratelimit.call" || counter.value != 1 || counter.tags["namespace"] != "tags" || counter.tags["status"] != status {
			t.Fatalf("decision counter: %+v", counter)
		}
		if timing.name != "ratelimit.latency" || timing.value <= 0 || timing.tags["status"] != status {
			t.Fatalf("decision latency: %+v", timing)
		}
	}
}

func BenchmarkRedisLimiter_Allow(b *testing.B) {
	for _, allowed := range []bool{true, false} {
		b.Run(fmt.Sprintf("allowed=%t", allowed), func(b *testing.B) {
			client := redisTestClient(b)
			limiter, err := NewRedisLimiter(client, WithPrefix(redisTestPrefix(b, client)))
			if err != nil {
				b.Fatal(err)
			}
			limit := Limit{Rate: 1, Period: 24 * time.Hour, Burst: 1}
			if allowed {
				limit = Limit{Rate: 1_000_000_000, Period: time.Second, Burst: 1_000_000_000}
			}
			ctx := context.Background()
			id := Identity{Namespace: "benchmark", Key: "shared"}
			if _, err := limiter.Allow(ctx, id, limit); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				dec, err := limiter.Allow(ctx, id, limit)
				if err != nil || dec.Allow != allowed {
					b.Fatalf("decision = %+v, err = %v", dec, err)
				}
			}
		})
	}
}

func TestRedisLimiter_ServerVersion(t *testing.T) {
	client := redisTestClient(t)
	info, err := client.Info(context.Background(), "server").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") || strings.HasPrefix(line, "redis_mode:") {
			t.Log(strings.TrimSpace(line))
		}
	}
}

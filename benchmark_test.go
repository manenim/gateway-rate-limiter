package limiter

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Explicit states keep the benchmark from silently changing from allowed to
// denied once a finite burst is exhausted during a longer measurement.
func BenchmarkMemoryLimiter_Decisions(b *testing.B) {
	for _, allowed := range []bool{true, false} {
		b.Run(fmt.Sprintf("allowed=%t", allowed), func(b *testing.B) {
			limiter := NewMemoryLimiter()
			ctx := context.Background()
			id := Identity{Namespace: "benchmark", Key: "shared"}
			limit := Limit{Rate: 1, Period: 24 * time.Hour, Burst: 1}
			if allowed {
				limit = Limit{Rate: 1_000_000_000, Period: time.Second, Burst: 1_000_000_000}
			}
			_, _ = limiter.Allow(ctx, id, limit)
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

func BenchmarkMemoryLimiter_ParallelSharedBucket(b *testing.B) {
	limiter := NewMemoryLimiter()
	ctx := context.Background()
	id := Identity{Namespace: "benchmark", Key: "shared"}
	limit := Limit{Rate: 1_000_000_000, Period: time.Second, Burst: 1_000_000_000}
	_, _ = limiter.Allow(ctx, id, limit)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dec, err := limiter.Allow(ctx, id, limit)
			if err != nil || !dec.Allow {
				b.Errorf("decision = %+v, err = %v", dec, err)
				return
			}
		}
	})
}

package limiter

import (
	"context"
	"time"
)

type Namespace string

// Limit defines a token-bucket policy.
//
// Rate is measured as tokens per Period. Burst is the maximum token capacity of
// the bucket and controls how many requests can be allowed immediately.
type Limit struct {
	Rate   int64
	Period time.Duration
	Burst  int64
}

// Decision is the result of a rate-limit check.
type Decision struct {
	Allow      bool
	Remaining  int64
	RetryAfter time.Duration
	ResetTime  time.Time
}

// Identity uniquely identifies the subject being rate-limited (for example,
// a user ID, an API key, or an IP address).
type Identity struct {
	Namespace Namespace
	Key       string
}

// RateLimiter performs token-bucket admission control.
type RateLimiter interface {
	Allow(ctx context.Context, id Identity, limit Limit) (Decision, error)
}

// MetricsRecorder defines the interface for collecting telemetry.
// We abstract this so we aren't tied to Prometheus, Datadog, or any specific vendor.
type MetricsRecorder interface {
	// Add increments a counter (e.g., requests_total)
	Add(name string, value float64, tags map[string]string)
	
	// Observe records a value in a histogram/distribution (e.g., latency)
	Observe(name string, value float64, tags map[string]string)
}
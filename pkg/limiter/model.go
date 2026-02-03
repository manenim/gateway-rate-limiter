package limiter

import (
	"context"
	"time"
)

type Namespace string

type Limit struct {
	Rate   int64
	Period time.Duration
	Burst  int64
}

type Decision struct {
	Allow      bool
	Remaining  int64
	RetryAfter time.Duration
	ResetTime  time.Time
}

type Identity struct {
	Namespace Namespace
	Key       string
}

type RateLimiter interface {
    Allow(ctx context.Context, id Identity, limit Limit) (Decision, error)
}
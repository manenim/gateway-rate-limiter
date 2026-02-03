package limiter

import (
	"context"
	"sync"
	"time"
)

type state struct {
	tokens     float64
	lastRefill time.Time
}

type MemoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*state
}

func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{
		buckets: make(map[string]*state),
	}
}

func (m *MemoryLimiter) Allow(ctx context.Context, id Identity, limit Limit) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	key := string(id.Namespace) + ":" + id.Key
	st, exists := m.buckets[key]
	if !exists {
		m.buckets[key] = &state{
			tokens:     float64(limit.Burst) - 1,
			lastRefill: now,
		}
		return Decision{
			Allow:      true,
			Remaining:  limit.Burst - 1,
			RetryAfter: 0,
			ResetTime:  now,
		}, nil
	} else {
		elapsed := now.Sub(st.lastRefill)
		if elapsed < 0 {
			elapsed = 0
		}
		delta := float64(elapsed) / float64(limit.Period)
		tokensToAdd := delta * float64(limit.Rate)
		newBalance := st.tokens + tokensToAdd
		if newBalance > float64(limit.Burst) {
			newBalance = float64(limit.Burst)
		}
		st.tokens = newBalance

		st.lastRefill = now

		if st.tokens >= 1 {
			st.tokens -= 1
			return Decision{
				Allow:      true,
				Remaining:  int64(st.tokens),
				RetryAfter: 0,
				ResetTime:  now,
			}, nil
		} else {
			costPerToken := float64(limit.Period) / float64(limit.Rate)
			missing := 1.0 - st.tokens
			waitParams := missing * costPerToken
			return Decision{
				Allow:      false,
				Remaining:  int64(st.tokens),
				RetryAfter: time.Duration(waitParams),
				ResetTime:  now.Add(time.Duration(waitParams)),
			}, nil
		}
	}

}

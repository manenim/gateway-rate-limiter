// Package limiter provides local and distributed rate limiting based on the
// Token Bucket algorithm.
//
// The primary entry point is the RateLimiter interface:
//
//	dec, err := limiter.Allow(ctx, id, limit)
//
// The returned Decision contains whether the request is allowed, how many whole
// tokens remain, and timing hints for callers that want to set rate-limit
// headers (for example, Retry-After).
//
// # Overview
//
// This package implements a Token Bucket:
//
//   - Each identity has a "bucket" holding tokens.
//   - The bucket refills over time up to a maximum capacity (Burst).
//   - Each Allow call consumes 1 token when available.
//
// Unlike fixed-window counters, token buckets naturally support bursts while
// still enforcing a long-term average rate.
//
// # Core Types
//
// Limit defines the policy:
//
//   - Rate: tokens earned per Period (for example, 10 per second or 60 per
//     minute)
//   - Period: the time window Rate is measured over
//   - Burst: maximum number of tokens the bucket can hold (also the maximum
//     immediate burst)
//
// Identity defines "who" is being rate-limited. It is split into:
//
//   - Namespace: a logical grouping (for example, "user", "ip", "api_key")
//   - Key: the identifier within that namespace (for example, "user_123")
//
// # Backends
//
// The package provides two implementations with the same Allow API:
//
//   - MemoryLimiter: an in-process limiter backed by a Go map. This is useful
//     for unit tests, local development, and single-instance deployments.
//     Because its state is local to the process, it does not enforce a global
//     limit across multiple replicas.
//
//   - RedisLimiter: a distributed limiter backed by Redis. It uses a Lua script
//     to perform the read/compute/write cycle atomically, which makes it safe to
//     use across many application instances while enforcing a single global
//     budget per identity.
//
// Recommendation: use RedisLimiter in production when you need a global limit,
// and MemoryLimiter in tests (as a fast, dependency-free stand-in).
//
// # Concurrency
//
// MemoryLimiter is safe for concurrent use by multiple goroutines (it uses a
// mutex to protect its internal map and per-identity state). RedisLimiter
// delegates concurrency safety to Redis and the go-redis client.
//
// # Context and Error Policy
//
// Allow accepts a context.Context. RedisLimiter passes this context through to
// Redis operations so callers can enforce deadlines and cancel work to avoid
// cascading failures during partial outages.
//
// This package does not impose a "fail open" vs "fail closed" policy. If Redis
// is unavailable or the context expires, Allow returns a non-nil error and the
// caller decides whether to deny traffic (protect the backend) or allow traffic
// (maximize availability).
//
// # Decision Semantics
//
// Decision fields are intended to be directly consumable by application code:
//
//   - Allow reports whether the current request is permitted.
//   - Remaining is the number of whole tokens remaining after the decision is
//     applied (floored to an int64).
//   - RetryAfter is 0 when allowed; when denied it is the approximate duration
//     until a single token is expected to be available.
//   - ResetTime is the absolute timestamp corresponding to time.Now()+RetryAfter.
//
// # Usage
//
// For a runnable example using MemoryLimiter, see ExampleMemoryLimiter in
// example_test.go.
//
// # Storage Details
//
// MemoryLimiter stores state in a process-local map keyed by:
//
//	"{namespace}:{key}"
//
// RedisLimiter stores state in Redis under keys prefixed with "limiter:" and
// uses a Redis hash with two fields:
//
//   - "tokens": current token balance (float)
//   - "last_refill": last update time as seconds since epoch (float)
//
// Redis keys are set to expire to avoid leaking memory for identities that stop
// sending requests.
//
// # Limitations and Notes
//
//   - MemoryLimiter does not evict old identities; for long-lived processes with
//     high-cardinality keys you likely want RedisLimiter or a custom in-memory
//     store with TTL/LRU eviction.
//   - RedisLimiter requires a reachable Redis instance and returns errors
//     directly; callers must decide their availability vs protection tradeoff.
//   - This package currently models each Allow call as a cost of 1 token.
//   - RedisLimiter uses EVALSHA; if Redis is restarted and script cache is
//     cleared, Allow may return a NOSCRIPT error until the script is reloaded
//     (recreating the limiter via NewRedisLimiter will load it).
//
// # Configuration
//
// RedisLimiter is configured using the Functional Options pattern:
//
//	limiter, _ := NewRedisLimiter(client,
//		WithPrefix("myapp:rate:"),
//		WithTimeout(2*time.Second),
//		WithRecorder(myMetrics),
//	)
//
// Supported options:
//
//   - WithPrefix(string): Sets the key prefix (default "limiter:").
//   - WithTimeout(time.Duration): Sets the context timeout for Redis operations
//     (default 5s).
//   - WithRecorder(MetricsRecorder): Injects a custom metrics backend.
package limiter

package limiter

import (
	"context"
	_ "embed"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed token_bucket.lua
var tokenBucketScript string

// RedisLimiter is a distributed rate limiter backed by Redis.
//
// It uses a Lua script to perform the token-bucket update atomically, which
// allows multiple application instances to enforce a single shared limit.
type RedisLimiter struct {
	client    *redis.Client
	scriptSHA string
	recorder  MetricsRecorder 
}

// NewRedisLimiter validates connectivity and loads the embedded Lua script into
// Redis (SCRIPT LOAD). The returned limiter is ready to use.
func NewRedisLimiter(client *redis.Client) (*RedisLimiter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	sha, err := client.ScriptLoad(ctx, tokenBucketScript).Result()
	if err != nil {
		return nil, err
	}

	return &RedisLimiter{
		client:    client,
		scriptSHA: sha,
		recorder:  &NoOpMetricsRecorder{},
	}, nil
}

// SetRecorder allows the caller to inject a real metrics collector (e.g., Prometheus)
func (r *RedisLimiter) SetRecorder(rec MetricsRecorder) {
	if rec != nil {
		r.recorder = rec
	}
}

// Allow checks whether a request for the given identity should be allowed under
// the provided limit. Each call has a fixed cost of 1 token.
func (r *RedisLimiter) Allow(ctx context.Context, id Identity, limit Limit) (Decision, error) {
	// 0. Instrumentation Setup
	start := time.Now()
	status := "error" // Default status if we fail before decision
	
	// We use a closure for defer so it captures the final value of 'status'
	defer func() {
		r.recorder.Observe("ratelimit.latency", time.Since(start).Seconds(), map[string]string{
			"namespace": string(id.Namespace),
			"status":    status,
		})
	}()
	
	// 1. Prepare Inputs
	key := "limiter:" + string(id.Namespace) + ":" + id.Key
	now := float64(time.Now().UnixMicro()) / 1e6
	cost := 1.0
	ratePerSecond := float64(limit.Rate) / limit.Period.Seconds()

	cmd := r.client.EvalSha(ctx, r.scriptSHA, []string{key},
		ratePerSecond, // ARGV[1]
		limit.Burst,   // ARGV[2]
		now,           // ARGV[3]
		cost,          // ARGV[4]
	)

	result, err := cmd.Result()
	if err != nil {
		// Record the error explicitly
		r.recorder.Add("ratelimit.errors", 1, map[string]string{
			"namespace": string(id.Namespace),
			"type":      "redis_eval",
		})
		return Decision{}, err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 4 {
		r.recorder.Add("ratelimit.errors", 1, map[string]string{
			"namespace": string(id.Namespace),
			"type":      "invalid_format",
		})
		return Decision{}, errors.New("invalid lua response format")
	}

	allowedVal := int64(convertToFloat(values[0]))
	remainingVal := int64(convertToFloat(values[1]))

	retryAfterFloat := convertToFloat(values[2])
	resetTimeFloat := convertToFloat(values[3])

	if allowedVal == 1 {
		status = "allowed"
	} else {
		status = "denied"
	}
	
	r.recorder.Add("ratelimit.call", 1, map[string]string{
		"namespace": string(id.Namespace),
		"status":    status,
	})

	return Decision{
		Allow:      allowedVal == 1,
		Remaining:  remainingVal,
		RetryAfter: time.Duration(retryAfterFloat * float64(time.Second)),
		ResetTime:  time.UnixMicro(int64(resetTimeFloat * 1e6)),
	}, nil
}

func convertToFloat(val interface{}) float64 {
	switch v := val.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

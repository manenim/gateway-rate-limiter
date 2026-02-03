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

type RedisLimiter struct {
	client    *redis.Client
	scriptSHA string
}

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
	}, nil
}

func (r *RedisLimiter) Allow(ctx context.Context, id Identity, limit Limit) (Decision, error) {
	// 1. Prepare Inputs
	key := "limiter:" + string(id.Namespace) + ":" + id.Key
	now := float64(time.Now().UnixMicro()) / 1e6 
	cost := 1.0 

	cmd := r.client.EvalSha(ctx, r.scriptSHA, []string{key},
		limit.Rate,   // ARGV[1]
		limit.Burst,  // ARGV[2]
		now,          // ARGV[3]
		cost,         // ARGV[4]
	)

	result, err := cmd.Result()
	if err != nil {
		return Decision{}, err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 4 {
		return Decision{}, errors.New("invalid lua response format")
	}
	
	allowedVal, _ := values[0].(int64)
	remainingVal, _ := values[1].(int64)
    
    retryAfterFloat := convertToFloat(values[2])
    resetTimeFloat := convertToFloat(values[3])

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
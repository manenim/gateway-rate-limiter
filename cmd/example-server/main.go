package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/manenim/gateway-rate-limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	opts := &redis.Options{Addr: redisAddr, ContextTimeoutEnabled: true}
	client := redis.NewClient(opts)
	defer client.Close()

	l, err := limiter.NewRedisLimiter(client,
		limiter.WithPrefix("demo:"),
		limiter.WithTimeout(100*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/ping", pingHandler(l))

	log.Printf("Server listening on :8080 (Redis: %s)", redisAddr)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// The demo enforces a quota per directly connected IP. Deployments behind a
// proxy need a trusted proxy policy before using forwarded identity headers.
func pingHandler(l limiter.RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid client address", http.StatusBadRequest)
			return
		}
		id := limiter.Identity{Namespace: "ip", Key: ip}
		limit := limiter.Limit{Rate: 5, Period: time.Second, Burst: 10}
		dec, err := l.Allow(r.Context(), id, limit)
		if err != nil {
			// This demonstration explicitly chooses fail open on Redis errors.
			log.Printf("Limiter error: %v", err)
		} else if !dec.Allow {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", math.Ceil(dec.RetryAfter.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("Rate limit exceeded\n"))
			return
		}
		_, _ = w.Write([]byte("Pong!\n"))
	})
}

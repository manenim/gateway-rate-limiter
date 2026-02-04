package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/manenim/gateway-rate-limiter/pkg/limiter"
	"github.com/redis/go-redis/v9"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	opts := &redis.Options{Addr: redisAddr}
	client := redis.NewClient(opts)

	l, err := limiter.NewRedisLimiter(client,
		limiter.WithPrefix("demo:"),
		limiter.WithTimeout(100*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Rate Limit: 5 req/sec (Burst 10) per IP
		ip := r.RemoteAddr
		id := limiter.Identity{Namespace: "ip", Key: ip}
		limit := limiter.Limit{Rate: 5, Period: time.Second, Burst: 10}

		dec, err := l.Allow(ctx, id, limit)
		if err != nil {
			// Fail Open or Closed? Here we Fail Open (allow traffic on error)
			log.Printf("Limiter error: %v", err)
		} else if !dec.Allow {
			w.Header().Set("Retry-After", fmt.Sprintf("%.2f", dec.RetryAfter.Seconds()))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded\n"))
			return
		}

		w.Write([]byte("Pong!\n"))
	})

	log.Printf("Server listening on :8080 (Redis: %s)", redisAddr)
	http.ListenAndServe(":8080", nil)
}

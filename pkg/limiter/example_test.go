package limiter

import (
	"context"
	"fmt"
	"time"
)

func ExampleMemoryLimiter() {
	l := NewMemoryLimiter()

	limit := Limit{
		Rate:   10,
		Period: time.Second,
		Burst:  10,
	}
	id := Identity{Namespace: "user", Key: "user_123"}

	dec, err := l.Allow(context.Background(), id, limit)
	if err != nil {
		panic(err)
	}

	fmt.Println(dec.Allow)
	// Output:
	// true
}

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	limiter "github.com/manenim/gateway-rate-limiter"
)

type stubLimiter struct {
	decision limiter.Decision
	err      error
	ids      []limiter.Identity
}

func (l *stubLimiter) Allow(_ context.Context, id limiter.Identity, _ limiter.Limit) (limiter.Decision, error) {
	l.ids = append(l.ids, id)
	return l.decision, l.err
}

func TestPingHandler_ClientIPIdentity(t *testing.T) {
	l := &stubLimiter{decision: limiter.Decision{Allow: true}}
	handler := pingHandler(l)
	for _, addr := range []string{"192.0.2.1:12345", "192.0.2.1:54321", "[2001:db8::1]:12345"} {
		r := httptest.NewRequest(http.MethodGet, "/ping", nil)
		r.RemoteAddr = addr
		r.Header.Set("X-Forwarded-For", "spoofed.example")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
	}
	if l.ids[0] != l.ids[1] || l.ids[0].Key != "192.0.2.1" || l.ids[2].Key != "2001:db8::1" {
		t.Fatalf("identities = %+v", l.ids)
	}
}

func TestPingHandler_DenialAndFailurePolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		decision   limiter.Decision
		err        error
		status     int
		retryAfter string
	}{
		{"denied", limiter.Decision{RetryAfter: 1250 * time.Millisecond}, nil, http.StatusTooManyRequests, "2"},
		{"dependency failure", limiter.Decision{}, errors.New("Redis unavailable"), http.StatusOK, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := &stubLimiter{decision: tc.decision, err: tc.err}
			r := httptest.NewRequest(http.MethodGet, "/ping", nil)
			r.RemoteAddr = "192.0.2.1:12345"
			w := httptest.NewRecorder()
			pingHandler(l).ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Retry-After") != tc.retryAfter {
				t.Fatalf("status=%d Retry-After=%q", w.Code, w.Header().Get("Retry-After"))
			}
		})
	}
}

package http

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type readyFunc func(context.Context) error

func (f readyFunc) Ready(ctx context.Context) error { return f(ctx) }

func TestHealthRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		failed       bool
		code         int
	}{
		{"GET", "/health/live", true, 200},
		{"GET", "/health/ready", false, 200},
		{"GET", "/health/ready", true, 503},
		{"POST", "/health/ready", false, 405},
		{"GET", "/api/v1/pages", false, 404},
	} {
		called := false
		handler := NewHandler(readyFunc(func(ctx context.Context) error {
			called = true
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second {
				t.Fatal("missing bounded context")
			}
			if tc.failed {
				return errors.New("postgres://secret")
			}
			return nil
		}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "secret") {
			t.Fatal("secret leaked")
		}
		if tc.path == "/health/live" && called {
			t.Fatal("liveness depends on infrastructure")
		}
	}
}

func TestReadinessPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewHandler(readyFunc(func(ctx context.Context) error { return ctx.Err() }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatal("request cancellation lost")
	}
}

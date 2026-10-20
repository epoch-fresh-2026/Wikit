// Package http maps Core health outcomes to HTTP responses.
package http

import (
	"context"
	"net/http"
	"time"
)

type Readiness interface{ Ready(context.Context) error }

func NewHandler(readiness Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { reply(w, http.StatusOK, "ok") })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if readiness.Ready(ctx) != nil {
			reply(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		reply(w, http.StatusOK, "ok")
	})
	return mux
}

func reply(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// net/http owns response write errors; no internal details enter this payload.
	_, _ = w.Write([]byte("{\"status\":\"" + state + "\"}\n"))
}

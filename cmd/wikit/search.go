package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

func runSearchWorker(ctx context.Context, worker *search.Worker) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(0)
		defer timer.Stop()
		failed := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
				err := worker.Process(attempt, 20)
				cancel()
				if err != nil && ctx.Err() == nil && !failed {
					fmt.Fprintln(os.Stderr, "search projection degraded; retries scheduled")
				}
				failed = err != nil
				timer.Reset(time.Second)
			}
		}
	}()
	return done
}

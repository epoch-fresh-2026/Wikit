// Command wikit is the composition root for the backend foundation.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	"github.com/epoch-fresh-2026/Wikit/internal/config"
	"github.com/epoch-fresh-2026/Wikit/internal/core/health"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wikit serve|migrate")
	}
	cfg, err := config.Load(os.Getenv, args[0])
	if err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := postgres.Open(startup, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	if args[0] == "migrate" {
		migration, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if err := db.Migrate(migration); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "migrations up to date")
		return nil
	}
	readiness := health.New(db, blobfs.Probe{Root: cfg.MediaRoot})
	readyCtx, readyCancel := context.WithTimeout(ctx, 2*time.Second)
	err = readiness.Ready(readyCtx)
	readyCancel()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return errors.New("HTTP listener failed")
	}
	server := &http.Server{
		Handler: httpadapter.NewHandler(readiness), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server failed")
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			closeErr := server.Close()
			return errors.Join(errors.New("HTTP shutdown failed"), closeErr)
		}
		if err := <-finished; !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server failed")
		}
		return nil
	}
}

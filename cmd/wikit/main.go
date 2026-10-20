// Command wikit is the composition root for the backend foundation.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	mcpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/mcp"
	"github.com/epoch-fresh-2026/Wikit/internal/config"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/health"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/epoch-fresh-2026/Wikit/internal/search/meili"
	"github.com/epoch-fresh-2026/Wikit/internal/security"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
	"golang.org/x/term"
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
		return errors.New("usage: wikit serve|migrate|create-admin|blob-gc|search-rebuild|search-retry|search-status")
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
	// Administrative writes must enforce the same schema compatibility as serve.
	startup, cancel = context.WithTimeout(ctx, 10*time.Second)
	err = db.Check(startup)
	cancel()
	if err != nil {
		return err
	}
	if args[0] == "blob-gc" {
		files, err := blobfs.New(cfg.MediaRoot)
		if err != nil {
			return err
		}
		defer func() {
			if err := files.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "media store close failed")
			}
		}()
		out, err := media.New(db, files, security.Secrets{}, cfg.MaxUploadBytes).Collect(ctx, cfg.GCGrace)
		fmt.Fprintf(os.Stdout, "collected blobs=%d orphan_files=%d\n", out.Blobs, out.Orphans)
		return err
	}
	index := meili.New(cfg.MeiliURL, cfg.MeiliKey)
	searchWorker := search.NewWorker(db, index, textadapter.Parser{}, security.Secrets{})
	if strings.HasPrefix(args[0], "search-") {
		switch args[0] {
		case "search-rebuild":
			name, err := searchWorker.Rebuild(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, "active_index="+name)
		case "search-retry":
			if err := searchWorker.Retry(ctx); err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, "search retries scheduled")
		case "search-status":
			stats, err := searchWorker.Status(ctx)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(os.Stdout).Encode(stats); err != nil {
				return errors.New("status output failed")
			}
		}
		return nil
	}
	passwords, err := security.NewPasswords()
	if err != nil {
		return err
	}
	secrets := security.Secrets{}
	accounts := identity.New(db, passwords, secrets, identity.Options{Registration: cfg.Registration, SessionTTL: cfg.SessionTTL})
	if args[0] == "create-admin" {
		return createAdmin(ctx, accounts, secrets)
	}
	files, err := blobfs.New(cfg.MediaRoot)
	if err != nil {
		return err
	}
	defer func() {
		if err := files.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "media store close failed")
		}
	}()
	if err := db.WithinAccess(ctx, func(tx access.Tx) error {
		count, err := tx.ActiveAdmins(ctx)
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("initialize an administrator with wikit create-admin before serving")
		}
		return nil
	}); err != nil {
		return err
	}
	readiness := health.New(db, blobfs.Probe{Root: cfg.MediaRoot})
	readyCtx, readyCancel := context.WithTimeout(ctx, 2*time.Second)
	err = readiness.Ready(readyCtx)
	readyCancel()
	if err != nil {
		return err
	}
	pageService := pages.New(db, secrets, textadapter.Parser{}, files)
	proposalService := proposals.New(db, secrets, textadapter.Parser{}, textadapter.Differ{}, files)
	discussionService := discussions.New(db, secrets, textadapter.Parser{})
	mediaService := media.New(db, files, secrets, cfg.MaxUploadBytes)
	searchService := search.New(db, index, textadapter.Parser{}, secrets)
	accessService := access.New(db, secrets)
	mcpEndpoint, err := mcpadapter.New(mcpadapter.Services{Identity: accounts, Access: accessService, Pages: pageService, Proposals: proposalService, Discussions: discussionService, Media: mediaService, Search: searchService, Secrets: secrets, PublicURL: cfg.PublicURL})
	if err != nil {
		return err
	}
	defer func() {
		if err := mcpEndpoint.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "MCP session shutdown failed")
		}
	}()
	router := http.NewServeMux()
	router.Handle("/mcp", mcpEndpoint)
	router.Handle("/", (httpadapter.API{Search: searchService, Discussions: discussionService, Identity: accounts, Access: accessService, Pages: pageService, Proposals: proposalService, Media: mediaService, Secrets: secrets, PublicURL: cfg.PublicURL, SessionTTL: cfg.SessionTTL}).Handler(httpadapter.NewHandler(readiness)))
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return errors.New("HTTP listener failed")
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := runSearchWorker(workerCtx, searchWorker)
	defer func() { stopWorker(); <-workerDone }()
	server := &http.Server{
		Handler: router, ReadHeaderTimeout: 5 * time.Second,
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

func createAdmin(ctx context.Context, accounts *identity.Service, secrets identity.Secrets) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("create-admin requires an interactive terminal")
	}
	fmt.Fprint(os.Stdout, "Username: ")
	username, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return errors.New("cannot read username")
	}
	fmt.Fprint(os.Stdout, "Password: ")
	password, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return errors.New("cannot read password")
	}
	defer clear(password)
	fmt.Fprint(os.Stdout, "Confirm password: ")
	confirm, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return errors.New("cannot read password confirmation")
	}
	defer clear(confirm)
	if string(password) != string(confirm) {
		return errors.New("passwords do not match")
	}
	id, err := secrets.ID()
	if err != nil {
		return err
	}
	if _, err := accounts.CreateAdmin(ctx, strings.TrimSpace(username), string(password), id); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "administrator created; log in to rotate a personal Token")
	return nil
}

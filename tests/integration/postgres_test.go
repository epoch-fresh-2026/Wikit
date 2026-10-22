//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	"github.com/epoch-fresh-2026/Wikit/internal/core/health"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
)

type database struct {
	store *postgres.Store
	conn  *pgx.Conn
	url   string
}

// Each case owns a database: tests never migrate or delete the database named
// in WIKIT_DATABASE_URL. The configured account needs CREATEDB privileges.
func newDatabase(t *testing.T) database {
	t.Helper()
	raw := os.Getenv("WIKIT_DATABASE_URL")
	if raw == "" {
		t.Fatal("WIKIT_DATABASE_URL is required for real PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("cannot connect to test PostgreSQL")
	}
	name := fmt.Sprintf("wikit_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		if closeErr := admin.Close(ctx); closeErr != nil {
			t.Log(closeErr)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
		if err := admin.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	u.Path = "/" + name
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal("cannot connect to isolated database")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	store, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return database{store, conn, u.String()}
}

func execSQL(t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationAndReadiness(t *testing.T) {
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.store.Check(ctx); err == nil {
		t.Fatal("unmigrated database ready")
	}
	service := health.New(db.store, blobfs.Probe{Root: t.TempDir()})
	h := httpadapter.NewHandler(service)
	probe := func(code int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
		if w.Code != code {
			t.Fatalf("expected %d, got %d", code, w.Code)
		}
	}
	probe(503)
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal("repeat migration", err)
	}
	probe(200)
	var count int
 if err := db.conn.QueryRow(ctx, "SELECT count(*) FROM public.wikit_schema_migrations").Scan(&count); err != nil || count != 9 {
		t.Fatal("unexpected migration count", count, err)
	}
	execSQL(t, db.conn, "UPDATE public.wikit_schema_migrations SET checksum = repeat('0',64)")
	probe(503)
	if err := db.store.Migrate(ctx); err == nil {
		t.Fatal("accepted edited history")
	}
}

func TestConcurrentMigrations(t *testing.T) {
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	other, err := postgres.Open(ctx, db.url)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*postgres.Store{db.store, other} {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errors <- store.Migrate(ctx) }()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
 if err := db.conn.QueryRow(ctx, "SELECT count(*) FROM public.wikit_schema_migrations").Scan(&count); err != nil || count != 9 {
		t.Fatal(count, err)
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	db := newDatabase(t)
	// A pre-existing schema makes 0001 fail after the history table was created.
	execSQL(t, db.conn, "CREATE SCHEMA wikit")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.store.Migrate(ctx); err == nil {
		t.Fatal("expected DDL failure")
	}
	var absent bool
	if err := db.conn.QueryRow(ctx, "SELECT to_regclass('public.wikit_schema_migrations') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("partial history committed", err)
	}
	execSQL(t, db.conn, "DROP SCHEMA wikit")
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal("failed migration not recoverable", err)
	}
}

func TestIncompatibleHistories(t *testing.T) {
	for _, sql := range []string{
  "INSERT INTO public.wikit_schema_migrations VALUES (10,'future',repeat('a',64),now())",
  "UPDATE public.wikit_schema_migrations SET version=10 WHERE version=1",
		"UPDATE public.wikit_schema_migrations SET name='renamed' WHERE version=1",
		"DELETE FROM public.wikit_schema_migrations",
	} {
		t.Run(strings.Fields(sql)[0]+fmt.Sprint(len(sql)), func(t *testing.T) {
			db := newDatabase(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := db.store.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			execSQL(t, db.conn, sql)
			if err := db.store.Check(ctx); err == nil {
				t.Fatal("incompatible schema ready")
			}
			if err := db.store.Migrate(ctx); err == nil {
				t.Fatal("accepted incompatible history")
			}
		})
	}
}

func TestMigrationLockCancellation(t *testing.T) {
	db := newDatabase(t)
	execSQL(t, db.conn, "BEGIN; SELECT pg_advisory_xact_lock(8734281001)")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := db.store.Migrate(ctx); err == nil {
		t.Fatal("ignored locked migration timeout")
	}
	if ctx.Err() == nil {
		t.Fatal("migration did not wait for held lock")
	}
	execSQL(t, db.conn, "ROLLBACK")
	retry, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := db.store.Migrate(retry); err != nil {
		t.Fatal("lock leaked", err)
	}
	if err := db.store.Check(retry); err != nil {
		t.Fatal(err)
	}
}

func TestUnavailableDependencies(t *testing.T) {
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service := health.New(db.store, blobfs.Probe{Root: t.TempDir() + "/missing"})
	if err := service.Ready(ctx); err == nil {
		t.Fatal("missing media directory ready")
	}
	db.store.Close()
	if err := db.store.Check(ctx); err == nil {
		t.Fatal("closed database ready")
	}
}

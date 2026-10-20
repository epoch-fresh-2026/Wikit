package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationLock int64 = 8734281001

type migration struct {
	version             int
	name, checksum, sql string
}

func migrations() ([]migration, error) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(`^migrations/([0-9]{4})_[a-z0-9_]+\.sql$`)
	var result []migration
	for i, name := range names {
		match := pattern.FindStringSubmatch(name)
		if match == nil {
			return nil, errors.New("invalid migration filename")
		}
		version, err := strconv.Atoi(match[1])
		if err != nil || version != i+1 {
			return nil, errors.New("migration versions must be contiguous from 0001")
		}
		data, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, err
		}
		result = append(result, migration{version, name, fmt.Sprintf("%x", sha256.Sum256(data)), string(data)})
	}
	if len(result) == 0 {
		return nil, errors.New("missing migrations")
	}
	return result, nil
}

type historyReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func history(ctx context.Context, db historyReader, expected []migration) (int, error) {
	rows, err := db.Query(ctx, "SELECT version, name, checksum FROM public.wikit_schema_migrations ORDER BY version")
	if err != nil {
		return 0, errors.New("migration history unavailable; run wikit migrate")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return 0, errors.New("invalid migration history")
		}
		if count >= len(expected) || version != expected[count].version || name != expected[count].name || checksum != expected[count].checksum {
			return 0, errors.New("incompatible migration history")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, errors.New("migration history read failed")
	}
	return count, nil
}

func (s *Store) checkHistory(ctx context.Context) error {
	expected, err := migrations()
	if err != nil {
		return err
	}
	count, err := history(ctx, s.pool, expected)
	if err != nil {
		return err
	}
	if count != len(expected) {
		return errors.New("pending migrations; run wikit migrate")
	}
	return nil
}

// Migrate serializes runners in PostgreSQL and commits all pending SQL with its
// history atomically. serve never invokes this method.
func (s *Store) Migrate(ctx context.Context) (result error) {
	expected, err := migrations()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.New("migration transaction failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			result = errors.Join(result, errors.New("migration rollback failed"))
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
		return errors.New("migration lock failed")
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.wikit_schema_migrations (
		version integer PRIMARY KEY CHECK (version > 0), name text NOT NULL UNIQUE,
		checksum text NOT NULL CHECK (length(checksum) = 64),
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return errors.New("migration history initialization failed")
	}
	count, err := history(ctx, tx, expected)
	if err != nil {
		return err
	}
	for _, m := range expected[count:] {
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %04d failed", m.version)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO public.wikit_schema_migrations(version,name,checksum) VALUES ($1,$2,$3)", m.version, m.name, m.checksum); err != nil {
			return errors.New("migration history write failed")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("migration commit failed")
	}
	return nil
}

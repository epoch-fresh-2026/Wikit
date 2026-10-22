//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
)

func TestPageCursorUpgradePreservesOffsetCursors(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	// Build the actual previous schema, including its checksummed history. Do not
	// simulate an upgrade by deleting a column from an already upgraded database.
	execSQL(t, db.conn, `CREATE TABLE public.wikit_schema_migrations (
 version integer PRIMARY KEY CHECK(version>0), name text NOT NULL UNIQUE,
 checksum text NOT NULL CHECK(length(checksum)=64), applied_at timestamptz NOT NULL DEFAULT now())`)
	files, err := filepath.Glob("../../internal/storage/postgres/migrations/000[1-8]_*.sql")
	if err != nil || len(files) != 8 {
		t.Fatal("previous migration files", files, err)
	}
	for n, path := range files {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		execSQL(t, db.conn, string(sql))
		if _, err := db.conn.Exec(ctx, "INSERT INTO public.wikit_schema_migrations(version,name,checksum) VALUES($1,$2,$3)", n+1, "migrations/"+filepath.Base(path), fmt.Sprintf("%x", sha256.Sum256(sql))); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"pages", "tags", "search", "accounts"} {
		// This is the six-column insert used by the previous binary.
		if _, err := db.conn.Exec(ctx, "INSERT INTO wikit.list_cursors VALUES($1,NULL,$2,20,42,now()+interval '10 minutes')", identity.Digest("old-"+kind), kind); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.store.Check(ctx); err == nil {
		t.Fatal("pending migration reported ready")
	}
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal("upgrade", err)
	}
	if err := db.store.Migrate(ctx); err != nil {
		t.Fatal("repeat upgrade", err)
	}
	if err := db.store.Check(ctx); err != nil {
		t.Fatal("upgraded schema not ready", err)
	}
	if err := db.store.WithinPages(ctx, func(tx pages.Tx) error {
		if _, err := tx.PageAnchor(ctx, "old-pages", "", "pages", 20); !errors.Is(err, fault.BadRequest) {
			t.Fatalf("old directory cursor accepted: %v", err)
		}
		for _, kind := range []string{"tags", "search", "accounts"} {
			position, err := tx.Cursor(ctx, "old-"+kind, "", kind, 20)
			if err != nil || position != 42 {
				t.Fatalf("old %s cursor changed: %d %v", kind, position, err)
			}
		}
		if _, err := tx.NewCursor(ctx, "new-offset", "", "tags", 43, 20); err != nil {
			return err
		}
		if position, err := tx.Cursor(ctx, "new-offset", "", "tags", 20); err != nil || position != 43 {
			t.Fatalf("new offset cursor: %d %v", position, err)
		}
		const anchor = "9223372036854775807"
		if _, err := tx.NewPageCursor(ctx, "new-page", "", "pages", anchor, 20); err != nil {
			return err
		}
		if got, err := tx.PageAnchor(ctx, "new-page", "", "pages", 20); err != nil || got != anchor {
			t.Fatalf("new page anchor lost precision: %s %v", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

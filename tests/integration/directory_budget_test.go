//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestDirectoryScanBudgetAndEmptyContinuation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	tx, err := s.db.conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE directory_fixture AS SELECT n,nextval('wikit.page_ids') p,gen_random_uuid() r FROM generate_series(1,1005) n;
 INSERT INTO wikit.page_policies SELECT p,CASE WHEN n=1005 THEN 'public' ELSE 'restricted' END,'published',1 FROM directory_fixture;
 INSERT INTO wikit.pages SELECT p,'document',NULL FROM directory_fixture;
 INSERT INTO wikit.revisions(id,page_id,type,source_kind,snapshot,author_id,publisher_id,created_at,sealed)
 SELECT r,p,'document','create',jsonb_build_object('type','document','title','directory','tags','[]'::jsonb,'markdown','body'),
 (SELECT id FROM wikit.users WHERE username='local_admin'),(SELECT id FROM wikit.users WHERE username='local_admin'),now(),true FROM directory_fixture;
 UPDATE wikit.pages p SET head_revision_id=f.r FROM directory_fixture f WHERE p.id=f.p;`)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	q := pages.Query{Kind: "pages", Limit: 1}
	first, err := svc.List(ctx, identity.Credentials{}, q)
	if err != nil || len(first.Items) != 1 || first.Next == nil {
		t.Fatal("home", err)
	}
	q.Cursor = *first.Next
	empty, err := svc.List(ctx, identity.Credentials{}, q)
	if err != nil || len(empty.Items) != 0 || empty.Next == nil {
		t.Fatal("scan budget must return empty continuation", err)
	}
	var anchor string
	if err := s.db.conn.QueryRow(ctx, "SELECT page_anchor::text FROM wikit.list_cursors WHERE digest=$1", identity.Digest(*empty.Next)).Scan(&anchor); err != nil || anchor != "1001" {
		t.Fatal("unbounded or incorrect scan", anchor, err)
	}
	q.Cursor = *empty.Next
	final, err := svc.List(ctx, identity.Credentials{}, q)
	if err != nil || len(final.Items) != 1 || final.Next != nil {
		t.Fatal("did not resume after invisible candidates", err)
	}
	var page pages.Detail
	if err := json.Unmarshal(final.Items[0], &page); err != nil || page.Page.ID != "1006" {
		t.Fatal("wrong final page", err)
	}
	// A migration-era offset cursor must fail rather than restart or expose data.
	if _, err := s.db.conn.Exec(ctx, "UPDATE wikit.list_cursors SET page_anchor=NULL WHERE digest=$1", identity.Digest(q.Cursor)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, identity.Credentials{}, q); !errors.Is(err, fault.BadRequest) {
		t.Fatal("legacy cursor accepted", err)
	}
}

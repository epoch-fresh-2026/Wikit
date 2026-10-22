//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestSearchCursorUsesConsumedCandidateAndResumesScanBudget(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	ps := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	valid := []search.Candidate{}
	for n := 0; n < 4; n++ {
		p, err := ps.Create(ctx, requestKey(identity.Credentials{Bearer: s.adminToken}, fmt.Sprintf("boundary-page-%d", n)), "public", document("boundary", "body"), "boundary")
		if err != nil {
			t.Fatal(err)
		}
		valid = append(valid, search.Candidate{ID: p.Page.ID, Revision: p.Revision.ID, PolicyVersion: 1})
	}
	missing := search.Candidate{ID: "9223372036854775807", Revision: s.id(t), PolicyVersion: 1}
	t.Run("partially consumed batch", func(t *testing.T) {
		index := &scriptedSearch{hits: []search.Candidate{valid[0], missing, missing, missing, valid[1], valid[2], valid[3]}}
		service := search.New(s.db.store, index, textadapter.Parser{}, s.secrets)
		q := search.Query{Q: "boundary", Limit: 2}
		first, err := service.Search(ctx, identity.Credentials{}, q)
		if err != nil || len(first.Items) != 2 || first.Next == nil {
			t.Fatal("first page", first, err)
		}
		if first.Items[0].PageID != valid[0].ID || first.Items[1].PageID != valid[1].ID {
			t.Fatal("candidate order changed", first.Items)
		}
		var position int
		if err := s.db.conn.QueryRow(ctx, "SELECT position FROM wikit.list_cursors WHERE digest=$1", identity.Digest(*first.Next)).Scan(&position); err != nil || position != 5 {
			t.Fatal("cursor skipped unconsumed candidates", position, err)
		}
		q.Cursor = *first.Next
		last, err := service.Search(ctx, identity.Credentials{}, q)
		if err != nil || len(last.Items) != 2 || last.Next != nil {
			t.Fatal("last page", last, err)
		}
		if last.Items[0].PageID != valid[2].ID || last.Items[1].PageID != valid[3].ID {
			t.Fatal("omitted or duplicated results", last.Items)
		}
	})
	t.Run("scan budget", func(t *testing.T) {
		index := &scriptedSearch{}
		for n := 0; n < 1000; n++ {
			index.hits = append(index.hits, missing)
		}
		index.hits = append(index.hits, valid[0])
		service := search.New(s.db.store, index, textadapter.Parser{}, s.secrets)
		q := search.Query{Q: "boundary", Limit: 2}
		first, err := service.Search(ctx, identity.Credentials{}, q)
		if err != nil || len(first.Items) != 0 || first.Next == nil {
			t.Fatal("scan budget did not yield empty continuation", first, err)
		}
		var position int
		if err := s.db.conn.QueryRow(ctx, "SELECT position FROM wikit.list_cursors WHERE digest=$1", identity.Digest(*first.Next)).Scan(&position); err != nil || position != 1000 {
			t.Fatal("incorrect scan budget", position, err)
		}
		q.Cursor = *first.Next
		last, err := service.Search(ctx, identity.Credentials{}, q)
		if err != nil || len(last.Items) != 1 || last.Next != nil || last.Items[0].PageID != valid[0].ID {
			t.Fatal("failed to resume past rejected candidates", last, err)
		}
	})
}

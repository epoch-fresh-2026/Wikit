//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

type scriptedSearch struct {
	search.Index
	hits             []search.Candidate
	calls            atomic.Int64
	entered, release chan struct{}
}

func (i *scriptedSearch) Search(ctx context.Context, _, _ string, _ search.Filter, offset, limit int) (search.Candidates, error) {
	call := i.calls.Add(1)
	if call == 2 && i.entered != nil {
		close(i.entered)
		select {
		case <-i.release:
		case <-ctx.Done():
			return search.Candidates{}, ctx.Err()
		}
	}
	end := min(offset+limit, len(i.hits))
	return search.Candidates{Hits: i.hits[offset:end], More: end < len(i.hits)}, nil
}

func TestSearchBatchStopsWhenFullAndUsesConstantSQL(t *testing.T) {
	s := newSuite(t)
	svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	index := &scriptedSearch{}
	for n := 0; n < 9; n++ {
		p, err := svc.Create(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, fmt.Sprintf("batch-seed-%d", n)), "public", document("batch", "body"), fmt.Sprintf("batch-ip-%d", n))
		if err != nil {
			t.Fatal(err)
		}
		index.hits = append(index.hits, search.Candidate{ID: p.Page.ID, Revision: p.Revision.ID, PolicyVersion: 1})
	}
	searcher := search.New(s.db.store, index, textadapter.Parser{}, s.secrets)
	var small int64
	for _, limit := range []int{2, 8} {
		index.calls.Store(0)
		ctx, m := postgres.Measure(context.Background())
		out, err := searcher.Search(ctx, identity.Credentials{}, search.Query{Q: "batch", Limit: limit})
		if err != nil || len(out.Items) != limit || out.Next == nil {
			t.Fatalf("result %d %v", len(out.Items), err)
		}
		if index.calls.Load() != 1 {
			t.Fatal("overfetched sufficient candidates")
		}
		if small == 0 {
			small = m.Queries.Load()
		} else if small != m.Queries.Load() {
			t.Fatal("SQL count grows with candidate count", small, m.Queries.Load())
		}
	}
}

func TestSearchRevalidatesSelectedCandidatesBetweenBatches(t *testing.T) {
	for _, change := range []string{"hidden", "policy", "revoked", "revision", "disabled", "index"} {
		t.Run(change, func(t *testing.T) {
			s := newSuite(t)
			actor, token := s.register(t, "batch_reader")
			c := identity.Credentials{Bearer: token}
			admin := identity.Credentials{Bearer: s.adminToken}
			ps := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
			fixtures := []pages.Created{}
			for n := 0; n < 3; n++ {
				p, err := ps.Create(context.Background(), requestKey(admin, fmt.Sprintf("race-batch-%d", n)), "public", document("batch", "private-at-return"), "batch-race")
				if err != nil {
					t.Fatal(err)
				}
				fixtures = append(fixtures, p)
			}
			i := &scriptedSearch{entered: make(chan struct{}), release: make(chan struct{})}
			for n, p := range fixtures {
				i.hits = append(i.hits, search.Candidate{ID: p.Page.ID, Revision: p.Revision.ID, PolicyVersion: 1})
				if n == 0 {
					i.hits = append(i.hits, search.Candidate{ID: "9223372036854775807", Revision: s.id(t), PolicyVersion: 1})
				}
			}
			var survivor identity.Account
			if change == "revoked" {
				survivor, _ = s.register(t, "batch_survivor")
				_, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "batch-initial-restriction"), fixtures[0].Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: actor.ID, Capabilities: []string{"read"}}, {SubjectType: "user", SubjectID: survivor.ID, Capabilities: []string{"read"}}}}, 1)
				if err != nil {
					t.Fatal(err)
				}
				i.hits[0].PolicyVersion = 2
			}
			service := search.New(s.db.store, i, textadapter.Parser{}, s.secrets)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			type result struct {
				out search.Result
				err error
			}
			done := make(chan result, 1)
			go func() {
				out, err := service.Search(ctx, c, search.Query{Q: "batch", Limit: 2})
				done <- result{out, err}
			}()
			awaitBarrier(t, i.entered)
			p := fixtures[0]
			switch change {
			case "revoked":
				_, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "batch-revoke-reader"), p.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: survivor.ID, Capabilities: []string{"read"}}}}, 2)
				if err != nil {
					t.Fatal(err)
				}
			case "hidden":
				if _, err := ps.Moderate(context.Background(), requestKey(admin, "batch-hide"), p.Page.ID, true, "test", 1); err != nil {
					t.Fatal(err)
				}
			case "policy":
				_, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "batch-restrict"), p.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: actor.ID, Capabilities: []string{"read"}}}}, 1)
				if err != nil {
					t.Fatal(err)
				}
				// Same actor still has read but the old indexed policy version is invalid.
			case "revision":
				proposal := s.createProposal(t, admin, p.Page.ID, p.Page.HeadRevisionID, document("batch", "new body"), "batch-new-proposal")
				s.publish(t, admin, proposal, p.Page.HeadRevisionID, "batch-publish")
			case "disabled":
				if _, err := s.access.UpdateAccount(context.Background(), requestKey(admin, "batch-disable"), actor.ID, "member", "disabled", 1, false); err != nil {
					t.Fatal(err)
				}
			case "index":
				execSQL(t, s.db.conn, "UPDATE wikit.search_state SET active_index='changed_generation'")
			}
			close(i.release)
			got := <-done
			if change == "disabled" {
				if got.err == nil || len(got.out.Items) != 0 {
					t.Fatal("disabled actor received data")
				}
				return
			}
			if change == "index" {
				if !errors.Is(got.err, fault.BadRequest) {
					t.Fatal("index generation not rejected", got.err)
				}
				return
			}
			if got.err != nil || len(got.out.Items) != 2 {
				t.Fatal("did not refill after revalidation", got.err, len(got.out.Items))
			}
			for _, hit := range got.out.Items {
				if hit.PageID == p.Page.ID {
					t.Fatal("prior batch leaked changed candidate")
				}
			}
		})
	}
}

func TestDirectoryKeysetTraversalAndCurrentAuthorization(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	// All IDs remain strings, including anchors larger than JS's safe integer.
	execSQL(t, s.db.conn, "SELECT setval('wikit.page_ids',9007199254740992)")
	ids := []string{"1"}
	for n := 0; n < 5; n++ {
		p, err := svc.Create(context.Background(), requestKey(admin, fmt.Sprintf("directory-seed-%d", n)), "public", document("directory", "body"), "directory-test")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.Page.ID)
	}
	q := pages.Query{Kind: "pages", Limit: 2}
	seen := map[string]bool{}
	for {
		out, err := svc.List(context.Background(), identity.Credentials{}, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range out.Items {
			var d pages.Detail
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			if seen[d.Page.ID] {
				t.Fatal("duplicate page")
			}
			seen[d.Page.ID] = true
		}
		if out.Next == nil {
			break
		}
		q.Cursor = *out.Next
	}
	if len(seen) != len(ids) {
		t.Fatal("missing pages", len(seen))
	}
	first, err := svc.List(context.Background(), identity.Credentials{}, pages.Query{Kind: "pages", Limit: 2})
	if err != nil || first.Next == nil {
		t.Fatal(err)
	}
	// Removing an earlier row must not shift the anchor and skip a later row.
	if _, err := svc.Moderate(context.Background(), requestKey(admin, "directory-hide-earlier"), ids[1], true, "test", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Moderate(context.Background(), requestKey(admin, "directory-hide-next"), ids[2], true, "test", 1); err != nil {
		t.Fatal(err)
	}
	q.Cursor = *first.Next
	q.Limit = 2
	next, err := svc.List(context.Background(), identity.Credentials{}, q)
	if err != nil || len(next.Items) != 2 {
		t.Fatal("keyset shifted", err)
	}
	for n, raw := range next.Items {
		var d pages.Detail
		_ = json.Unmarshal(raw, &d)
		if d.Page.ID != ids[n+3] {
			t.Fatal("wrong continuation", d.Page.ID)
		}
	}
	reader, _ := s.register(t, "directory_retained_reader")
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "directory-restrict-after-cursor"), ids[3], access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: reader.ID, Capabilities: []string{"read"}}}}, 1); err != nil {
		t.Fatal(err)
	}
	revoked, err := svc.List(context.Background(), identity.Credentials{}, q)
	if err != nil || len(revoked.Items) != 2 {
		t.Fatal("revocation continuation", err)
	}
	for n, raw := range revoked.Items {
		var d pages.Detail
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		if d.Page.ID != ids[n+4] {
			t.Fatal("revoked directory page returned")
		}
	}
	for _, bad := range []pages.Query{{Kind: "pages", Limit: 1, Cursor: q.Cursor}, {Kind: "pages", Limit: 2, Tag: "other", Cursor: q.Cursor}, {Kind: "pages", Limit: 2, Type: "media", Cursor: q.Cursor}} {
		if _, err := svc.List(context.Background(), identity.Credentials{}, bad); !errors.Is(err, fault.BadRequest) {
			t.Fatal("cursor binding", err)
		}
	}
	if _, err := svc.List(context.Background(), admin, q); !errors.Is(err, fault.BadRequest) {
		t.Fatal("cursor identity", err)
	}
	execSQL(t, s.db.conn, "UPDATE wikit.list_cursors SET expires_at=now()-interval '1 second'")
	if _, err := svc.List(context.Background(), identity.Credentials{}, q); !errors.Is(err, fault.BadRequest) {
		t.Fatal("expired cursor", err)
	}
}

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
)

func (s *suite) seedPolicy(t *testing.T, p access.Policy) {
	t.Helper()
	if err := s.db.store.WithinAccess(context.Background(), func(tx access.Tx) error { return tx.SavePolicy(context.Background(), p) }); err != nil {
		t.Fatal(err)
	}
}
func (s *suite) id(t *testing.T) string {
	t.Helper()
	id, err := s.secrets.ID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func TestACLMatrixGroupsAndGrantBoundaries(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	manager, managerToken := s.register(t, "page_manager")
	reader, readerToken := s.register(t, "page_reader")
	reviewer, reviewerToken := s.register(t, "global_reviewer")
	admin := identity.Credentials{Bearer: s.adminToken}
	m := identity.Credentials{Bearer: managerToken}
	r := identity.Credentials{Bearer: readerToken}
	v := identity.Credentials{Bearer: reviewerToken}
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "reviewer-role"), reviewer.ID, "moderator", "active", 1, false); err != nil {
		t.Fatal(err)
	}
	groupJSON, err := s.access.SaveGroup(ctx, requestKey(admin, "create-group"), "", "Readers", nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var group access.Group
	if err := json.Unmarshal(groupJSON, &group); err != nil {
		t.Fatal(err)
	}
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "group-members"), group.ID, "Readers", []string{reader.ID}, 1, false); err != nil {
		t.Fatal(err)
	}
	page := s.pageID(t)
	p := access.Policy{PageID: page, Visibility: "restricted", Status: "published", Version: 1, Grants: []access.Grant{{SubjectType: "user", SubjectID: manager.ID, Capabilities: []string{"read", "manage"}}, {SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read", "merge"}}}}
	s.seedPolicy(t, p)
	for _, tc := range []struct {
		c    identity.Credentials
		cap  string
		want error
	}{{identity.Credentials{}, "read", fault.NotFound}, {v, "read", fault.NotFound}, {m, "read", nil}, {m, "merge", fault.Forbidden}, {r, "merge", nil}, {admin, "manage", nil}} {
		if err := s.access.Check(ctx, tc.c, page, tc.cap, false); err != tc.want {
			t.Fatalf("%s: %v expected %v", tc.cap, err, tc.want)
		}
	}
	s.req(t, "GET", "pages/"+page+"/policy", nil, m, "", 200)
	s.req(t, "GET", "pages/"+page+"/policy", nil, r, "", 403)
	s.req(t, "GET", "pages/"+page+"/policy", nil, identity.Credentials{}, "", 404)
	expanded := access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: manager.ID, Capabilities: []string{"read", "manage", "merge"}}}}
	if _, err := s.access.UpdatePolicy(ctx, requestKey(m, "self-expansion"), page, expanded, 1); err != fault.Forbidden {
		t.Fatal("manager gained review", err)
	}
	expanded.Grants = []access.Grant{{SubjectType: "user", SubjectID: manager.ID, Capabilities: []string{"read", "manage"}}, {SubjectType: "user", SubjectID: reader.ID, Capabilities: []string{"read", "manage"}}}
	if _, err := s.access.UpdatePolicy(ctx, requestKey(m, "manage-expansion"), page, expanded, 1); err != fault.Forbidden {
		t.Fatal("manager granted manage", err)
	}
	expanded.Grants = []access.Grant{{SubjectType: "user", SubjectID: reader.ID, Capabilities: []string{"merge"}}}
	if _, err := s.access.UpdatePolicy(ctx, requestKey(admin, "missing-read"), page, expanded, 1); err != fault.Invalid {
		t.Fatal("non-read grant lacks read", err)
	}
	// Preserve another subject's existing review grant while editing visibility.
	p.Visibility = "public"
	data, err := s.access.UpdatePolicy(ctx, requestKey(m, "visibility-change"), page, p, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("missing policy result")
	}
	if err := s.access.Check(ctx, identity.Credentials{}, page, "read", false); err != nil {
		t.Fatal(err)
	}
	if err := s.access.Check(ctx, v, page, "merge", false); err != nil {
		t.Fatal("public reviewer", err)
	}
	// A separate restricted page demonstrates immediate group-based revocation.
	other := s.pageID(t)
	p.PageID = other
	p.Visibility = "restricted"
	p.Version = 1
	s.seedPolicy(t, p)
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "revoke-group"), group.ID, "Readers", nil, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := s.access.Check(ctx, r, other, "read", false); err != fault.NotFound {
		t.Fatal("group revocation stale", err)
	}
	p.Status = "hidden"
	p.PageID = page
	p.Visibility = "public"
	p.Version = 3
	s.seedPolicy(t, p)
	if err := s.access.Check(ctx, m, page, "read", false); err != fault.NotFound {
		t.Fatal("hidden content exposed")
	}
	if _, err := s.access.GetPolicy(ctx, m, page); err != nil {
		t.Fatal("manager cannot govern hidden page", err)
	}
	if err := s.access.Check(ctx, v, page, "read", true); err != fault.NotFound {
		t.Fatal("hidden title exposed")
	}
}

func waitForAdvisoryWaiter(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("no controlled advisory waiter")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
func TestACL03RevocationSerializesWithProtectedWrite(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	u, token := s.register(t, "concurrent_reviewer")
	c := identity.Credentials{Bearer: token}
	page := s.pageID(t)
	p := access.Policy{PageID: page, Visibility: "restricted", Status: "published", Version: 1, Grants: []access.Grant{{SubjectType: "user", SubjectID: u.ID, Capabilities: []string{"read", "merge"}}}}
	s.seedPolicy(t, p)
	// Hold the same database lock used by both governance and publication use cases,
	// remove review, then prove the waiting protected action rechecks after commit.
	execSQL(t, s.db.conn, "BEGIN")
	if _, err := s.db.conn.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", postgres.SecurityLock); err != nil {
		t.Fatal(err)
	}
	execSQL(t, s.db.conn, "DELETE FROM wikit.page_grants WHERE page_id='"+page+"' AND capability='merge'")
	done := make(chan error, 1)
	go func() {
		done <- s.db.store.WithinAccess(ctx, func(tx access.Tx) error {
			a, _, err := access.Authorize(ctx, tx, c, page, "merge", false)
			if err != nil {
				return err
			}
			return tx.Audit(ctx, a, "protected.publish", page, "race-test")
		})
	}()
	waitForAdvisoryWaiter(t, s.db.conn)
	execSQL(t, s.db.conn, "COMMIT")
	if err := <-done; err != fault.Forbidden {
		t.Fatal("revoked review committed protected action", err)
	}
	var count int
	if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.audit_events WHERE action='protected.publish'").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial write", err)
	}
}

func TestACL03ProtectedWriteCompletesBeforeRevocation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	u, token := s.register(t, "ordered_reviewer")
	page := s.pageID(t)
	c := identity.Credentials{Bearer: token}
	s.seedPolicy(t, access.Policy{PageID: page, Visibility: "restricted", Status: "published", Version: 1, Grants: []access.Grant{{SubjectType: "user", SubjectID: u.ID, Capabilities: []string{"read", "merge"}}}})
	checked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.db.store.WithinAccess(ctx, func(tx access.Tx) error {
			a, _, err := access.Authorize(ctx, tx, c, page, "merge", false)
			if err != nil {
				return err
			}
			close(checked)
			<-release
			return tx.Audit(ctx, a, "protected.publish", page, "ordered-test")
		})
	}()
	<-checked
	revoked := make(chan error, 1)
	admin := identity.Credentials{Bearer: s.adminToken}
	go func() {
		_, err := s.access.UpdatePolicy(ctx, requestKey(admin, "ordered-revoke"), page, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: u.ID, Capabilities: []string{"read"}}}}, 1)
		revoked <- err
	}()
	waitForAdvisoryWaiter(t, s.db.conn)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	if err := s.access.Check(ctx, c, page, "merge", false); err != fault.Forbidden {
		t.Fatal("post-revocation access", err)
	}
}

func (s *suite) pageID(t *testing.T) string {
	t.Helper()
	var id string
	if err := s.db.conn.QueryRow(context.Background(), "SELECT nextval('wikit.page_ids')::text").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

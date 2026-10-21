//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"sync"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestWikiHomeBootstrapProtectionAndAdminMerge(t *testing.T) {
	s := newMCPSuite(t)
	ctx := context.Background()
	admin := identity.Credentials{Bearer: s.adminToken}
	raw, _ := s.req(t, "GET", "pages/1", nil, identity.Credentials{}, "", 200)
	home := decodeResult[pages.Detail](t, raw)
	if home.Revision == nil || home.Revision.SourceKind != "bootstrap" || home.Revision.AuthorID != nil || home.Revision.Snapshot.Title != "首页" || *home.Revision.Snapshot.Markdown != "" {
		t.Fatal("incorrect bootstrap", home)
	}
	if err := s.db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.revisions").Scan(&count); err != nil || count != 1 {
		t.Fatal("bootstrap repeated", count, err)
	}
	_, mod := s.reviewer(t, "home_moderator")
	p := s.createProposal(t, mod, "1", home.Page.HeadRevisionID, document("首页", "Welcome"), "home-proposal-key")
	s.merge(t, mod, p, home.Page.HeadRevisionID, "home-deny-moderator", 403)
	client, _ := s.connectMCP(t, mod.Bearer)
	callMCP(t, client, "proposal_merge", map[string]any{"proposal_id": p.ID, "body": expected(p, home.Page.HeadRevisionID), "idempotency_key": "home-mcp-deny"}, "forbidden")
	s.req(t, "PUT", "pages/1/policy", map[string]any{"visibility": "restricted", "grants": []any{}, "expected_policy_version": 1}, admin, "home-restrict-key", 422)
	s.req(t, "POST", "pages/1/moderation", map[string]any{"hidden": true, "reason": "test", "expected_version": 1}, admin, "home-hide-key", 403)
	for _, sql := range []string{"DELETE FROM wikit.pages WHERE id=1", "DELETE FROM wikit.page_policies WHERE page_id=1", "UPDATE wikit.page_policies SET status='hidden' WHERE page_id=1", "UPDATE wikit.page_policies SET visibility='restricted' WHERE page_id=1", "UPDATE wikit.pages SET id=99 WHERE id=1"} {
		rejectSQL(t, s, sql)
	}
	s.merge(t, admin, p, home.Page.HeadRevisionID, "home-admin-merge", 200)
	if err := s.db.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	raw, _ = s.req(t, "GET", "pages/1", nil, identity.Credentials{}, "", 200)
	if *decodeResult[pages.Detail](t, raw).Revision.Snapshot.Markdown != "Welcome" {
		t.Fatal("initialization overwrote home")
	}
}

func TestCreationGroupGrantRevocationAndUniqueIDs(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	u, token := s.register(t, "group_creator")
	c := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	body := map[string]any{"visibility": "public", "snapshot": document("Published", "body")}
	s.req(t, "POST", "pages", body, c, "no-create-grant", 403)
	raw, err := s.access.SaveGroup(ctx, requestKey(admin, "creation-group-key"), "", "Creators", []string{u.ID}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	group := decodeResult[access.Group](t, raw)
	current, err := s.accounts.Current(ctx, c)
	if err != nil || !current.CanCreatePage || current.CreatePage {
		t.Fatal("group capability missing", err)
	}
	svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := svc.Create(ctx, requestKey(c, fmt.Sprintf("unique-page-%d", i)), "public", document("Concurrent", "body"), "unique-creation")
			ids <- p.Page.ID
			errs <- err
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for id := range ids {
		if id == "1" || seen[id] || !access.ValidPageID(id) {
			t.Fatal("invalid or reused ID", id)
		}
		seen[id] = true
	}
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "creation-revoke-key"), group.ID, group.Name, nil, 1, true); err != nil {
		t.Fatal(err)
	}
	current, err = s.accounts.Current(ctx, c)
	if err != nil || current.CanCreatePage {
		t.Fatal("stale creation capability", err)
	}
	s.req(t, "POST", "pages", body, c, "revoked-create-key", 403)
	grantCreate(t, s, u.ID)
	raw, _ = s.req(t, "POST", "pages", body, c, "personal-create-key", 201)
	p := decodeResult[pages.Created](t, raw)
	if p.Revision.ID == "" || p.Revision.SourceKind != "create" {
		t.Fatal("missing direct publication")
	}
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "creation-disabled-key"), u.ID, "member", "disabled", 2, true); err != nil {
		t.Fatal(err)
	}
	s.req(t, "POST", "pages", body, c, "disabled-create-key", 401)
}

func TestPageIDBeyondJavaScriptPrecision(t *testing.T) {
	s := newMCPSuite(t)
	if _, err := s.db.conn.Exec(context.Background(), "SELECT setval('wikit.page_ids',9007199254740991)"); err != nil {
		t.Fatal(err)
	}
	client, _ := s.connectMCP(t, s.adminToken)
	raw := callMCP(t, client, "page_create", map[string]any{"body": map[string]any{"visibility": "public", "snapshot": document("Large ID", "body")}, "idempotency_key": "large-page-id-key"}, "")
	p := decodeResult[pages.Created](t, raw)
	if p.Page.ID != "9007199254740992" || p.Revision.PageID != p.Page.ID {
		t.Fatal("numeric ID lost precision")
	}
	s.req(t, "GET", "pages/"+p.Page.ID, nil, identity.Credentials{}, "", 200)
	for _, id := range []string{"0", "01", "-1", "9223372036854775808", s.id(t)} {
		s.req(t, "GET", "pages/"+id, nil, identity.Credentials{}, "", 404)
	}
}
func TestCreatePermissionRevocationCommitOrders(t *testing.T) {
	for _, createFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(createFirst), func(t *testing.T) {
			s := newSuite(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			u, token := s.register(t, "creation_race")
			grantCreate(t, s, u.ID)
			admin := identity.Credentials{Bearer: s.adminToken}
			creator := identity.Credentials{Bearer: token}
			entered, release := make(chan struct{}), make(chan struct{})
			svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
			governance := s.access
			if createFirst {
				svc = pages.New(heldPage{s.db.store, entered, release}, s.secrets, textadapter.Parser{}, s.files)
			} else {
				governance = access.New(heldAccess{s.db.store, entered, release}, s.secrets)
			}
			create := func() error {
				_, err := svc.Create(ctx, requestKey(creator, "race-direct-create"), "public", document("Race", "body"), "race-create")
				return err
			}
			revoke := func() error {
				_, err := governance.UpdateAccount(ctx, requestKey(admin, "race-create-revoke"), u.ID, "member", "active", 2, false)
				return err
			}
			first, second := revoke, create
			if createFirst {
				first, second = create, revoke
			}
			a, b := make(chan error, 1), make(chan error, 1)
			go func() { a <- first() }()
			awaitBarrier(t, entered)
			go func() { b <- second() }()
			waitForAdvisoryWaiter(t, s.db.conn)
			close(release)
			if err := <-a; err != nil {
				t.Fatal(err)
			}
			err := <-b
			if createFirst && err != nil || !createFirst && !errors.Is(err, fault.Forbidden) {
				t.Fatal("wrong authorization order", err)
			}
			var n int
			if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.pages").Scan(&n); err != nil {
				t.Fatal(err)
			}
			want := 1
			if createFirst {
				want = 2
			}
			if n != want {
				t.Fatal("partial publication", n, want)
			}
		})
	}
}

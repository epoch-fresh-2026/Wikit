//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func TestPROP05CurrentStancesAndProtectedReads(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	admin := identity.Credentials{Bearer: s.adminToken}
	owner, token := s.register(t, "stance_owner")
	c := identity.Credentials{Bearer: token}
	reviewer, rc := s.reviewer(t, "stance_reviewer")
	negative, nc := s.reviewer(t, "negative_reviewer")
	page := s.createPageWithProposal(t, c, "restricted", document("Restricted", "private body"), "stance-page-key")
	policy := access.Policy{PageID: page.Page.ID, Visibility: "restricted", Status: "published", Version: 2, Grants: []access.Grant{
		{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose"}},
		{SubjectType: "user", SubjectID: reviewer.ID, Capabilities: []string{"read", "merge"}},
		{SubjectType: "user", SubjectID: negative.ID, Capabilities: []string{"read", "merge"}},
	}}
	s.seedPolicy(t, policy)
	p := page.Proposal
	s.req(t, "POST", "proposals/"+p.ID+"/revisions", map[string]any{"expected_head_revision_id": nil, "expected_proposal_revision_id": p.CurrentRevision.ID, "snapshot": document("No", "edit")}, rc, "non-author-update", 403)

	s.merge(t, c, p, nil, "no-merge-capability", 403)
	// Revoking the negative reviewer's permission removes that stance immediately.
	policy.Grants = policy.Grants[:2]
	policy.Version++
	s.seedPolicy(t, policy)
	for _, path := range []string{"proposals/" + p.ID + "/diff"} {
		s.req(t, "GET", path, nil, nc, "", 404)
		s.req(t, "GET", path, nil, c, "", 200)
	}
	// Disabling the sole approving reviewer leaves no valid approval.
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "disable-approved-reviewer"), reviewer.ID, "moderator", "disabled", 2, false); err != nil {
		t.Fatal(err)
	}
	s.merge(t, rc, p, nil, "disabled-actor-key", 401)
	s.req(t, "GET", "proposals/"+p.ID+"/diff", nil, rc, "", 401)

	rev := s.merge(t, admin, p, nil, "stance-publication-key", 200)
	// Merge replay cannot expose content after the source becomes hidden.
	policy.Status = "hidden"
	policy.Version++
	s.seedPolicy(t, policy)
	s.merge(t, admin, p, nil, "stance-publication-key", 404)
	for _, path := range []string{"proposals/" + p.ID + "/diff", "pages/" + page.Page.ID + "/revisions/" + rev.ID} {
		s.req(t, "GET", path, nil, c, "", 404)
	}
}

func TestProposalCloseAfterProposeRevocationAndManagerUpdate(t *testing.T) {
	s := newSuite(t)
	owner, token := s.register(t, "closing_owner")
	c := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	created := s.createPageWithProposal(t, c, "restricted", document("Close", "body"), "closing-create-key")
	s.seedPolicy(t, access.Policy{PageID: created.Page.ID, Visibility: "restricted", Status: "published", Version: 2, Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read"}}}})
	raw, _ := s.req(t, "POST", "proposals/"+created.Proposal.ID+"/state", map[string]any{"state": "closed", "expected_proposal_version": 1}, c, "closing-author-key", 200)
	p := decodeResult[proposals.Proposal](t, raw)
	s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "open", "expected_proposal_version": p.Version}, c, "reopen-revoked-key", 403)
	raw, _ = s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "open", "expected_proposal_version": p.Version}, admin, "reopen-manager-key", 200)
	p = decodeResult[proposals.Proposal](t, raw)
	s.req(t, "POST", "proposals/"+p.ID+"/revisions", map[string]any{"expected_head_revision_id": created.Page.HeadRevisionID, "expected_proposal_revision_id": p.CurrentRevision.ID, "snapshot": document("Managed", "new")}, admin, "update-manager-key", 200)
}

func TestMergeFailureRollsBackPublication(t *testing.T) {
	for _, table := range []string{"audit_events", "outbox_events", "idempotency_records"} {
		t.Run(table, func(t *testing.T) {
			s := newSuite(t)
			admin := identity.Credentials{Bearer: s.adminToken}
			created := s.createPageWithProposal(t, admin, "public", document("Atomic", "body"), "atomic-proposal-page")

			var before int
			if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit."+table).Scan(&before); err != nil {
				t.Fatal(err)
			}
			execSQL(t, s.db.conn, `CREATE FUNCTION wikit.fail_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected publish failure'; END $$; CREATE TRIGGER fail_publish BEFORE INSERT ON wikit.`+table+` FOR EACH ROW EXECUTE FUNCTION wikit.fail_publish()`)
			s.merge(t, admin, created.Proposal, nil, "atomic-proposal-merge", 500)
			for _, empty := range []string{"revisions", "outbox_events"} {
				var n int
				if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit."+empty).Scan(&n); err != nil || n != 2 {
					t.Fatal("partial publication", empty, n, err)
				}
			}
			var after int
			if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit."+table).Scan(&after); err != nil || after != before {
				t.Fatal("partial transaction side effects", err)
			}
			raw, _ := s.req(t, "GET", "proposals/"+created.Proposal.ID, nil, admin, "", 200)
			if decodeResult[proposals.Proposal](t, raw).State != "open" {
				t.Fatal("proposal partially merged")
			}
			execSQL(t, s.db.conn, "DROP TRIGGER fail_publish ON wikit."+table)
			s.merge(t, admin, created.Proposal, nil, "atomic-proposal-merge", 200)
		})
	}
}

// Transaction decorators place a deterministic barrier after real use-case
// checks/writes but before COMMIT. The store still owns the real database lock.
type heldAccess struct {
	base    access.Repository
	entered chan struct{}
	release chan struct{}
}

func (h heldAccess) WithinAccess(ctx context.Context, fn func(access.Tx) error) error {
	return h.base.WithinAccess(ctx, func(tx access.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		close(h.entered)
		select {
		case <-h.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

type heldPublication struct {
	base    proposals.Repository
	entered chan struct{}
	release chan struct{}
}
type heldPublicationTx struct {
	proposals.Tx
	entered chan struct{}
	release chan struct{}
}

func (h heldPublication) WithinProposals(ctx context.Context, fn func(proposals.Tx) error) error {
	return h.base.WithinProposals(ctx, func(tx proposals.Tx) error { return fn(heldPublicationTx{tx, h.entered, h.release}) })
}
func (h heldPublicationTx) Publish(ctx context.Context, r content.Published, refs []content.MediaRef) error {
	if err := h.Tx.Publish(ctx, r, refs); err != nil {
		return err
	}
	close(h.entered)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func awaitBarrier(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("transaction barrier timeout")
	}
}

func TestACL03RealMergeAndRevocationCommitOrders(t *testing.T) {
	for _, kind := range []string{"account", "group", "page"} {
		for _, mergeFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mergeFirst=%v", kind, mergeFirst), func(t *testing.T) {
				s := newSuite(t)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				admin := identity.Credentials{Bearer: s.adminToken}
				owner, token := s.register(t, "race_owner")
				reviewer, reviewToken := s.register(t, "race_approver")
				created := s.createPageWithProposal(t, identity.Credentials{Bearer: token}, "restricted", document("Race", "body"), "race-create-page")
				grants := []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose"}}, {SubjectType: "user", SubjectID: reviewer.ID, Capabilities: []string{"read", "merge"}}}
				var group access.Group
				if kind == "group" {
					raw, err := s.access.SaveGroup(ctx, requestKey(admin, "race-group-create"), "", "Reviewers", nil, 0, false)
					if err != nil {
						t.Fatal(err)
					}
					group = decodeResult[access.Group](t, raw)
					if _, err := s.access.SaveGroup(ctx, requestKey(admin, "race-group-members"), group.ID, group.Name, []string{reviewer.ID}, 1, false); err != nil {
						t.Fatal(err)
					}
					grants[1].Capabilities = []string{"read"}
					grants = append(grants, access.Grant{SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read", "merge"}})
				}
				s.seedPolicy(t, access.Policy{PageID: created.Page.ID, Visibility: "restricted", Status: "published", Version: 2, Grants: grants})

				entered, release := make(chan struct{}), make(chan struct{})
				var governance *access.Service = s.access
				publisher := s.proposalService()
				if mergeFirst {
					publisher = proposals.New(heldPublication{s.db.store, entered, release}, s.secrets, textadapter.Parser{}, textadapter.Differ{}, s.files)
				} else {
					governance = access.New(heldAccess{s.db.store, entered, release}, s.secrets)
				}
				revoke := func() error {
					switch kind {
					case "account":
						_, err := governance.UpdateAccount(ctx, requestKey(admin, "race-disable-account"), reviewer.ID, "member", "disabled", 1, false)
						return err
					case "group":
						_, err := governance.SaveGroup(ctx, requestKey(admin, "race-remove-members"), group.ID, group.Name, nil, 2, false)
						return err
					default:
						grants[1].Capabilities = []string{"read"}
						_, err := governance.UpdatePolicy(ctx, requestKey(admin, "race-revoke-policy"), created.Page.ID, access.Policy{Visibility: "restricted", Grants: grants}, 2)
						return err
					}
				}
				merge := func() error {
					_, err := publisher.Merge(ctx, requestKey(identity.Credentials{Bearer: reviewToken}, "race-real-merge"), created.Proposal.ID, expected(created.Proposal, nil))
					return err
				}
				first, second := revoke, merge
				if mergeFirst {
					first, second = merge, revoke
				}
				a, b := make(chan error, 1), make(chan error, 1)
				go func() { a <- first() }()
				awaitBarrier(t, entered)
				go func() { b <- second() }()
				waitForAdvisoryWaiter(t, s.db.conn)
				close(release)
				if err := <-a; err != nil {
					t.Fatal("first transaction", err)
				}
				err := <-b
				if mergeFirst && err != nil || !mergeFirst && !(errors.Is(err, fault.Forbidden) || kind == "account" && errors.Is(err, fault.Unauthenticated)) {
					t.Fatal("wrong serial outcome", err)
				}
				var count int
				if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.revisions").Scan(&count); err != nil {
					t.Fatal(err)
				}
				want := 2
				if mergeFirst {
					want = 3
				}
				if count != want {
					t.Fatal("revocation order violated", count, want)
				}
				// The real HTTP merge endpoint sees the same final state and authorization.
				status := 403
				if kind == "account" {
					status = 401
				}
				s.merge(t, identity.Credentials{Bearer: reviewToken}, created.Proposal, nil, "race-after-revocation", status)
			})
		}
	}
}

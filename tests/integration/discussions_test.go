//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *suite) issue(t *testing.T, c identity.Credentials, page, key string) discussions.Issue {
	t.Helper()
	raw, _ := s.req(t, "POST", "pages/"+page+"/issues", map[string]any{"title": "Discussion", "body": "original issue"}, c, key, 201)
	return decodeResult[discussions.Issue](t, raw)
}

type heldDiscussion struct {
	*postgres.Store
	entered, release chan struct{}
}
type heldDiscussionTx struct {
	discussions.Tx
	entered, release chan struct{}
}

func (r heldDiscussion) WithinDiscussions(ctx context.Context, fn func(discussions.Tx) error) error {
	return r.Store.WithinDiscussions(ctx, func(tx discussions.Tx) error { return fn(heldDiscussionTx{tx, r.entered, r.release}) })
}
func (t heldDiscussionTx) InsertComment(ctx context.Context, c discussions.Comment) error {
	close(t.entered)
	<-t.release
	return t.Tx.InsertComment(ctx, c)
}

func TestDISC05CommentAndModerationCommitOrder(t *testing.T) {
	for _, first := range []string{"comment", "hide"} {
		t.Run(first, func(t *testing.T) {
			s := newSuite(t)
			ctx := context.Background()
			admin := identity.Credentials{Bearer: s.adminToken}
			p := s.createPage(t, admin, "public", document("Page", "text"), "race-discussion-page")
			i := s.issue(t, admin, p.Page.ID, "race-discussion-issue")
			normal := discussions.New(s.db.store, s.secrets, textadapter.Parser{})
			done := make(chan error, 1)
			if first == "comment" {
				entered, release := make(chan struct{}), make(chan struct{})
				svc := discussions.New(heldDiscussion{s.db.store, entered, release}, s.secrets, textadapter.Parser{})
				go func() {
					_, err := svc.Comment(ctx, requestKey(admin, "ordered-comment-key"), i.ID, "before hide", "race-ip")
					done <- err
				}()
				awaitBarrier(t, entered)
				hidden := make(chan error, 1)
				go func() {
					_, err := normal.Moderate(ctx, requestKey(admin, "ordered-hide-key"), "issue", i.ID, true, "reason", 1)
					hidden <- err
				}()
				waitForAdvisoryWaiter(t, s.db.conn)
				close(release)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if err := <-hidden; err != nil {
					t.Fatal(err)
				}
			} else {
				// Hold a real governance transaction before commit; the comment preflight must wait.
				entered, release := make(chan struct{}), make(chan struct{})
				hidden := make(chan error, 1)
				go func() {
					hidden <- s.db.store.WithinDiscussions(ctx, func(tx discussions.Tx) error {
						v, err := tx.Issue(ctx, i.ID)
						if err != nil {
							return err
						}
						v.Hidden = true
						v.Version++
						if err := tx.SaveIssue(ctx, v); err != nil {
							return err
						}
						close(entered)
						<-release
						return nil
					})
				}()
				awaitBarrier(t, entered)
				go func() {
					_, err := normal.Comment(ctx, requestKey(admin, "after-hide-comment"), i.ID, "after hide", "race-ip")
					done <- err
				}()
				waitForAdvisoryWaiter(t, s.db.conn)
				close(release)
				if err := <-hidden; err != nil {
					t.Fatal(err)
				}
				if err := <-done; err != fault.NotFound {
					t.Fatal("hidden issue accepted comment", err)
				}
			}
			var count int
			if err := s.db.conn.QueryRow(ctx, `SELECT count(*) FROM wikit.comments WHERE issue_id=$1`, i.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if first == "comment" {
				want = 1
			}
			if count != want {
				t.Fatalf("comments=%d want %d", count, want)
			}
		})
	}
}

func TestDISC06RateLimitsAndPublishedPageRestore(t *testing.T) {
	s := newSuite(t)
	_, c := s.reviewer(t, "discussion_rate_author")
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, c, "public", document("Published", "text"), "rate-discussion-page")
	revision := s.publish(t, c, p.Proposal, nil, "rate-publish")
	i := s.issue(t, c, p.Page.ID, "rate-issue-key")
	for n := 0; n < 10; n++ {
		s.req(t, "POST", "issues/"+i.ID+"/comments", map[string]any{"body": "comment"}, c, fmt.Sprintf("rate-comment-%d", n), 201)
	}
	s.req(t, "POST", "issues/"+i.ID+"/comments", map[string]any{"body": "limited"}, c, "rate-comment-final", 429)
	// Creating Issues has a separate bucket from comments.
	s.issue(t, c, p.Page.ID, "rate-separate-issue")
	path := "pages/" + p.Page.ID + "/moderation"
	s.req(t, "POST", path, map[string]any{"hidden": true, "reason": "hide page", "expected_version": 1}, admin, "hide-published-key", 200)
	s.req(t, "GET", "pages/"+p.Page.ID+"/revisions/"+revision.ID, nil, c, "", 404)
	raw, _ := s.req(t, "POST", path, map[string]any{"hidden": false, "reason": "restore page", "expected_version": 2}, admin, "restore-published-key", 200)
	restored := decodeResult[pages.Page](t, raw)
	if restored.Status != "published" || restored.HeadRevisionID == nil || *restored.HeadRevisionID != revision.ID || restored.Visibility != "public" || restored.PolicyVersion != 3 {
		t.Fatal("restore changed publication or policy")
	}
	s.req(t, "GET", "pages/"+p.Page.ID+"/revisions/"+revision.ID, nil, c, "", 200)
}
func TestDISC01LifecycleAssociationAndMerge(t *testing.T) {
	s := newSuite(t)
	_, owner := s.reviewer(t, "discussion_owner")
	_, token := s.register(t, "other_member")
	other := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, owner, "public", document("Page", "text"), "discussion-page")
	i := s.issue(t, owner, p.Page.ID, "discussion-create")
	again := s.issue(t, owner, p.Page.ID, "discussion-create")
	if again.ID != i.ID {
		t.Fatal("duplicate issue")
	}
	path := "issues/" + i.ID
	s.req(t, "POST", path+"/state", map[string]any{"state": "closed", "expected_version": 1}, other, "other-close-key", 403)
	raw, _ := s.req(t, "POST", path+"/proposals", map[string]any{"proposal_id": p.Proposal.ID, "expected_version": 1}, owner, "discussion-link", 200)
	i = decodeResult[discussions.Issue](t, raw)
	if i.Version != 2 || len(i.ProposalIDs) != 1 {
		t.Fatal("missing association")
	}
	raw, _ = s.req(t, "POST", path+"/proposals", map[string]any{"proposal_id": p.Proposal.ID, "expected_version": 2}, owner, "duplicate-link-key", 200)
	if decodeResult[discussions.Issue](t, raw).Version != 2 {
		t.Fatal("duplicate advanced version")
	}
	p2 := s.createPage(t, owner, "public", document("Other", "text"), "discussion-other-page")
	s.req(t, "POST", path+"/proposals", map[string]any{"proposal_id": p2.Proposal.ID, "expected_version": 2}, owner, "cross-page-link-key", 422)
	_, err := s.db.conn.Exec(context.Background(), `INSERT INTO wikit.issue_proposals(issue_id,page_id,proposal_id) VALUES($1,$2,$3)`, i.ID, p.Page.ID, p2.Proposal.ID)
	if pg, ok := err.(*pgconn.PgError); !ok || pg.Code != "23503" {
		t.Fatalf("cross-page storage constraint: %v", err)
	}
	s.publish(t, owner, p.Proposal, nil, "discussion-publish")
	raw, _ = s.req(t, "GET", path, nil, identity.Credentials{}, "", 200)
	if decodeResult[discussions.Issue](t, raw).State != "open" {
		t.Fatal("merge closed discussion")
	}
	s.req(t, "POST", path+"/state", map[string]any{"state": "closed", "expected_version": 2}, owner, "discussion-close", 200)
	s.req(t, "POST", path+"/comments", map[string]any{"body": "closed issue comment"}, other, "closed-comment-key", 201)
	s.req(t, "POST", path+"/state", map[string]any{"state": "open", "expected_version": 2}, owner, "stale-open-key", 409)
	s.req(t, "POST", path+"/state", map[string]any{"state": "open", "expected_version": 3}, admin, "discussion-reopen", 200)
	s.req(t, "POST", path+"/comments", map[string]any{"body": "anonymous"}, identity.Credentials{}, "anonymous-comment", 401)
	s.req(t, "POST", path+"/comments", map[string]any{"body": "  "}, owner, "empty-comment-key", 422)
}

func TestDISC02ModerationVisibilityAndReplay(t *testing.T) {
	s := newSuite(t)
	_, token := s.register(t, "discussion_author")
	owner := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, owner, "public", document("Page", "text"), "moderation-page-key")
	i := s.issue(t, owner, p.Page.ID, "moderation-issue-key")
	path := "issues/" + i.ID
	input := map[string]any{"body": "immutable comment"}
	raw, _ := s.req(t, "POST", path+"/comments", input, owner, "moderation-comment", 201)
	c := decodeResult[discussions.Comment](t, raw)
	moderate := func(path string, hidden bool, version int, status int) {
		t.Helper()
		s.req(t, "POST", path+"/moderation", map[string]any{"hidden": hidden, "reason": "reviewed reason", "expected_version": version}, admin, fmt.Sprintf("moderate-%s-%d", path, version), status)
	}
	s.req(t, "POST", "comments/"+c.ID+"/moderation", map[string]any{"hidden": true, "reason": "reason", "expected_version": 1}, owner, "author-hide-key", 403)
	moderate("comments/"+c.ID, true, 1, 200)
	s.req(t, "POST", path+"/comments", input, owner, "moderation-comment", 404)
	raw, _ = s.req(t, "GET", path+"/comments", nil, admin, "", 200)
	if len(decodeResult[discussions.List](t, raw).Items) != 0 {
		t.Fatal("hidden comment leaked")
	}
	raw, _ = s.req(t, "GET", path+"/comments?include_hidden=true", nil, admin, "", 200)
	if len(decodeResult[discussions.List](t, raw).Items) != 1 {
		t.Fatal("missing governance comment")
	}
	s.req(t, "GET", path+"?include_hidden=true", nil, owner, "", 403)
	moderate(path, true, 1, 200)
	s.req(t, "GET", path, nil, admin, "", 404)
	s.req(t, "GET", path+"/comments", nil, owner, "", 404)
	s.req(t, "POST", "pages/"+p.Page.ID+"/issues", map[string]any{"title": "Discussion", "body": "original issue"}, owner, "moderation-issue-key", 404)
	moderate(path, false, 2, 200)
	raw, _ = s.req(t, "GET", path+"/comments", nil, owner, "", 200)
	if len(decodeResult[discussions.List](t, raw).Items) != 0 {
		t.Fatal("issue restore restored comment")
	}
	moderate("pages/"+p.Page.ID, true, 1, 200)
	s.req(t, "GET", path, nil, owner, "", 404)
	s.req(t, "GET", path+"?include_hidden=true", nil, admin, "", 200)
	moderate("pages/"+p.Page.ID, false, 2, 200)
	raw, _ = s.req(t, "GET", path, nil, owner, "", 200)
	if decodeResult[discussions.Issue](t, raw).Hidden {
		t.Fatal("restore failed")
	}
	var count int
	if err := s.db.conn.QueryRow(context.Background(), `SELECT count(*) FROM wikit.moderation_events WHERE page_id=$1 AND reason='reviewed reason'`, p.Page.ID).Scan(&count); err != nil || count != 5 {
		t.Fatalf("moderation audit %d: %v", count, err)
	}
	for _, sql := range []string{`UPDATE wikit.comments SET body='rewritten',version=version+1`, `UPDATE wikit.page_issues SET title='rewritten',version=version+1`, `UPDATE wikit.moderation_events SET reason='rewritten'`, `TRUNCATE wikit.comments`} {
		_, err := s.db.conn.Exec(context.Background(), sql)
		pg, ok := err.(*pgconn.PgError)
		if !ok || pg.Code != "23514" {
			t.Fatalf("immutability: %v", err)
		}
	}
}

func TestDISC03CurrentCapabilitiesAndBoundCursors(t *testing.T) {
	s := newSuite(t)
	author, token := s.register(t, "restricted_author")
	owner := identity.Credentials{Bearer: token}
	manager, mt := s.register(t, "discussion_manager")
	mc := identity.Credentials{Bearer: mt}
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, admin, "restricted", document("Private", "text"), "private-discussion-page")
	policy := access.Policy{PageID: p.Page.ID, Visibility: "restricted", Status: "proposed", Version: 2, Grants: []access.Grant{{SubjectType: "user", SubjectID: author.ID, Capabilities: []string{"read", "discuss"}}, {SubjectType: "user", SubjectID: manager.ID, Capabilities: []string{"read", "manage"}}}}
	s.seedPolicy(t, policy)
	i := s.issue(t, owner, p.Page.ID, "private-issue-key")
	s.issue(t, owner, p.Page.ID, "private-issue-two")
	path := "issues/" + i.ID
	s.req(t, "POST", path+"/comments", map[string]any{"body": "manager lacks discuss"}, mc, "manager-comment-key", 403)
	s.req(t, "POST", path+"/state", map[string]any{"state": "closed", "expected_version": 1}, mc, "manager-close-key", 200)
	raw, _ := s.req(t, "GET", "pages/"+p.Page.ID+"/issues?limit=1", nil, owner, "", 200)
	list := decodeResult[discussions.List](t, raw)
	if list.Next == nil {
		t.Fatal("missing cursor")
	}
	cursor := *list.Next
	s.req(t, "GET", "pages/"+p.Page.ID+"/issues?limit=1&cursor="+cursor, nil, mc, "", 400)
	s.req(t, "GET", "pages/"+p.Page.ID+"/issues?limit=2&cursor="+cursor, nil, owner, "", 400)
	s.req(t, "GET", "pages/"+p.Page.ID+"/issues?limit=1&state=open&cursor="+cursor, nil, owner, "", 400)
	policy.Version++
	policy.Grants[0].Capabilities = []string{"read"}
	s.seedPolicy(t, policy)
	s.req(t, "POST", path+"/state", map[string]any{"state": "open", "expected_version": 2}, owner, "author-without-discuss", 200)
	s.req(t, "POST", path+"/comments", map[string]any{"body": "revoked"}, owner, "revoked-discuss-key", 403)
	s.req(t, "POST", path+"/proposals", map[string]any{"proposal_id": p.Proposal.ID, "expected_version": 3}, owner, "revoked-link-key", 403)
	policy.Version++
	policy.Grants = policy.Grants[1:]
	s.seedPolicy(t, policy)
	s.req(t, "GET", path, nil, owner, "", 404)
	s.req(t, "GET", "pages/"+p.Page.ID+"/issues?limit=1&cursor="+cursor, nil, owner, "", 404)
}

func TestDISC04AtomicFailureAndInputLimits(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, admin, "public", document("Page", "text"), "failure-discussion-page")
	i := s.issue(t, admin, p.Page.ID, "failure-issue-key")
	path := "issues/" + i.ID
	execSQL(t, s.db.conn, `CREATE FUNCTION wikit.test_discussion_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='issue.moderate' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_discussion_audit BEFORE INSERT ON wikit.audit_events FOR EACH ROW EXECUTE FUNCTION wikit.test_discussion_audit()`)
	s.req(t, "POST", path+"/moderation", map[string]any{"hidden": true, "reason": "reason", "expected_version": 1}, admin, "failed-moderation-key", 500)
	raw, _ := s.req(t, "GET", path, nil, admin, "", 200)
	v := decodeResult[discussions.Issue](t, raw)
	if v.Hidden || v.Version != 1 {
		t.Fatal("failed audit committed mutation")
	}
	var count int
	if err := s.db.conn.QueryRow(context.Background(), `SELECT count(*) FROM wikit.moderation_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed transaction kept reason: %d %v", count, err)
	}
	s.req(t, "POST", path+"/comments", map[string]any{"body": strings.Repeat("知", 65536)}, admin, "unicode-comment-key", 201)
	s.req(t, "POST", path+"/comments", map[string]any{"body": strings.Repeat("知", 65537)}, admin, "too-long-comment-key", 422)
	s.req(t, "POST", path+"/comments", map[string]any{"body": "\x00"}, admin, "nul-comment-key", 422)
	s.req(t, "GET", path+"?include_hidden=invalid", nil, admin, "", 400)
}

func TestDISC07ConcurrentStateAndIdempotencyConflict(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, admin, "public", document("Page", "text"), "state-race-page")
	i := s.issue(t, admin, p.Page.ID, "state-race-issue")
	svc := discussions.New(s.db.store, s.secrets, textadapter.Parser{})
	start := make(chan struct{})
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		go func(n int) {
			<-start
			_, err := svc.State(context.Background(), requestKey(admin, fmt.Sprintf("concurrent-state-%d", n)), i.ID, "closed", 1)
			results <- err
		}(n)
	}
	close(start)
	wins, stale := 0, 0
	for n := 0; n < 2; n++ {
		err := <-results
		switch err {
		case nil:
			wins++
		case fault.Stale:
			stale++
		default:
			t.Fatal(err)
		}
	}
	if wins != 1 || stale != 1 {
		t.Fatalf("wins=%d stale=%d", wins, stale)
	}
	s.req(t, "POST", "pages/"+p.Page.ID+"/issues", map[string]any{"title": "Different payload", "body": "original issue"}, admin, "state-race-issue", 409)
	raw, _ := s.req(t, "GET", "issues/"+i.ID, nil, admin, "", 200)
	if v := decodeResult[discussions.Issue](t, raw); v.Version != 2 || v.State != "closed" {
		t.Fatal("concurrent state lost update")
	}
}

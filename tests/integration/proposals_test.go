//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func decodeResult[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func (s *suite) proposalService() *proposals.Service {
	return proposals.New(s.db.store, s.secrets, textadapter.Parser{}, textadapter.Differ{}, s.files)
}
func expected(p proposals.Proposal, head *string) proposals.Expected {
	return proposals.Expected{Head: head, Candidate: p.CurrentRevision.ID}
}
func (s *suite) reviewer(t *testing.T, name string) (identity.Account, identity.Credentials) {
	t.Helper()
	u, token := s.register(t, name)
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, "role-"+name), u.ID, "reviewer", "active", 1); err != nil {
		t.Fatal(err)
	}
	return u, identity.Credentials{Bearer: token}
}
func (s *suite) createProposal(t *testing.T, c identity.Credentials, page string, head *string, snapshot content.Snapshot, key string) proposals.Proposal {
	t.Helper()
	raw, _ := s.req(t, "POST", "pages/"+page+"/proposals", map[string]any{"expected_head_revision_id": head, "snapshot": snapshot}, c, key, 201)
	return decodeResult[proposals.Proposal](t, raw)
}
func (s *suite) review(t *testing.T, c identity.Credentials, p proposals.Proposal, head *string, decision, key string, status int) {
	t.Helper()
	s.req(t, "POST", "proposals/"+p.ID+"/reviews", map[string]any{"expected_head_revision_id": head, "expected_proposal_revision_id": p.CurrentRevision.ID, "expected_review_round": p.ReviewRound, "decision": decision, "body": "review body"}, c, key, status)
}
func (s *suite) merge(t *testing.T, c identity.Credentials, p proposals.Proposal, head *string, key string, status int) content.Published {
	t.Helper()
	raw, _ := s.req(t, "POST", "proposals/"+p.ID+"/merge", expected(p, head), c, key, status)
	if status != 200 {
		return content.Published{}
	}
	return decodeResult[content.Published](t, raw)
}
func (s *suite) publish(t *testing.T, c identity.Credentials, p proposals.Proposal, head *string, key string) content.Published {
	t.Helper()
	s.review(t, c, p, head, "approve", key+"-review", 201)
	return s.merge(t, c, p, head, key+"-merge", 200)
}
func TestPROP01StateReviewRoundsAndSelfReview(t *testing.T) {
	s := newSuite(t)
	_, c := s.reviewer(t, "self_reviewer")
	created := s.createPage(t, c, "public", document("Original", "one\ntwo\n"), "self-create-page")
	p := created.Proposal
	s.merge(t, c, p, nil, "unapproved-merge", 409)
	s.req(t, "POST", "proposals/"+p.ID+"/merge", map[string]any{"expected_proposal_revision_id": p.CurrentRevision.ID}, c, "missing-head-key", 400)
	s.review(t, c, p, nil, "approve", "self-approve-key", 201)
	// A comment does not clear a negative stance; the same actor can later approve.
	s.review(t, c, p, nil, "request_changes", "self-negative-key", 201)
	s.review(t, c, p, nil, "comment", "self-comment-key", 201)
	s.merge(t, c, p, nil, "negative-merge-key", 409)
	s.review(t, c, p, nil, "approve", "self-resolve-key", 201)
	input := map[string]any{"expected_head_revision_id": nil, "expected_proposal_revision_id": p.CurrentRevision.ID, "snapshot": document("Updated", "ONE\ntwo\n")}
	raw, _ := s.req(t, "POST", "proposals/"+p.ID+"/revisions", input, c, "update-candidate-key", 200)
	updated := decodeResult[proposals.Proposal](t, raw)
	if updated.ReviewRound != 2 || updated.Version != 2 || updated.CurrentRevision.ID == p.CurrentRevision.ID {
		t.Fatal("candidate did not advance")
	}
	s.review(t, c, p, nil, "approve", "obsolete-review-key", 409)
	s.merge(t, c, updated, nil, "updated-unapproved", 409)
	s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "closed", "expected_proposal_version": 1}, c, "close-stale-version", 409)
	raw, _ = s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "closed", "expected_proposal_version": 2}, c, "close-proposal-key", 200)
	closed := decodeResult[proposals.Proposal](t, raw)
	s.review(t, c, closed, nil, "approve", "closed-review-key", 409)
	raw, _ = s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "open", "expected_proposal_version": closed.Version}, c, "reopen-proposal-key", 200)
	p = decodeResult[proposals.Proposal](t, raw)
	if p.ReviewRound != 3 {
		t.Fatal("reopen retained review round")
	}
	s.merge(t, c, p, nil, "reopened-unapproved", 409)
	rev := s.publish(t, c, p, nil, "self-publish-key")
	retry := s.merge(t, c, p, nil, "self-publish-key-merge", 200)
	if rev.ID != retry.ID || rev.ParentID != nil || rev.Snapshot.Title != "Updated" {
		t.Fatal("duplicate or incorrect publication")
	}
	s.merge(t, c, p, nil, "second-merge-newkey", 409)
	s.req(t, "POST", "proposals/"+p.ID+"/state", map[string]any{"state": "open", "expected_proposal_version": p.Version + 1}, c, "reopen-merged-key", 409)
	raw, _ = s.req(t, "GET", "proposals/"+p.ID+"/revisions", nil, c, "", 200)
	var history struct{ Items []proposals.Revision }
	if err := json.Unmarshal(raw, &history); err != nil || len(history.Items) != 2 {
		t.Fatal("candidate history lost", err)
	}
	raw, _ = s.req(t, "GET", "proposals/"+p.ID+"/reviews?limit=1", nil, c, "", 200)
	list := decodeResult[proposals.ReviewList](t, raw)
	if list.Next == nil || len(list.Items) != 1 {
		t.Fatal("review pagination missing")
	}
	s.req(t, "GET", "proposals/"+p.ID+"/reviews?limit=1&cursor="+*list.Next, nil, c, "", 200)
	s.req(t, "GET", "proposals/"+p.ID+"/reviews?limit=1&cursor="+*list.Next, nil, identity.Credentials{}, "", 400)
	s.req(t, "GET", "proposals/"+p.ID+"/reviews?limit=2&cursor="+*list.Next, nil, c, "", 400)
	raw, _ = s.req(t, "GET", "proposals/"+p.ID+"/diff", nil, c, "", 200)
	diff := decodeResult[proposals.Diff](t, raw)
	if !diff.Stale || len(diff.Fields) == 0 {
		t.Fatal("diff stale/base missing")
	}
	for _, f := range diff.Fields {
		if f.Path == "/markdown" && (f.UnifiedDiff == nil || !strings.Contains(*f.UnifiedDiff, "+ONE")) {
			t.Fatal("missing line diff")
		}
	}
	rejectSQL(t, s, "UPDATE wikit.reviews SET decision='approve' WHERE proposal_id=$1", p.ID)
	rejectSQL(t, s, "DELETE FROM wikit.reviews WHERE proposal_id=$1", p.ID)
}

func TestPROP02And03ConcurrentMergeAndRebase(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	created := s.createPage(t, admin, "public", document("Lines", "a\nb\nc\n"), "parallel-seed-page")
	base := s.publish(t, admin, created.Proposal, nil, "parallel-seed")
	one := s.createProposal(t, admin, created.Page.ID, &base.ID, document("Lines", "A\nb\nc\n"), "parallel-proposal-one")
	two := s.createProposal(t, admin, created.Page.ID, &base.ID, document("Lines", "a\nb\nC\n"), "parallel-proposal-two")
	for i, p := range []proposals.Proposal{one, two} {
		s.review(t, admin, p, &base.ID, "approve", fmt.Sprintf("parallel-review-%d", i), 201)
	}
	type result struct {
		p   proposals.Proposal
		rev content.Published
		err error
	}
	done := make(chan result, 2)
	start := make(chan struct{})
	for i, p := range []proposals.Proposal{one, two} {
		go func(i int, p proposals.Proposal) {
			<-start
			rev, err := s.proposalService().Merge(context.Background(), requestKey(admin, fmt.Sprintf("parallel-merge-%d", i)), p.ID, expected(p, &base.ID))
			done <- result{p, rev, err}
		}(i, p)
	}
	close(start)
	a, b := <-done, <-done
	if a.err != nil {
		a, b = b, a
	}
	if a.err != nil || !errors.Is(b.err, fault.Stale) {
		t.Fatal("concurrent merges did not serialize", a.err, b.err)
	}
	s.review(t, admin, b.p, &a.rev.ID, "approve", "stale-base-review", 409)
	rebased, err := s.proposalService().Rebase(context.Background(), requestKey(admin, "parallel-rebase"), b.p.ID, expected(b.p, &a.rev.ID))
	if err != nil {
		t.Fatal(err)
	}
	if *rebased.CurrentRevision.Snapshot.Markdown != "A\nb\nC\n" || rebased.ReviewRound != 2 {
		t.Fatal("nonoverlapping edits lost")
	}
	s.merge(t, admin, rebased, &a.rev.ID, "rebase-needs-review", 409)
	final := s.publish(t, admin, rebased, &a.rev.ID, "parallel-final")
	if final.ParentID == nil || *final.ParentID != a.rev.ID {
		t.Fatal("nonlinear parent")
	}
	var count int
	if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit.outbox_events").Scan(&count); err != nil || count != 3 {
		t.Fatal("outbox publication mismatch", count, err)
	}
}

func TestPROP04StructuredConflictsAndResolution(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	created := s.createPage(t, admin, "public", document("Base", "base\n"), "conflict-seed-page")
	base := s.publish(t, admin, created.Proposal, nil, "conflict-base")
	one := s.createProposal(t, admin, created.Page.ID, &base.ID, document("Ours", "ours\n"), "conflict-ours-key")
	two := s.createProposal(t, admin, created.Page.ID, &base.ID, document("Theirs", "theirs\n"), "conflict-theirs-key")
	head := s.publish(t, admin, one, &base.ID, "conflict-head")
	raw, _ := s.req(t, "POST", "proposals/"+two.ID+"/rebase", expected(two, &head.ID), admin, "conflicted-rebase", 409)
	var failure struct {
		Error struct {
			Code      string
			Conflicts []fault.Conflict
		}
	}
	if err := json.Unmarshal(raw, &failure); err != nil || failure.Error.Code != "merge_conflict" || len(failure.Error.Conflicts) != 2 {
		t.Fatal("missing structured conflicts", err)
	}
	for _, f := range failure.Error.Conflicts {
		if f.Base == nil || f.Ours == nil || f.Theirs == nil {
			t.Fatal("missing B/H/P")
		}
	}
	raw, _ = s.req(t, "GET", "proposals/"+two.ID, nil, admin, "", 200)
	unchanged := decodeResult[proposals.Proposal](t, raw)
	if unchanged.Version != 1 || unchanged.CurrentRevision.ID != two.CurrentRevision.ID {
		t.Fatal("conflict saved candidate")
	}
	input := map[string]any{"expected_head_revision_id": head.ID, "expected_proposal_revision_id": two.CurrentRevision.ID, "snapshot": document("Resolved", "resolved\n")}
	raw, _ = s.req(t, "POST", "proposals/"+two.ID+"/revisions", input, admin, "resolve-conflict-key", 200)
	resolved := decodeResult[proposals.Proposal](t, raw)
	s.merge(t, admin, resolved, &head.ID, "resolved-unreviewed", 409)
	s.publish(t, admin, resolved, &head.ID, "resolved-publish")
	// A full resolved snapshot still cannot overwrite a subsequently changed head.
	s.req(t, "POST", "proposals/"+two.ID+"/revisions", input, admin, "resolve-old-head-key", 409)
}

func TestROLL01RevertRestorePreserveHistoryAndACL(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	created := s.createPage(t, admin, "public", document("Rollback", "a\nb\nc\n"), "rollback-seed-page")
	first := s.publish(t, admin, created.Proposal, nil, "rollback-first")
	p := s.createProposal(t, admin, created.Page.ID, &first.ID, document("Rollback", "A\nb\nc\n"), "rollback-second-proposal")
	second := s.publish(t, admin, p, &first.ID, "rollback-second")
	p = s.createProposal(t, admin, created.Page.ID, &second.ID, document("Rollback", "A\nb\nC\n"), "rollback-third-proposal")
	third := s.publish(t, admin, p, &second.ID, "rollback-third")
	policy, err := s.access.GetPolicy(context.Background(), admin, created.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := s.req(t, "POST", "pages/"+created.Page.ID+"/revert", map[string]any{"revision_id": second.ID, "expected_head_revision_id": third.ID}, admin, "rollback-revert-key", 201)
	revert := decodeResult[proposals.Proposal](t, raw)
	if *revert.CurrentRevision.Snapshot.Markdown != "a\nb\nC\n" {
		t.Fatal("revert removed later edit")
	}
	reverted := s.publish(t, admin, revert, &third.ID, "rollback-reverted")
	raw, _ = s.req(t, "POST", "pages/"+created.Page.ID+"/restore", map[string]any{"revision_id": first.ID, "expected_head_revision_id": reverted.ID}, admin, "rollback-restore-key", 201)
	restore := decodeResult[proposals.Proposal](t, raw)
	if *restore.CurrentRevision.Snapshot.Markdown != *first.Snapshot.Markdown {
		t.Fatal("restore changed snapshot")
	}
	last := s.publish(t, admin, restore, &reverted.ID, "rollback-restored")
	s.req(t, "POST", "pages/"+created.Page.ID+"/revert", map[string]any{"revision_id": first.ID, "expected_head_revision_id": last.ID}, admin, "rollback-first-invalid", 409)
	newPolicy, err := s.access.GetPolicy(context.Background(), admin, created.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(policy)
	after, _ := json.Marshal(newPolicy)
	if string(before) != string(after) {
		t.Fatal("rollback changed ACL")
	}
	raw, _ = s.req(t, "GET", "pages/"+created.Page.ID+"/revisions", nil, admin, "", 200)
	var history struct{ Items []content.Published }
	if err := json.Unmarshal(raw, &history); err != nil || len(history.Items) != 5 || *history.Items[1].Snapshot.Markdown != "A\nb\nc\n" {
		t.Fatal("publication history rewritten", err)
	}
	// Same-line inverse modifications conflict and do not create a proposal.
	p = s.createProposal(t, admin, created.Page.ID, &last.ID, document("Rollback", "Z\nb\nc\n"), "rollback-conflict-proposal")
	head := s.publish(t, admin, p, &last.ID, "rollback-conflict-head")
	s.req(t, "POST", "pages/"+created.Page.ID+"/revert", map[string]any{"revision_id": second.ID, "expected_head_revision_id": head.ID}, admin, "rollback-conflict-key", 409)
}

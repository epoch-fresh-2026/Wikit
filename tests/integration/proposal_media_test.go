//go:build integration

package integration

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
)

func TestPROP04LineMergeCasesAndTagSets(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	for i, tc := range []struct {
		base, ours, theirs, want string
		conflict                 bool
	}{
		{"a\nb\nc\n", "a\nx\nb\nc\n", "a\nb\ny\nc\n", "a\nx\nb\ny\nc\n", false},
		{"a\nb\nc\n", "a\nc\n", "a\nb\nC\n", "a\nC\n", false},
		{"a\nb\n", "x\na\nb\n", "a\nB\n", "x\na\nB\n", false},
		{"a\nb", "A\nb", "a\nB", "A\nB", false},
		{"a\nb\n", "a\nx\nb\n", "a\nx\nb\n", "a\nx\nb\n", false},
		{"a\nb\n", "a\nx\nb\n", "a\ny\nb\n", "", true},
		{"a\nb\nc\n", "a\nc\n", "a\nB\nc\n", "", true},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			key := fmt.Sprintf("line-case-%d", i)
			baseSnapshot := document("Lines", tc.base)
			baseSnapshot.Tags = []string{"keep", "remove"}
			page := s.createPage(t, admin, "public", baseSnapshot, key+"-page")
			base := s.publish(t, admin, page.Proposal, nil, key+"-base")
			ours, theirs := document("Lines", tc.ours), document("Lines", tc.theirs)
			ours.Tags = []string{"keep", "ours"}
			theirs.Tags = []string{"keep", "remove", "theirs"}
			p := s.createProposal(t, admin, page.Page.ID, &base.ID, ours, key+"-ours")
			q := s.createProposal(t, admin, page.Page.ID, &base.ID, theirs, key+"-theirs")
			head := s.publish(t, admin, p, &base.ID, key+"-head")
			out, err := s.proposalService().Rebase(context.Background(), requestKey(admin, key+"-rebase"), q.ID, expected(q, &head.ID))
			if tc.conflict {
				if _, ok := err.(*fault.Conflicts); !ok {
					t.Fatal("missing conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *out.CurrentRevision.Snapshot.Markdown != tc.want || !slices.Equal(out.CurrentRevision.Snapshot.Tags, []string{"keep", "ours", "theirs"}) {
				t.Fatal("line/tag merge lost changes")
			}
		})
	}
}

func TestPROP04MediaScalarsAndROLL01OriginalReuse(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	admin := identity.Credentials{Bearer: s.adminToken}
	owner, token := s.register(t, "media_proposal_owner")
	c := identity.Credentials{Bearer: token}
	firstBlob := s.blobFixture(t, owner.ID, true)
	initial := content.Snapshot{Type: "media", Title: "Media", Tags: []string{}, Description: ptr("description"), BlobID: &firstBlob}
	page := s.createPage(t, c, "public", initial, "media-workflow-page")
	base := s.publish(t, admin, page.Proposal, nil, "media-workflow-base")
	secondBlob, thirdBlob := s.blobFixture(t, owner.ID, true), s.blobFixture(t, owner.ID, true)
	ours, theirs := initial, initial
	ours.BlobID = &secondBlob
	ours.Description = ptr("ours")
	theirs.BlobID = &thirdBlob
	theirs.Description = ptr("theirs")
	p := s.createProposal(t, c, page.Page.ID, &base.ID, ours, "media-ours-proposal")
	q := s.createProposal(t, c, page.Page.ID, &base.ID, theirs, "media-theirs-proposal")
	head := s.publish(t, admin, p, &base.ID, "media-ours-head")
	_, err := s.proposalService().Rebase(ctx, requestKey(c, "media-conflict-rebase"), q.ID, expected(q, &head.ID))
	conflicts, ok := err.(*fault.Conflicts)
	if !ok || len(conflicts.Fields) != 2 {
		t.Fatal("media bytes/description silently merged", err)
	}
	// A different authorized editor can restore this Page's historical original.
	_, editorToken := s.register(t, "media_restore_editor")
	editor := identity.Credentials{Bearer: editorToken}
	raw, _ := s.req(t, "POST", "pages/"+page.Page.ID+"/restore", map[string]any{"revision_id": base.ID, "expected_head_revision_id": head.ID}, editor, "media-restore-original", 201)
	restored := decodeResult[proposals.Proposal](t, raw)
	if restored.CurrentRevision.Snapshot.BlobID == nil || *restored.CurrentRevision.Snapshot.BlobID != firstBlob {
		t.Fatal("restore substituted current blob")
	}
	last := s.publish(t, admin, restored, &head.ID, "media-restored-head")
	s.req(t, "POST", "pages/"+page.Page.ID+"/proposals", map[string]any{"expected_head_revision_id": last.ID, "snapshot": content.Snapshot{Type: "media", Title: "Steal", Tags: []string{}, Description: ptr(""), BlobID: ptr(s.blobFixture(t, owner.ID, true))}}, editor, "unassociated-blob-key", 404)
	// Missing/unavailable originals must fail explicitly, without substituting head.
	execSQL(t, s.db.conn, "UPDATE wikit.blobs SET state='deleting' WHERE id='"+firstBlob+"'")
	s.req(t, "POST", "pages/"+page.Page.ID+"/restore", map[string]any{"revision_id": base.ID, "expected_head_revision_id": last.ID}, editor, "unavailable-original", 409)
}

func TestMergeAndRollbackRecheckFixedReferenceACL(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	ctx := context.Background()
	owner, token := s.register(t, "reference_owner")
	c := identity.Credentials{Bearer: token}
	blob := s.blobFixture(t, owner.ID, true)
	media := s.createPage(t, c, "public", content.Snapshot{Type: "media", Title: "Reference", Tags: []string{}, Description: ptr(""), BlobID: &blob}, "referenced-media-page")
	mediaRev := s.publish(t, admin, media.Proposal, nil, "referenced-media-publish")
	link := "![fixed](/api/v1/pages/" + media.Page.ID + "/revisions/" + mediaRev.ID + "/content)"
	doc := s.createPage(t, c, "public", document("Document", link), "reference-document-page")
	first := s.publish(t, admin, doc.Proposal, nil, "reference-document-publish")
	p := s.createProposal(t, c, doc.Page.ID, &first.ID, document("Document", link+"\nmore"), "reference-document-next")
	s.review(t, admin, p, &first.ID, "approve", "reference-next-review", 201)
	policy := access.Policy{PageID: media.Page.ID, Visibility: "restricted", Status: "published", Version: 2, Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose"}}}}
	s.seedPolicy(t, policy)
	_, otherToken := s.register(t, "reference_merger")
	other := identity.Credentials{Bearer: otherToken}
	// Give this actor review on the source only, preserving no target read grant.
	if _, err := s.access.UpdatePolicy(ctx, requestKey(admin, "reference-source-grant"), doc.Page.ID, access.Policy{Visibility: "public", Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose"}}, {SubjectType: "user", SubjectID: s.accountID(t, other), Capabilities: []string{"read", "review"}}}}, 1); err != nil {
		t.Fatal(err)
	}
	s.merge(t, other, p, &first.ID, "target-revoked-merge", 404)
	s.req(t, "POST", "pages/"+doc.Page.ID+"/restore", map[string]any{"revision_id": first.ID, "expected_head_revision_id": first.ID}, other, "target-revoked-restore", 404)
	s.req(t, "POST", "pages/"+doc.Page.ID+"/restore", map[string]any{"revision_id": mediaRev.ID, "expected_head_revision_id": first.ID}, c, "cross-page-restore", 404)
	raw, _ := s.req(t, "GET", "proposals/"+p.ID+"/diff", nil, other, "", 200)
	if strings.Contains(string(raw), "storage_key") {
		t.Fatal("diff leaked storage internals")
	}
	// Source diff exposes only its own text; target bytes still need target read.
	s.merge(t, admin, p, &first.ID, "reference-admin-merge", 200)
}
func (s *suite) accountID(t *testing.T, c identity.Credentials) string {
	t.Helper()
	a, err := s.accounts.Current(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

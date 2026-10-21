//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/postgres"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func ptr(s string) *string { return &s }
func document(title, markdown string) content.Snapshot {
	return content.Snapshot{Type: "document", Title: title, Tags: []string{" Go ", "go", "知识"}, Markdown: ptr(markdown)}
}

type pageFixture struct {
	Page     pages.Page         `json:"page"`
	Revision pages.Revision     `json:"revision"`
	Proposal proposals.Proposal `json:"proposal"`
}

// createPageWithProposal explicitly seeds an already-published Page and an editing proposal.
func (s *suite) createPageWithProposal(t *testing.T, c identity.Credentials, visibility string, snapshot content.Snapshot, key string) pageFixture {
	t.Helper()
	actor, err := s.accounts.Current(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if !actor.CanCreatePage {
		if _, err := s.db.conn.Exec(context.Background(), "UPDATE wikit.users SET create_page=true WHERE id=$1", actor.ID); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := s.req(t, "POST", "pages", map[string]any{"visibility": visibility, "snapshot": snapshot}, c, key, 201)
	out := decodeResult[pageFixture](t, raw)
	out.Proposal = s.createProposal(t, c, out.Page.ID, out.Page.HeadRevisionID, snapshot, key+"-proposal")
	return out
}

// publishFixture creates immutable publication data for read/constraint tests.
// It is intentionally not a runtime publishing use case; review/merge is #5.
func (s *suite) publishFixture(t *testing.T, created pageFixture, snapshot content.Snapshot) string {
	t.Helper()
	ctx := context.Background()
	id := s.id(t)
	tx, err := s.db.conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", postgres.SecurityLock); err != nil {
		t.Fatal(err)
	}
	var parent *string
	if err := tx.QueryRow(ctx, "SELECT head_revision_id::text FROM wikit.pages WHERE id=$1 FOR UPDATE", created.Page.ID).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	normalized, refs, err := content.Normalize(textadapter.Parser{}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO wikit.revisions(id,page_id,type,parent_id,source_proposal_id,snapshot,author_id,created_at,source_kind,publisher_id) VALUES($1,$2,$3,$4,$5,$6,$7,now(),'merge',$7)`, id, created.Page.ID, created.Page.Type, parent, created.Proposal.ID, raw, created.Proposal.AuthorID); err != nil {
		t.Fatal(err)
	}
	for _, tag := range normalized.Tags {
		if _, err := tx.Exec(ctx, "INSERT INTO wikit.tags VALUES($1) ON CONFLICT DO NOTHING", tag); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO wikit.revision_tags VALUES($1,$2)", id, tag); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range refs {
		if _, err := tx.Exec(ctx, "INSERT INTO wikit.revision_media_refs(revision_id,target_page_id,target_revision_id) VALUES($1,$2,$3)", id, ref.PageID, ref.RevisionID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE wikit.revisions SET sealed=true WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE wikit.pages SET head_revision_id=$2 WHERE id=$1", created.Page.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE wikit.page_policies SET status='published' WHERE page_id=$1", created.Page.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE wikit.proposals SET state='merged' WHERE id=$1", created.Proposal.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}
func (s *suite) nextProposalFixture(t *testing.T, previous pageFixture, snapshot content.Snapshot) pageFixture {
	t.Helper()
	id := s.id(t)
	candidate := s.id(t)
	p := proposals.Initial(id, previous.Page.ID, previous.Proposal.AuthorID, candidate, snapshot, time.Now().UTC())
	if err := s.db.store.WithinProposals(context.Background(), func(tx proposals.Tx) error {
		page, err := tx.Page(context.Background(), previous.Page.ID)
		if err != nil {
			return err
		}
		p.CurrentRevision.BaseRevisionID = page.HeadRevisionID
		return tx.InsertInitial(context.Background(), p, nil)
	}); err != nil {
		t.Fatal(err)
	}
	return pageFixture{Page: previous.Page, Proposal: p}
}
func TestPageCreationIsolationAndIdempotency(t *testing.T) {
	s := newSuite(t)
	creator, token := s.register(t, "page_creator")
	c := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	body := map[string]any{"visibility": "public", "snapshot": document("  Cafe\u0301  ", "published-body")}
	s.req(t, "POST", "pages", body, c, "create-page-key", 403)
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(admin, "grant-creation"), creator.ID, "member", "active", 1, true); err != nil {
		t.Fatal(err)
	}
	raw, _ := s.req(t, "POST", "pages", body, c, "create-page-key", 201)
	created := decodeResult[pages.Created](t, raw)
	if created.Page.ID != "2" || created.Page.HeadRevisionID == nil || created.Revision.ID != *created.Page.HeadRevisionID || created.Revision.SourceKind != "create" || created.Revision.SourceProposalID != nil || created.Revision.Snapshot.Title != "Café" {
		t.Fatal("incorrect direct publication")
	}
	raw, _ = s.req(t, "POST", "pages", body, c, "create-page-key", 201)
	if decodeResult[pages.Created](t, raw).Revision.ID != created.Revision.ID {
		t.Fatal("duplicate publication")
	}
	raw, _ = s.req(t, "GET", "pages/"+created.Page.ID, nil, identity.Credentials{}, "", 200)
	if !strings.Contains(string(raw), "published-body") {
		t.Fatal("first version not readable")
	}
	var count int
	if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit.proposals").Scan(&count); err != nil || count != 0 {
		t.Fatal("creation generated proposal", err)
	}
	for _, cap := range []string{"merge", "manage"} {
		if err := s.access.Check(context.Background(), c, created.Page.ID, cap, false); err == nil {
			t.Fatal("creator gained", cap)
		}
	}
	s.req(t, "POST", "pages", map[string]any{"visibility": "restricted", "snapshot": document("Private", "secret")}, c, "private-create-key", 201)
	s.req(t, "GET", "pages/3", nil, identity.Credentials{}, "", 404)
}
func TestCreatePageFailureRollsBackEveryRecord(t *testing.T) {
	s := newSuite(t)
	_, token := s.register(t, "atomic_creator")
	execSQL(t, s.db.conn, "UPDATE wikit.users SET create_page=true")
	execSQL(t, s.db.conn, `CREATE FUNCTION wikit.test_fail_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='page.create' THEN RAISE EXCEPTION 'injected failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_audit BEFORE INSERT ON wikit.audit_events FOR EACH ROW EXECUTE FUNCTION wikit.test_fail_audit()`)
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": document("Atomic", "body")}, identity.Credentials{Bearer: token}, "atomic-failure", 500)
	for _, table := range []string{"pages", "page_policies", "proposals", "proposal_revisions", "tags", "idempotency_records"} {
		var n int
		want := 0
		if table == "pages" || table == "page_policies" {
			want = 1
		}
		if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit."+table).Scan(&n); err != nil || n != want {
			t.Fatalf("partial %s records: %d (%v)", table, n, err)
		}
	}
}
func TestCurrentACLControlsHistoryAndPagination(t *testing.T) {
	s := newSuite(t)
	owner, token := s.register(t, "history_owner")
	reader, readerToken := s.register(t, "history_reader")
	c := identity.Credentials{Bearer: token}
	r := identity.Credentials{Bearer: readerToken}
	admin := identity.Credentials{Bearer: s.adminToken}
	one := s.createPageWithProposal(t, c, "public", document("One", "first publication"), "history-page-one")
	rev := s.publishFixture(t, one, document("One", "first publication"))
	two := s.createPageWithProposal(t, c, "public", document("Two", "other publication"), "history-page-two")
	s.publishFixture(t, two, document("Two", "other publication"))
	data, _ := s.req(t, "GET", "pages?limit=1&tag=GO", nil, identity.Credentials{}, "", 200)
	var list pages.List
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Next == nil {
		t.Fatal("missing directory pagination")
	}
	s.req(t, "GET", "pages?limit=1&tag=go&cursor="+*list.Next, nil, identity.Credentials{}, "", 200)
	s.req(t, "GET", "pages?limit=1&tag=other&cursor="+*list.Next, nil, identity.Credentials{}, "", 400)
	s.req(t, "GET", "pages?limit=1&tag=go&cursor="+*list.Next, nil, r, "", 400)
	s.req(t, "GET", "pages/"+two.Page.ID+"/revisions/"+rev, nil, c, "", 404)
	_, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "restrict-history"), one.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose", "discuss"}}, {SubjectType: "user", SubjectID: reader.ID, Capabilities: []string{"read"}}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pages/" + one.Page.ID, "pages/" + one.Page.ID + "/revisions", "pages/" + one.Page.ID + "/revisions/" + rev, "proposals/" + one.Proposal.ID, "proposals/" + one.Proposal.ID + "/revisions"} {
		s.req(t, "GET", path, nil, r, "", 200)
		s.req(t, "GET", path, nil, identity.Credentials{}, "", 404)
	}
	_, err = s.access.UpdatePolicy(context.Background(), requestKey(admin, "revoke-history"), one.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose", "discuss"}}}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pages/" + one.Page.ID, "pages/" + one.Page.ID + "/revisions", "pages/" + one.Page.ID + "/revisions/" + rev, "proposals/" + one.Proposal.ID, "proposals/" + one.Proposal.ID + "/revisions"} {
		s.req(t, "GET", path, nil, r, "", 404)
	}
	execSQL(t, s.db.conn, "UPDATE wikit.page_policies SET status='hidden' WHERE page_id='"+one.Page.ID+"'")
	s.req(t, "GET", "pages/"+one.Page.ID+"/revisions/"+rev, nil, c, "", 404)
}

func (s *suite) blobFixture(t *testing.T, uploader string, ready bool) string {
	t.Helper()
	id := s.id(t)
	state := "pending"
	if ready {
		state = "ready"
	}
	if _, err := s.db.conn.Exec(context.Background(), "INSERT INTO wikit.blobs(id,digest,size,mime,storage_key,state,uploader) VALUES($1,repeat('a',64),10,'image/png',$2,$3,$4)", id, "objects/"+id, state, uploader); err != nil {
		t.Fatal(err)
	}
	if ready {
		if err := os.WriteFile(filepath.Join(s.mediaRoot, "objects", id), []byte("\x89PNG\r\n\x1a\n00"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return id
}
func TestFixedMediaReferences(t *testing.T) {
	s := newSuite(t)
	owner, token := s.register(t, "media_owner")
	other, otherToken := s.register(t, "other_media_user")
	grantCreate(t, s, other.ID)
	c := identity.Credentials{Bearer: token}
	blob := s.blobFixture(t, owner.ID, true)
	media := content.Snapshot{Type: "media", Title: "Media", Tags: []string{}, Description: ptr("description"), BlobID: &blob}
	m := s.createPageWithProposal(t, c, "public", media, "create-media-one")
	old := s.publishFixture(t, m, media)
	link := "/api/v1/pages/" + m.Page.ID + "/revisions/" + old + "/content"
	doc := s.createPageWithProposal(t, c, "public", document("Reference", "![media]("+link+")"), "fixed-reference")
	newBlob := s.blobFixture(t, owner.ID, true)
	newMedia := media
	newMedia.BlobID = &newBlob
	next := s.nextProposalFixture(t, m, newMedia)
	latest := s.publishFixture(t, next, newMedia)
	data, _ := s.req(t, "GET", "proposals/"+doc.Proposal.ID, nil, c, "", 200)
	if !strings.Contains(string(data), old) || strings.Contains(string(data), latest) {
		t.Fatal("reference followed new head")
	}
	data, _ = s.req(t, "GET", "pages/"+m.Page.ID+"/revisions/"+old, nil, c, "", 200)
	if !strings.Contains(string(data), blob) || strings.Contains(string(data), newBlob) {
		t.Fatal("old media snapshot changed")
	}
	docRevision := s.publishFixture(t, doc, document("Reference", "![media]("+link+")"))
	rejectMediaAssociation(t, s, doc, doc.Page.ID, docRevision)
	rejectMediaAssociation(t, s, doc, m.Page.ID, docRevision)
	rejectSQL(t, s, "DELETE FROM wikit.blobs WHERE id=$1", blob)
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": media}, identity.Credentials{Bearer: otherToken}, "other-blob-create", 404)
	pending := s.blobFixture(t, owner.ID, false)
	media.BlobID = &pending
	raw, err := json.Marshal(media)
	if err != nil {
		t.Fatal(err)
	}
	rejectSQL(t, s, `INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,snapshot,created_at,sealed) VALUES($1,$2,$3,'media',$4,now(),true)`, s.id(t), m.Proposal.ID, m.Page.ID, raw)
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": media}, c, "pending-blob-create", 409)
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": document("Invalid", "![x](/api/v1/pages/"+m.Page.ID+"/content)")}, c, "unfixed-reference", 422)
	unpublished := s.createPageWithProposal(t, c, "public", document("Unpublished", "body"), "unpublished-target")
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": document("Invalid", "![x](/api/v1/pages/"+unpublished.Page.ID+"/revisions/"+unpublished.Proposal.CurrentRevision.ID+"/content)")}, c, "candidate-reference", 422)
}

func rejectMediaAssociation(t *testing.T, s *suite, source pageFixture, target, revision string) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	id := s.id(t)
	raw, err := json.Marshal(document("Association fixture", "body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,snapshot,created_at) VALUES($1,$2,$3,'document',$4,now())`, id, source.Proposal.ID, source.Page.ID, raw); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO wikit.proposal_revision_media_refs(revision_id,target_page_id,target_revision_id) VALUES($1,$2,$3)`, id, target, revision)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
		t.Fatalf("invalid media relation accepted or wrong error: %v", err)
	}
}

func rejectSQL(t *testing.T, s *suite, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, query, args...)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
		t.Error(rollbackErr)
	}
	if err == nil {
		t.Fatalf("database accepted forbidden mutation: %s", query)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
		t.Fatalf("expected a database constraint violation, got %v", err)
	}
}
func TestSnapshotImmutabilityAndCompositeRelationships(t *testing.T) {
	s := newSuite(t)
	_, token := s.register(t, "constraint_owner")
	c := identity.Credentials{Bearer: token}
	one := s.createPageWithProposal(t, c, "public", document("One", "one"), "constraint-one")
	rid := s.publishFixture(t, one, document("One", "one"))
	two := s.createPageWithProposal(t, c, "public", document("Two", "two"), "constraint-two")
	rejectSQL(t, s, "UPDATE wikit.revisions SET snapshot=jsonb_set(snapshot,'{title}','\"changed\"') WHERE id=$1", rid)
	rejectSQL(t, s, "DELETE FROM wikit.revisions WHERE id=$1", rid)
	rejectSQL(t, s, "UPDATE wikit.proposal_revisions SET snapshot='{}' WHERE id=$1", one.Proposal.CurrentRevision.ID)
	rejectSQL(t, s, "DELETE FROM wikit.proposal_revisions WHERE id=$1", one.Proposal.CurrentRevision.ID)
	rejectSQL(t, s, "TRUNCATE wikit.revisions CASCADE")
	rejectSQL(t, s, "UPDATE wikit.pages SET type='media' WHERE id=$1", one.Page.ID)
	rejectSQL(t, s, "UPDATE wikit.pages SET head_revision_id=$2 WHERE id=$1", two.Page.ID, rid)
	rejectSQL(t, s, "UPDATE wikit.proposals SET current_revision_id=$2 WHERE id=$1", one.Proposal.ID, two.Proposal.CurrentRevision.ID)
	rejectSQL(t, s, "INSERT INTO wikit.proposal_revision_tags VALUES($1,'go')", one.Proposal.CurrentRevision.ID)
	rejectSQL(t, s, "INSERT INTO wikit.revision_tags VALUES($1,'knowledge')", rid)
	newID := s.id(t)
	raw, _ := json.Marshal(document("Bad parent", "body"))
	rejectSQL(t, s, `INSERT INTO wikit.revisions(id,page_id,type,parent_id,source_proposal_id,snapshot,author_id,created_at,sealed) VALUES($1,$2,'document',$3,$4,$5,$6,now(),true)`, newID, two.Page.ID, rid, two.Proposal.ID, raw, two.Proposal.AuthorID)
	rejectSQL(t, s, `INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,base_revision_id,snapshot,created_at,sealed) VALUES($1,$2,$3,'document',$4,$5,now(),true)`, s.id(t), two.Proposal.ID, two.Page.ID, rid, raw)
	// The deferred seal constraint also forbids committing a half-built candidate.
	rejectSQL(t, s, `INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,snapshot,created_at) VALUES($1,$2,$3,'document',$4,now())`, s.id(t), two.Proposal.ID, two.Page.ID, raw)
}

func TestConcurrentCreateAndLargeDocument(t *testing.T) {
	s := newSuite(t)
	u, token := s.register(t, "parallel_page_creator")
	grantCreate(t, s, u.ID)
	c := identity.Credentials{Bearer: token}
	svc := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan pages.Created, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, err := svc.Create(ctx, requestKey(c, "concurrent-create-key"), "public", document("Concurrent", "body"), "same-ip")
			results <- value
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for result := range results {
		if id != "" && id != result.Page.ID {
			t.Fatal("duplicate creation")
		}
		id = result.Page.ID
	}
	var count int
	if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.pages").Scan(&count); err != nil || count != 2 {
		t.Fatal("non-atomic concurrent creation", count, err)
	}
	s.createPageWithProposal(t, c, "public", document("Large", strings.Repeat("字", 70000)), "large-document-key")
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": document("NUL", "bad\x00text")}, c, "nul-document-key", 422)
	s.req(t, "GET", "pages?tag=%00", nil, c, "", 400)
}

func grantCreate(t *testing.T, s *suite, id string) {
	t.Helper()
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(identity.Credentials{Bearer: s.adminToken}, "grant-create-"+id), id, "member", "active", 1, true); err != nil {
		t.Fatal(err)
	}
}

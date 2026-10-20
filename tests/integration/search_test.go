//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "github.com/epoch-fresh-2026/Wikit/internal/adapters/http"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/health"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/epoch-fresh-2026/Wikit/internal/search/meili"
	"github.com/epoch-fresh-2026/Wikit/internal/storage/blobfs"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func newSearchSuite(t *testing.T) *suite {
	t.Helper()
	if os.Getenv("WIKIT_MEILI_URL") == "" || os.Getenv("WIKIT_MEILI_KEY") == "" {
		t.Fatal("WIKIT_MEILI_URL and WIKIT_MEILI_KEY are required for real Meilisearch integration tests")
	}
	s := newSuite(t)
	return s
}
func (s *suite) drain(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := s.worker.Process(ctx, 1000); err != nil {
		t.Fatal("projection drain", err)
	}
}
func (s *suite) find(t *testing.T, c identity.Credentials, q string) search.Result {
	t.Helper()
	raw, _ := s.req(t, "GET", "search?q="+url.QueryEscape(q), nil, c, "", 200)
	return decodeResult[search.Result](t, raw)
}
func (s *suite) searchPage(t *testing.T, c identity.Credentials, visibility string, snapshot content.Snapshot, key string) pages.Created {
	t.Helper()
	p := s.createPage(t, c, visibility, snapshot, key+"-page")
	revision := s.publish(t, c, p.Proposal, nil, key)
	p.Page.HeadRevisionID = &revision.ID
	p.Page.Status = "published"
	return p
}
func activeIndex(t *testing.T, s *suite) string {
	t.Helper()
	var index string
	if err := s.db.conn.QueryRow(context.Background(), `SELECT active_index FROM wikit.search_state`).Scan(&index); err != nil {
		t.Fatal(err)
	}
	return index
}

func TestSEARCH01ChineseEnglishRankingAndPlainText(t *testing.T) {
	s := newSearchSuite(t)
	_, c := s.reviewer(t, "search_corpus_author")
	title := s.searchPage(t, c, "public", content.Snapshot{Type: "document", Title: "知识管理 Orbit", Tags: []string{}, Markdown: ptr("团队协作基础。")}, "search-title")
	tagged := s.searchPage(t, c, "public", content.Snapshot{Type: "document", Title: "分类页面", Tags: []string{"知识", "orbit"}, Markdown: ptr("归档资料。")}, "search-tag")
	body := s.searchPage(t, c, "public", content.Snapshot{Type: "document", Title: "正文页面", Tags: []string{}, Markdown: ptr("知识管理能够帮助团队共享文档。 orbit catalog **searchable** [label](https://notindexed.example/private-target)\n\n<script>notindexedscript</script>")}, "search-body")
	draft := s.createPage(t, c, "public", document("draftonlyneedle", "draftonlyneedle"), "search-draft")
	s.issue(t, c, draft.Page.ID, "search-draft-issue")
	rawIssue, _ := s.req(t, "POST", "pages/"+title.Page.ID+"/issues", map[string]any{"title": "discussiononlyneedle", "body": "discussiononlyneedle"}, c, "search-unindexed-issue", 201)
	var issue struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rawIssue, &issue); err != nil {
		t.Fatal(err)
	}
	s.req(t, "POST", "issues/"+issue.ID+"/comments", map[string]any{"body": "commentonlyneedle"}, c, "search-unindexed-comment", 201)
	blob := s.uploadPNG(t, c, "search-media-upload")
	media := s.searchPage(t, c, "public", content.Snapshot{Type: "media", Title: "媒体图片", Tags: []string{}, Description: ptr("descriptiononlyneedle"), BlobID: &blob.ID}, "search-media")
	s.drain(t)
	for _, query := range []string{"知识", "知识管理", "orbit", "知识 orbit"} {
		r := s.find(t, identity.Credentials{}, query)
		if len(r.Items) < 2 || r.Items[0].PageID != title.Page.ID {
			t.Fatalf("query %q ranking: %+v", query, r.Items)
		}
	}
	r := s.find(t, identity.Credentials{}, "orbit")
	if len(r.Items) != 3 || r.Items[1].PageID != tagged.Page.ID || r.Items[2].PageID != body.Page.ID {
		t.Fatalf("title/tag/body rank: %+v", r.Items)
	}
	for _, query := range []string{"draftonlyneedle", "notindexedscript", "private-target", "discussiononlyneedle", "commentonlyneedle", "real-original-bytes"} {
		if got := s.find(t, c, query); len(got.Items) != 0 {
			t.Fatalf("indexed excluded content: %q", query)
		}
	}
	r = s.find(t, c, "searchable")
	if len(r.Items) != 1 || strings.ContainsAny(r.Items[0].Snippet, "<>*") {
		t.Fatal("snippet contains markup")
	}
	s.req(t, "GET", "search?q=orbit&filter=visibility%3Dpublic", nil, c, "", 400)
	for _, q := range []string{"", "%20%20", "%00", url.QueryEscape(strings.Repeat("字", 501))} {
		s.req(t, "GET", "search?q="+q, nil, c, "", 400)
	}
	for _, q := range []string{"!@#$%", "orb", "知识管理能够帮助团队"} {
		s.find(t, c, q)
	}
	long := s.find(t, c, "知识管理能够帮助团队")
	found := false
	for _, hit := range long.Items {
		if hit.PageID == body.Page.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("long Chinese phrase missed body")
	}
	if r := s.find(t, c, "descriptiononlyneedle"); len(r.Items) != 1 || r.Items[0].PageID != media.Page.ID || r.Items[0].Type != "media" {
		t.Fatal("media description missing")
	}
	raw, _ := s.req(t, "GET", "search?q=orbit&tag=ORBIT", nil, c, "", 200)
	filtered := decodeResult[search.Result](t, raw)
	if len(filtered.Items) != 1 || filtered.Items[0].PageID != tagged.Page.ID {
		t.Fatal("tag normalization/filter")
	}
}

func TestSEARCH02CurrentACLVersionAndCursorIsolation(t *testing.T) {
	s := newSearchSuite(t)
	author, c := s.reviewer(t, "search_acl_author")
	reader, token := s.register(t, "search_reader")
	rc := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.searchPage(t, c, "public", document("visibilityneedle", "old searchable text"), "search-visible")
	other := s.searchPage(t, c, "public", document("visibilityneedle second", "other"), "search-second")
	_ = other
	s.drain(t)
	raw, _ := s.req(t, "GET", "search?q=visibilityneedle&limit=1", nil, rc, "", 200)
	page := decodeResult[search.Result](t, raw)
	if page.Next == nil {
		t.Fatal("missing cursor")
	}
	cursor := url.QueryEscape(*page.Next)
	s.req(t, "GET", "search?q=visibilityneedle&limit=1&cursor="+cursor, nil, c, "", 400)
	s.req(t, "GET", "search?q=other&limit=1&cursor="+cursor, nil, rc, "", 400)
	policy := access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: reader.ID, Capabilities: []string{"read"}}}}
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "search-restrict-key"), p.Page.ID, policy, 1); err != nil {
		t.Fatal(err)
	}
	if r := s.find(t, identity.Credentials{}, "visibilityneedle"); len(r.Items) != 1 {
		t.Fatal("stale public ACL leaked private page")
	}
	// A readable but stale policy version is also withheld until repaired.
	if r := s.find(t, rc, "visibilityneedle"); len(r.Items) != 1 {
		t.Fatal("stale policy version was returned")
	}
	s.drain(t)
	if r := s.find(t, rc, "visibilityneedle"); len(r.Items) != 2 {
		t.Fatal("new ACL did not reach index")
	}
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "search-revoke-key"), p.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: author.ID, Capabilities: []string{"read"}}}}, 2); err != nil {
		t.Fatal(err)
	}
	if r := s.find(t, rc, "visibilityneedle"); len(r.Items) != 1 {
		t.Fatal("revoked private hit leaked")
	}
	// Old head text must not produce a current result under an obsolete query.
	proposal := s.createProposal(t, c, other.Page.ID, other.Page.HeadRevisionID, document("replacementneedle", "new body"), "search-replacement")
	s.publish(t, c, proposal, other.Page.HeadRevisionID, "search-replace-publish")
	if r := s.find(t, rc, "visibilityneedle"); len(r.Items) != 0 {
		t.Fatal("stale head query returned current snippet")
	}
	s.drain(t)
	if r := s.find(t, rc, "replacementneedle"); len(r.Items) != 1 {
		t.Fatal("new head missing")
	}
	// A late obsolete document is never returned and schedules a repair.
	uid, err := s.index.Put(context.Background(), activeIndex(t, s), []search.Document{{ID: other.Page.ID, Revision: *other.Page.HeadRevisionID, PolicyVersion: 1, Type: "document", Title: "visibilityneedle second", Tags: []string{}, Body: "stale private text", Visibility: "public", Users: []string{}, Groups: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.index.Wait(context.Background(), uid); err != nil {
		t.Fatal(err)
	}
	if r := s.find(t, rc, "visibilityneedle"); len(r.Items) != 0 {
		t.Fatal("late document leaked stale text")
	}
	s.drain(t)
	if r := s.find(t, rc, "replacementneedle"); len(r.Items) != 1 {
		t.Fatal("repair did not restore current head")
	}
	if _, err := s.worker.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.req(t, "GET", "search?q=visibilityneedle&limit=1&cursor="+cursor, nil, rc, "", 400)
}

func TestSEARCH03RebuildFailureRecoveryAndLateEvents(t *testing.T) {
	s := newSearchSuite(t)
	_, c := s.reviewer(t, "search_rebuild_author")
	p := s.searchPage(t, c, "public", document("recoverneedle", "current body"), "search-recover")
	s.drain(t)
	old := activeIndex(t, s)
	bad := meili.New("http://127.0.0.1:1", "local_test_key_2026")
	failed := search.NewWorker(s.db.store, bad, textadapter.Parser{}, s.secrets)
	if _, err := failed.Rebuild(context.Background()); err == nil {
		t.Fatal("unavailable rebuild succeeded")
	}
	if activeIndex(t, s) != old {
		t.Fatal("failed rebuild switched active index")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name, err := s.worker.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name == old || activeIndex(t, s) != name {
		t.Fatal("rebuild did not switch")
	}
	if r := s.find(t, c, "recoverneedle"); len(r.Items) != 1 {
		t.Fatal("rebuilt index missing current page")
	}
	// Duplicate/late publication event projects current state rather than its old revision.
	execSQL(t, s.db.conn, `UPDATE wikit.outbox_events SET completed_at=NULL WHERE page_id='`+p.Page.ID+`'`)
	s.drain(t)
	if r := s.find(t, c, "recoverneedle"); len(r.Items) != 1 {
		t.Fatal("duplicate event changed results")
	}
	down := search.New(s.db.store, bad, textadapter.Parser{}, s.secrets)
	if _, err := down.Search(ctx, c, search.Query{Q: "recoverneedle", Limit: 20}); !errors.Is(err, fault.SearchUnavailable) {
		t.Fatal("search outage", err)
	}
	s.req(t, "GET", "pages/"+p.Page.ID, nil, c, "", 200)
}

// The port wrapper interrupts only task observation, preserving the actual submitted task.
type delayedTask struct {
	search.Index
	submissions int
	failWait    bool
}

func (d *delayedTask) Put(ctx context.Context, index string, docs []search.Document) (int64, error) {
	d.submissions++
	return d.Index.Put(ctx, index, docs)
}
func (d *delayedTask) Wait(ctx context.Context, id int64) error {
	if d.failWait {
		return fault.SearchUnavailable
	}
	return d.Index.Wait(ctx, id)
}
func TestSEARCH03DurableTaskCheckpointAndRetry(t *testing.T) {
	s := newSearchSuite(t)
	_, c := s.reviewer(t, "search_task_author")
	s.searchPage(t, c, "public", document("taskneedle", "body"), "search-task")
	// Prepare settings before interrupting document task observation.
	if err := s.index.Ensure(context.Background(), activeIndex(t, s)); err != nil {
		t.Fatal(err)
	}
	d := &delayedTask{Index: s.index, failWait: true}
	w := search.NewWorker(s.db.store, d, textadapter.Parser{}, s.secrets)
	if err := w.Process(context.Background(), 1); !errors.Is(err, fault.SearchUnavailable) {
		t.Fatal(err)
	}
	var uid *int64
	var attempts int
	if err := s.db.conn.QueryRow(context.Background(), `SELECT task_uid,attempts FROM wikit.outbox_events WHERE completed_at IS NULL ORDER BY sequence LIMIT 1`).Scan(&uid, &attempts); err != nil || uid == nil || attempts != 1 {
		t.Fatalf("checkpoint missing %v %d %v", uid, attempts, err)
	}
	d.failWait = false
	if err := w.Retry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.Process(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if d.submissions != 1 {
		t.Fatal("known task resubmitted")
	}
	stats, err := w.Status(context.Background())
	if err != nil || stats.Pending != 0 || stats.Failures != 1 {
		t.Fatalf("stats %+v %v", stats, err)
	}
	raw, _ := s.req(t, "GET", "search?q=taskneedle", nil, c, "", 200)
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 2 || response["items"] == nil || response["next_cursor"] == nil {
		t.Fatal("exposed engine metadata")
	}
}

type heldIndex struct {
	search.Index
	entered, release chan struct{}
	once             sync.Once
}

func (h *heldIndex) Put(ctx context.Context, index string, docs []search.Document) (int64, error) {
	h.once.Do(func() { close(h.entered); <-h.release })
	return h.Index.Put(ctx, index, docs)
}

func TestSEARCH03RebuildCatchesConcurrentPublication(t *testing.T) {
	s := newSearchSuite(t)
	_, c := s.reviewer(t, "search_catchup_author")
	p := s.searchPage(t, c, "public", document("beforecatchup", "old"), "catchup-initial")
	s.drain(t)
	h := &heldIndex{Index: s.index, entered: make(chan struct{}), release: make(chan struct{})}
	w := search.NewWorker(s.db.store, h, textadapter.Parser{}, s.secrets)
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := w.Rebuild(ctx)
		done <- err
	}()
	awaitBarrier(t, h.entered)
	if err := s.worker.Process(context.Background(), 1); !errors.Is(err, fault.State) {
		t.Fatal("concurrent worker bypassed rebuild lock", err)
	}
	next := s.createProposal(t, c, p.Page.ID, p.Page.HeadRevisionID, document("aftercatchup", "new"), "catchup-proposal")
	s.publish(t, c, next, p.Page.HeadRevisionID, "catchup-publish")
	close(h.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r := s.find(t, c, "aftercatchup"); len(r.Items) != 1 {
		t.Fatal("rebuild missed committed increment")
	}
	if r := s.find(t, c, "beforecatchup"); len(r.Items) != 0 {
		t.Fatal("rebuild revived old text")
	}
	s.drain(t)
	if r := s.find(t, c, "aftercatchup"); len(r.Items) != 1 {
		t.Fatal("late old event revived content")
	}
}

type heldSearch struct {
	search.Index
	entered, release chan struct{}
	once             sync.Once
}

func (h *heldSearch) Search(ctx context.Context, index, q string, f search.Filter, offset, limit int) (search.Candidates, error) {
	out, err := h.Index.Search(ctx, index, q, f, offset, limit)
	h.once.Do(func() { close(h.entered); <-h.release })
	return out, err
}
func TestSEARCH02FinalAuthorizationAfterCandidateFetch(t *testing.T) {
	s := newSearchSuite(t)
	_, c := s.reviewer(t, "search_race_author")
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.searchPage(t, c, "public", document("barrierneedle", "sensitive body"), "search-barrier")
	s.drain(t)
	h := &heldSearch{Index: s.index, entered: make(chan struct{}), release: make(chan struct{})}
	svc := search.New(s.db.store, h, textadapter.Parser{}, s.secrets)
	type outcome struct {
		result search.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := svc.Search(context.Background(), c, search.Query{Q: "barrierneedle", Limit: 20})
		done <- outcome{out, err}
	}()
	awaitBarrier(t, h.entered)
	s.req(t, "POST", "pages/"+p.Page.ID+"/moderation", map[string]any{"hidden": true, "reason": "race hide", "expected_version": 1}, admin, "search-race-hide", 200)
	close(h.release)
	got := <-done
	if got.err != nil || len(got.result.Items) != 0 {
		t.Fatalf("post-fetch governance leaked: %+v %v", got.result, got.err)
	}
	s.req(t, "POST", "pages/"+p.Page.ID+"/moderation", map[string]any{"hidden": false, "reason": "restore", "expected_version": 2}, admin, "search-race-restore", 200)
	s.drain(t)
	if r := s.find(t, c, "barrierneedle"); len(r.Items) != 1 {
		t.Fatal("restore projection missing")
	}
}

func TestSEARCH02GroupMembershipAndDisabledAccount(t *testing.T) {
	s := newSearchSuite(t)
	author, c := s.reviewer(t, "search_group_author")
	reader, token := s.register(t, "search_group_reader")
	rc := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	ctx := context.Background()
	raw, err := s.access.SaveGroup(ctx, requestKey(admin, "search-create-group"), "", "Search readers", []string{reader.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	group := decodeResult[access.Group](t, raw)
	p := s.searchPage(t, c, "public", document("groupneedle", "secret"), "search-group-page")
	policy := access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: author.ID, Capabilities: []string{"read"}}, {SubjectType: "group", SubjectID: group.ID, Capabilities: []string{"read"}}}}
	if _, err := s.access.UpdatePolicy(ctx, requestKey(admin, "search-group-policy"), p.Page.ID, policy, 1); err != nil {
		t.Fatal(err)
	}
	s.drain(t)
	if r := s.find(t, rc, "groupneedle"); len(r.Items) != 1 {
		t.Fatal("group reader not found")
	}
	if r := s.find(t, identity.Credentials{}, "groupneedle"); len(r.Items) != 0 {
		t.Fatal("private group hit leaked")
	}
	if _, err := s.access.SaveGroup(ctx, requestKey(admin, "search-remove-member"), group.ID, group.Name, []string{}, 1); err != nil {
		t.Fatal(err)
	}
	if r := s.find(t, rc, "groupneedle"); len(r.Items) != 0 {
		t.Fatal("group removal not immediate")
	}
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "search-disable-reader"), reader.ID, "member", "disabled", 1); err != nil {
		t.Fatal(err)
	}
	s.req(t, "GET", "search?q=groupneedle", nil, rc, "", 401)
}

func TestSEARCH03FailedTaskBackoffAndPageOrder(t *testing.T) {
	s := newSearchSuite(t)
	author, c := s.reviewer(t, "search_failed_task")
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.searchPage(t, c, "public", document("retryneedle", "current"), "search-failed-task")
	ctx := context.Background()
	index := activeIndex(t, s)
	if err := s.index.Ensure(ctx, index); err != nil {
		t.Fatal(err)
	}
	uid, err := s.index.Put(ctx, index, []search.Document{{ID: ""}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.index.Wait(ctx, uid); !errors.Is(err, search.ErrTaskFailed) {
		t.Fatal("invalid document task did not fail", err)
	}
	if _, err := s.db.conn.Exec(ctx, `UPDATE wikit.outbox_events SET task_uid=$1,target_index=$2 WHERE page_id=$3`, uid, index, p.Page.ID); err != nil {
		t.Fatal(err)
	}
	policy := access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: author.ID, Capabilities: []string{"read"}}}}
	if _, err := s.access.UpdatePolicy(ctx, requestKey(admin, "search-later-event"), p.Page.ID, policy, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Process(ctx, 10); !errors.Is(err, fault.SearchUnavailable) {
		t.Fatal(err)
	}
	var pending *search.Event
	if err := s.db.store.WithSearchWorker(ctx, func(q search.Queue) error { var err error; pending, err = q.NextEvent(ctx); return err }); err != nil {
		t.Fatal(err)
	}
	if pending != nil {
		t.Fatal("later same-Page event overtook retry")
	}
	if err := s.worker.Retry(ctx); err != nil {
		t.Fatal(err)
	}
	s.drain(t)
	if r := s.find(t, c, "retryneedle"); len(r.Items) != 1 {
		t.Fatal("failed task did not recover")
	}
	if r := s.find(t, identity.Credentials{}, "retryneedle"); len(r.Items) != 0 {
		t.Fatal("old publication event restored public ACL")
	}
}

func TestSEARCH03HTTPOutageKeepsDatabaseAvailable(t *testing.T) {
	s := newSearchSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	p := s.createPage(t, admin, "public", document("Outage page", "database body"), "search-outage-page")
	bad := meili.New("http://127.0.0.1:1", "local_outage_test_key_2026")
	api := httpadapter.API{Search: search.New(s.db.store, bad, textadapter.Parser{}, s.secrets), Identity: s.accounts, Access: s.access, Pages: pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files), Secrets: s.secrets, PublicURL: "http://localhost:8080", SessionTTL: time.Hour}
	original := s.server
	s.server = httptest.NewServer(api.Handler(httpadapter.NewHandler(health.New(s.db.store, blobfs.Probe{Root: s.mediaRoot}))))
	defer func() { s.server.Close(); s.server = original }()
	raw, _ := s.req(t, "GET", "search?q=anything", nil, admin, "", 503)
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &failure); err != nil || failure.Error.Code != "search_unavailable" {
		t.Fatalf("wrong search outage error: %v", err)
	}
	s.req(t, "GET", "pages/"+p.Page.ID, nil, admin, "", 200)
	s.createPage(t, admin, "public", document("Outage write", "new body"), "search-outage-write")
	res, err := http.Get(s.server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("search outage disabled readiness: %d", res.StatusCode)
	}
}

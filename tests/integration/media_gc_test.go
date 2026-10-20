//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	textadapter "github.com/epoch-fresh-2026/Wikit/internal/text"
)

func (s *suite) ageBlob(t *testing.T, id string) {
	t.Helper()
	if _, err := s.db.conn.Exec(context.Background(), "UPDATE wikit.blobs SET created_at=now()-interval '8 days' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}
func TestMEDIA03GarbageRetainsAllCandidateAndPublishedHistory(t *testing.T) {
	s := newSuite(t)
	admin := identity.Credentials{Bearer: s.adminToken}
	ctx := context.Background()
	unreferenced := s.uploadPNG(t, admin, "gc-unused-upload")
	closedBlob := s.uploadPNG(t, admin, "gc-closed-upload")
	snapshot := content.Snapshot{Type: "media", Title: "Closed", Tags: []string{}, Description: ptr(""), BlobID: &closedBlob.ID}
	closed := s.createPage(t, admin, "public", snapshot, "gc-closed-page")
	s.req(t, "POST", "proposals/"+closed.Proposal.ID+"/state", map[string]any{"state": "closed", "expected_proposal_version": 1}, admin, "gc-close-proposal", 200)
	firstBlob := s.uploadPNG(t, admin, "gc-first-upload")
	snapshot.BlobID = &firstBlob.ID
	page := s.createPage(t, admin, "public", snapshot, "gc-history-page")
	first := s.publish(t, admin, page.Proposal, nil, "gc-first-publication")
	secondBlob := s.uploadPNG(t, admin, "gc-second-upload")
	snapshot.BlobID = &secondBlob.ID
	p := s.createProposal(t, admin, page.Page.ID, &first.ID, snapshot, "gc-second-proposal")
	s.publish(t, admin, p, &first.ID, "gc-second-publication")
	for _, id := range []string{unreferenced.ID, closedBlob.ID, firstBlob.ID, secondBlob.ID} {
		s.ageBlob(t, id)
	}
	out, err := s.media.Collect(ctx, 7*24*time.Hour)
	if err != nil || out.Blobs != 1 {
		t.Fatal("incorrect collection", out, err)
	}
	if _, err := os.Stat(filepath.Join(s.mediaRoot, "objects", unreferenced.ID)); !os.IsNotExist(err) {
		t.Fatal("unused original retained")
	}
	for _, id := range []string{closedBlob.ID, firstBlob.ID, secondBlob.ID} {
		if _, err := os.Stat(filepath.Join(s.mediaRoot, "objects", id)); err != nil {
			t.Fatal("history original removed", err)
		}
	}
	s.binary(t, "GET", *first.ContentURL, nil, identity.Credentials{}, nil, 200)
	s.binary(t, "GET", "/api/v1/proposals/"+closed.Proposal.ID+"/revisions/"+closed.Proposal.CurrentRevision.ID+"/content", nil, admin, nil, 200)
	// Repeated runs are safe; a ready upload whose unused blob was collected is failed.
	if _, err := s.media.Collect(ctx, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
}

type failFiles struct {
	media.BlobStore
	stage, commit, remove bool
}

func (f failFiles) Stage(ctx context.Context, id string, r io.Reader, n int64) (media.Staged, error) {
	if f.stage {
		return media.Staged{}, fault.StorageUnavailable
	}
	return f.BlobStore.Stage(ctx, id, r, n)
}
func (f failFiles) Commit(ctx context.Context, key, id string) (string, error) {
	if f.commit {
		return "", fault.StorageUnavailable
	}
	return f.BlobStore.Commit(ctx, key, id)
}
func (f failFiles) Remove(ctx context.Context, key string) error {
	if f.remove {
		return fault.StorageUnavailable
	}
	return f.BlobStore.Remove(ctx, key)
}
func TestMEDIA02FilesystemAndDatabaseFailureOrphans(t *testing.T) {
	for _, mode := range []string{"stage", "commit", "database"} {
		t.Run(mode, func(t *testing.T) {
			s := newSuite(t)
			c := identity.Credentials{Bearer: s.adminToken}
			ctx := context.Background()
			u := s.prepareUpload(t, c, "image/png", len(pngBody), "failure-orphan-upload")
			files := failFiles{BlobStore: s.files, stage: mode == "stage", commit: mode == "commit"}
			if mode == "database" {
				execSQL(t, s.db.conn, `CREATE FUNCTION wikit.fail_upload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='upload.complete' THEN RAISE EXCEPTION 'injected upload database failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_upload BEFORE INSERT ON wikit.audit_events FOR EACH ROW EXECUTE FUNCTION wikit.fail_upload()`)
			}
			svc := media.New(s.db.store, files, s.secrets, 104857600)
			if _, err := svc.Put(ctx, c, u.ID, "application/octet-stream", "failure-upload-request", "test-upload-ip", bytes.NewReader(pngBody)); err == nil {
				t.Fatal("injected upload failure accepted")
			}
			var n int
			if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.blobs").Scan(&n); err != nil || n != 0 {
				t.Fatal("failed original became ready", n, err)
			}
			state, err := s.media.Get(ctx, c, u.ID)
			if err != nil || state.State != "failed" || state.BlobID != nil {
				t.Fatal("failure state/lease not cleared", err)
			}
			entries, err := os.ReadDir(filepath.Join(s.mediaRoot, "objects"))
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if mode == "database" {
				want = 1
			}
			if len(entries) != want {
				t.Fatal("unexpected orphan count", len(entries), want)
			}
			old := time.Now().Add(-8 * 24 * time.Hour)
			for _, entry := range entries {
				if err := os.Chtimes(filepath.Join(s.mediaRoot, "objects", entry.Name()), old, old); err != nil {
					t.Fatal(err)
				}
			}
			out, err := s.media.Collect(ctx, 7*24*time.Hour)
			if err != nil || out.Orphans != want {
				t.Fatal("orphan not reclaimed", out, err)
			}
			if mode == "database" {
				execSQL(t, s.db.conn, "DROP TRIGGER fail_upload ON wikit.audit_events")
			}
			// Same reservation can retry after the failure, without an old active lease.
			if _, err := s.media.Put(ctx, c, u.ID, "application/octet-stream", "retry-upload-request", "test-upload-ip", bytes.NewReader(pngBody)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestMEDIA03DeletionFailureKeepsTombstoneAndRetries(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	c := identity.Credentials{Bearer: s.adminToken}
	b := s.uploadPNG(t, c, "gc-delete-failure")
	s.ageBlob(t, b.ID)
	svc := media.New(s.db.store, failFiles{BlobStore: s.files, remove: true}, s.secrets, 104857600)
	if _, err := svc.Collect(ctx, 7*24*time.Hour); !errors.Is(err, fault.StorageUnavailable) {
		t.Fatal(err)
	}
	var state string
	if err := s.db.conn.QueryRow(ctx, "SELECT state FROM wikit.blobs WHERE id=$1", b.ID).Scan(&state); err != nil || state != "deleting" {
		t.Fatal("delete intent rolled back", state, err)
	}
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": content.Snapshot{Type: "media", Title: "Deleting", Tags: []string{}, Description: ptr(""), BlobID: &b.ID}}, c, "gc-deleting-proposal", 409)
	if _, err := s.media.Collect(ctx, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.db.conn.QueryRow(ctx, "SELECT state FROM wikit.blobs WHERE id=$1", b.ID).Scan(&state); err != nil || state != "deleted" {
		t.Fatal("deletion did not finish", err)
	}
}

type heldGarbage struct {
	media.Repository
	entered, release chan struct{}
}
type heldGarbageTx struct {
	media.Tx
	entered, release chan struct{}
}

func (h heldGarbage) WithinMedia(ctx context.Context, fn func(media.Tx) error) error {
	return h.Repository.WithinMedia(ctx, func(tx media.Tx) error { return fn(heldGarbageTx{tx, h.entered, h.release}) })
}
func (h heldGarbageTx) ClaimGarbage(ctx context.Context, cutoff time.Time, limit int) ([]media.Stored, error) {
	rows, err := h.Tx.ClaimGarbage(ctx, cutoff, limit)
	if err != nil {
		return nil, err
	}
	close(h.entered)
	select {
	case <-h.release:
		return rows, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type heldPage struct {
	pages.Repository
	entered, release chan struct{}
}
type heldPageTx struct {
	pages.Tx
	entered, release chan struct{}
}

func (h heldPage) WithinPages(ctx context.Context, fn func(pages.Tx) error) error {
	return h.Repository.WithinPages(ctx, func(tx pages.Tx) error { return fn(heldPageTx{tx, h.entered, h.release}) })
}
func (h heldPageTx) InsertInitial(ctx context.Context, p proposals.Proposal, refs []content.MediaRef) error {
	if err := h.Tx.InsertInitial(ctx, p, refs); err != nil {
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
func TestMEDIA03GCAndFirstReferenceCommitOrders(t *testing.T) {
	for _, gcFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(gcFirst), func(t *testing.T) {
			s := newSuite(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			c := identity.Credentials{Bearer: s.adminToken}
			b := s.uploadPNG(t, c, "gc-reference-race")
			s.ageBlob(t, b.ID)
			entered, release := make(chan struct{}), make(chan struct{})
			gc := s.media
			creator := pages.New(s.db.store, s.secrets, textadapter.Parser{}, s.files)
			if gcFirst {
				gc = media.New(heldGarbage{s.db.store, entered, release}, s.files, s.secrets, 104857600)
			} else {
				creator = pages.New(heldPage{s.db.store, entered, release}, s.secrets, textadapter.Parser{}, s.files)
			}
			create := func() error {
				_, err := creator.Create(ctx, requestKey(c, "gc-create-race-page"), "public", content.Snapshot{Type: "media", Title: "Race", Tags: []string{}, Description: ptr(""), BlobID: &b.ID}, "race-ip")
				return err
			}
			collect := func() error { _, err := gc.Collect(ctx, 7*24*time.Hour); return err }
			first, second := create, collect
			if gcFirst {
				first, second = collect, create
			}
			a, z := make(chan error, 1), make(chan error, 1)
			go func() { a <- first() }()
			awaitBarrier(t, entered)
			go func() { z <- second() }()
			waitForAdvisoryWaiter(t, s.db.conn)
			close(release)
			if err := <-a; err != nil {
				t.Fatal(err)
			}
			err := <-z
			if gcFirst && !errors.Is(err, fault.State) || !gcFirst && err != nil {
				t.Fatal("wrong GC/reference serial outcome", err)
			}
			var n int
			if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.proposal_revisions").Scan(&n); err != nil {
				t.Fatal(err)
			}
			want := 1
			if gcFirst {
				want = 0
			}
			if n != want {
				t.Fatal("partially referenced deletion")
			}
		})
	}
}

func TestMEDIA03GCSerializesWithRealPublication(t *testing.T) {
	s := newSuite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := identity.Credentials{Bearer: s.adminToken}
	b := s.uploadPNG(t, c, "gc-real-merge-upload")
	page := s.createPage(t, c, "public", content.Snapshot{Type: "media", Title: "Publication", Tags: []string{}, Description: ptr(""), BlobID: &b.ID}, "gc-real-merge-page")
	s.ageBlob(t, b.ID)
	s.review(t, c, page.Proposal, nil, "approve", "gc-real-merge-approve", 201)
	entered, release := make(chan struct{}), make(chan struct{})
	publisher := proposals.New(heldPublication{s.db.store, entered, release}, s.secrets, textadapter.Parser{}, textadapter.Differ{}, s.files)
	merged, collected := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := publisher.Merge(ctx, requestKey(c, "gc-real-publication"), page.Proposal.ID, expected(page.Proposal, nil))
		merged <- err
	}()
	awaitBarrier(t, entered)
	go func() { _, err := s.media.Collect(ctx, 7*24*time.Hour); collected <- err }()
	waitForAdvisoryWaiter(t, s.db.conn)
	close(release)
	if err := <-merged; err != nil {
		t.Fatal(err)
	}
	if err := <-collected; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.mediaRoot, "objects", b.ID)); err != nil {
		t.Fatal("published original collected", err)
	}
}

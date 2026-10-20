//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
)

type heldStage struct {
	media.BlobStore
	staged  chan media.Staged
	release chan struct{}
}

func (h heldStage) Stage(ctx context.Context, id string, r io.Reader, n int64) (media.Staged, error) {
	stage, err := h.BlobStore.Stage(ctx, id, r, n)
	if err != nil {
		return stage, err
	}
	h.staged <- stage
	select {
	case <-h.release:
		return stage, nil
	case <-ctx.Done():
		return stage, ctx.Err()
	}
}
func awaitStage(t *testing.T, ch <-chan media.Staged) media.Staged {
	t.Helper()
	select {
	case stage := <-ch:
		return stage
	case <-time.After(10 * time.Second):
		t.Fatal("stream barrier timeout")
	}
	return media.Staged{}
}
func TestMEDIA03ActiveUploadProtectedAndCompletionReauthorizes(t *testing.T) {
	s := newSuite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	user, token := s.register(t, "streaming_account")
	c := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	u := s.prepareUpload(t, c, "image/png", len(pngBody), "streaming-upload-key")
	staged, release := make(chan media.Staged, 1), make(chan struct{})
	svc := media.New(s.db.store, heldStage{s.files, staged, release}, s.secrets, 104857600)
	done := make(chan error, 1)
	go func() {
		_, err := svc.Put(ctx, c, u.ID, "application/octet-stream", "stream-in-progress", "test-upload-ip", bytes.NewReader(pngBody))
		done <- err
	}()
	stage := awaitStage(t, staged)
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(s.mediaRoot, filepath.FromSlash(stage.Key)), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.media.Collect(ctx, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.mediaRoot, filepath.FromSlash(stage.Key))); err != nil {
		t.Fatal("active staging removed", err)
	}
	if _, err := s.media.Put(ctx, c, u.ID, "application/octet-stream", "parallel-put-request", "test-upload-ip", bytes.NewReader(pngBody)); !errors.Is(err, fault.State) {
		t.Fatal("parallel writer acquired lease", err)
	}
	// Governance and status reads proceed while the stream is paused.
	if _, err := s.media.Get(ctx, c, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.access.UpdateAccount(ctx, requestKey(admin, "disable-stream-account"), user.ID, "member", "disabled", 1); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, fault.Unauthenticated) {
		t.Fatal("disabled account completed upload", err)
	}
	var count int
	if err := s.db.conn.QueryRow(ctx, "SELECT count(*) FROM wikit.blobs").Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked upload became ready", err)
	}
	if _, err := os.Stat(filepath.Join(s.mediaRoot, filepath.FromSlash(stage.Key))); !os.IsNotExist(err) {
		t.Fatal("failed staging not removed")
	}
}
func TestExpiredLeaseFencesOldWriterAndExpiredReservationRejects(t *testing.T) {
	s := newSuite(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := identity.Credentials{Bearer: s.adminToken}
	u := s.prepareUpload(t, c, "image/png", len(pngBody), "lease-recovery-upload")
	staged, release := make(chan media.Staged, 1), make(chan struct{})
	svc := media.New(s.db.store, heldStage{s.files, staged, release}, s.secrets, 104857600)
	done := make(chan error, 1)
	go func() {
		_, err := svc.Put(ctx, c, u.ID, "application/octet-stream", "expired-lease-writer", "test-upload-ip", bytes.NewReader(pngBody))
		done <- err
	}()
	awaitStage(t, staged)
	if _, err := s.db.conn.Exec(ctx, "UPDATE wikit.uploads SET lease_until=now()-interval '1 second' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	newBody := append([]byte{}, pngBody...)
	newBody[len(newBody)-1] = '!'
	newBlob, err := s.media.Put(ctx, c, u.ID, "application/octet-stream", "recovered-lease-writer", "test-upload-ip", bytes.NewReader(newBody))
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, fault.State) {
		t.Fatal("expired writer replaced successor", err)
	}
	state, err := s.media.Get(ctx, c, u.ID)
	if err != nil || state.State != "ready" || state.BlobID == nil || *state.BlobID != newBlob.ID {
		t.Fatal("old cleanup overwrote ready state", err)
	}
	u = s.prepareUpload(t, c, "image/png", len(pngBody), "expired-upload-key")
	if _, err := s.db.conn.Exec(ctx, "UPDATE wikit.uploads SET expires_at=now()-interval '1 second' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	s.binary(t, "PUT", u.Path, pngBody, c, nil, 409)
}

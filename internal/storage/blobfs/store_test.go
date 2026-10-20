package blobfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

const testAttempt = "00000000-0000-4000-8000-000000000001"
const testBlob = "00000000-0000-4000-8000-000000000002"

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("source unavailable") }
func TestImmutableCommitLimitsAndFailure(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	body := []byte("\x89PNG\r\n\x1a\nbytes")
	stage, err := s.Stage(ctx, testAttempt, bytes.NewReader(body), int64(len(body)))
	if err != nil || stage.Mime != "image/png" {
		t.Fatal(stage, err)
	}
	key, err := s.Commit(ctx, stage.Key, testBlob)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx, content.Blob{Key: key, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	stage, err = s.Stage(ctx, testAttempt, bytes.NewReader([]byte("different")), 9)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, stage.Key, testBlob); err == nil {
		t.Fatal("existing original overwritten")
	}
	f, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(f)
	closeErr := f.Close()
	if err != nil || closeErr != nil || !bytes.Equal(raw, body) {
		t.Fatal("original changed")
	}
	if err := s.Remove(ctx, stage.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(ctx, testAttempt, bytes.NewReader(body), 3); !errors.Is(err, fault.UploadTooLarge) {
		t.Fatal("oversized stream", err)
	}
	if _, err := s.Stage(ctx, testAttempt, brokenReader{}, 10); !errors.Is(err, fault.StorageUnavailable) {
		t.Fatal("read failure accepted", err)
	}
	if _, err := os.Stat(filepath.Join(root, "staging", testAttempt)); !os.IsNotExist(err) {
		t.Fatal("failed staging leaked", err)
	}
	for _, key := range []string{"../escape", "objects/../../escape", "objects/CON", "objects/" + testBlob + "/extra"} {
		if err := s.Remove(ctx, key); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
	if err := s.Remove(ctx, "objects/"+testBlob); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(ctx, content.Blob{Key: key, Size: int64(len(body))}); !errors.Is(err, fault.StorageUnavailable) {
		t.Fatal("missing file accepted", err)
	}
}
func TestSupportedFileSignatures(t *testing.T) {
	for _, tc := range []struct {
		mime string
		body []byte
	}{
		{"image/jpeg", []byte{255, 216, 255, 224}},
		{"image/png", []byte("\x89PNG\r\n\x1a\n")},
		{"image/gif", []byte("GIF89a")},
		{"image/webp", []byte("RIFF0000WEBP")},
		{"audio/wav", []byte("RIFF0000WAVE")},
		{"audio/mpeg", []byte("ID3\x04\x00\x00\x00\x00\x00\x00")},
		{"audio/mpeg", []byte{255, 251, 144, 0}},
		{"audio/ogg", []byte("OggS\x00")},
		{"video/mp4", []byte("\x00\x00\x00\x18ftypisom0000")},
		{"video/webm", []byte("\x1a\x45\xdf\xa3\x42\x82\x84webm")},
		{"application/pdf", []byte("%PDF-1.7\n")},
	} {
		if got := sniff(tc.body); got != tc.mime {
			t.Fatalf("%s: %s", tc.mime, got)
		}
	}
	for _, body := range []string{"<svg></svg>", "<!doctype html>", "MZ executable", "%PDF-junk", "ID3"} {
		if sniff([]byte(body)) != "" {
			t.Fatal("unsupported file recognized")
		}
	}
}

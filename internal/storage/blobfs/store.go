package blobfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
)

type Store struct {
	root      *os.Root
	directory string
}

func New(directory string) (*Store, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fault.StorageUnavailable
	}
	directory = absolute
	r, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fault.StorageUnavailable
	}
	for _, name := range []string{"objects", "staging"} {
		if err := r.MkdirAll(name, 0700); err != nil {
			return nil, errors.Join(fault.StorageUnavailable, r.Close())
		}
		dir, err := r.OpenRoot(name)
		if err != nil {
			return nil, errors.Join(fault.StorageUnavailable, r.Close())
		}
		if err := dir.Close(); err != nil {
			return nil, errors.Join(fault.StorageUnavailable, r.Close())
		}
	}
	s := &Store{r, directory}
	if err := s.initialize(); err != nil {
		return nil, errors.Join(fault.StorageUnavailable, r.Close())
	}
	return s, nil
}
func (s *Store) Close() error { return s.root.Close() }
func (s *Store) Check(ctx context.Context, b content.Blob) error {
	f, err := s.Open(ctx, b.Key)
	if err != nil {
		return err
	}
	n, seekErr := f.Seek(0, io.SeekEnd)
	closeErr := f.Close()
	if seekErr != nil || closeErr != nil || n != b.Size {
		return fault.StorageUnavailable
	}
	return nil
}
func validKey(key string) bool {
	p := strings.Split(key, "/")
	return len(p) == 2 && (p[0] == "objects" || p[0] == "staging") && identity.ValidID(p[1]) && p[1] == strings.ToLower(p[1])
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
func (s *Store) Stage(ctx context.Context, attempt string, source io.Reader, limit int64) (out media.Staged, result error) {
	key := "staging/" + attempt
	if !validKey(key) || limit < 1 || limit == int64(^uint64(0)>>1) {
		return out, fault.Invalid
	}
	f, err := s.root.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, fault.StorageUnavailable
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, s.Remove(context.Background(), key))
		}
	}()
	hash := sha256.New()
	header := &prefixWriter{}
	n, copyErr := io.Copy(io.MultiWriter(f, hash, header), io.LimitReader(contextReader{ctx, source}, limit+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || ctx.Err() != nil {
		return out, fault.StorageUnavailable
	}
	if n > limit {
		return out, fault.UploadTooLarge
	}
	return media.Staged{Key: key, Size: n, Digest: hex.EncodeToString(hash.Sum(nil)), Mime: sniff(header.data)}, nil
}

type prefixWriter struct{ data []byte }

func (p *prefixWriter) Write(b []byte) (int, error) {
	n := len(b)
	if len(p.data) < 512 {
		p.data = append(p.data, b[:min(n, 512-len(p.data))]...)
	}
	return n, nil
}
func (s *Store) Commit(ctx context.Context, stage, id string) (string, error) {
	key := "objects/" + id
	if !validKey(stage) || !strings.HasPrefix(stage, "staging/") || !validKey(key) {
		return "", fault.Invalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.commit(stage, key); err != nil {
		return "", fault.StorageUnavailable
	}
	return key, nil
}
func (s *Store) Open(ctx context.Context, key string) (media.Reader, error) {
	if !validKey(key) || !strings.HasPrefix(key, "objects/") {
		return nil, fault.StorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, fault.StorageUnavailable
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.Join(fault.StorageUnavailable, f.Close())
	}
	return f, nil
}
func (s *Store) Remove(ctx context.Context, key string) error {
	if !validKey(key) {
		return fault.StorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.root.Remove(key)
	if err != nil && !os.IsNotExist(err) {
		return fault.StorageUnavailable
	}
	return nil
}
func (s *Store) Orphans(ctx context.Context, cutoff time.Time) ([]string, error) {
	out := []string{}
	for _, dir := range []string{"objects", "staging"} {
		f, err := s.root.Open(dir)
		if err != nil {
			return nil, fault.StorageUnavailable
		}
		rows, readErr := f.ReadDir(-1)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			return nil, fault.StorageUnavailable
		}
		for _, entry := range rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			key := dir + "/" + entry.Name()
			if !validKey(key) || !entry.Type().IsRegular() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, fault.StorageUnavailable
			}
			if info.ModTime().Before(cutoff) {
				out = append(out, key)
			}
		}
	}
	return out, nil
}

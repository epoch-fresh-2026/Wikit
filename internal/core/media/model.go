// Package media owns uploads, authorized original reads and garbage collection.
package media

import (
	"context"
	"io"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
)

type Upload struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Path      string    `json:"upload_path"`
	ExpiresAt time.Time `json:"expires_at"`
	BlobID    *string   `json:"blob_id"`
}
type Reservation struct {
	Upload
	Owner, Filename, Mime string
	Size                  int64
	Attempt, AttemptBlob  string
	LeaseUntil            time.Time
}
type Blob struct {
	ID     string `json:"id"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
	Digest string `json:"sha256"`
}
type Stored struct {
	Blob
	Key, State, Uploader string
	CreatedAt            time.Time
}
type Staged struct {
	Key, Digest, Mime string
	Size              int64
}
type Reader interface {
	io.Reader
	io.Seeker
	io.Closer
}
type BlobStore interface {
	Stage(context.Context, string, io.Reader, int64) (Staged, error)
	Commit(context.Context, string, string) (string, error)
	Open(context.Context, string) (Reader, error)
	Remove(context.Context, string) error
	Orphans(context.Context, time.Time) ([]string, error)
}
type Tx interface {
	access.Authorization
	Revision(context.Context, string, string) (content.Published, error)
	Proposal(context.Context, string) (proposals.Proposal, error)
	Candidate(context.Context, string, string) (proposals.Revision, error)
	Upload(context.Context, string) (Reservation, error)
	SaveUpload(context.Context, Reservation, bool) error
	StoredBlob(context.Context, string) (Stored, error)
	InsertBlob(context.Context, Stored) error
	ClaimGarbage(context.Context, time.Time, int) ([]Stored, error)
	FinishDelete(context.Context, string) error
	FileLive(context.Context, string) (bool, error)
	Audit(context.Context, identity.Actor, string, string, string) error
	Replay(context.Context, string, string, string, string) ([]byte, error)
	Remember(context.Context, string, string, string, string, []byte) error
}
type Repository interface {
	WithinMedia(context.Context, func(Tx) error) error
	Allow(context.Context, string, string) error
}
type Service struct {
	repo    Repository
	files   BlobStore
	secrets identity.Secrets
	max     int64
}

func New(repo Repository, files BlobStore, secrets identity.Secrets, max int64) *Service {
	return &Service{repo, files, secrets, max}
}

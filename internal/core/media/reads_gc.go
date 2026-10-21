package media

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Download struct {
	Reader    Reader
	Blob      Blob
	CreatedAt time.Time
}

func (s *Service) open(ctx context.Context, b Stored) (Reader, error) {
	reader, err := s.files.Open(ctx, b.Key)
	if err != nil {
		return nil, err
	}
	size, err := reader.Seek(0, io.SeekEnd)
	if err != nil || size != b.Size {
		return nil, errors.Join(fault.StorageUnavailable, reader.Close())
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Join(fault.StorageUnavailable, reader.Close())
	}
	return reader, nil
}

func (s *Service) Read(ctx context.Context, c identity.Credentials, kind, id, revision string) (Download, error) {
	id, revision = strings.ToLower(id), strings.ToLower(revision)
	if (kind == "published" && !access.ValidPageID(id)) || (kind != "published" && !identity.ValidID(id)) || !identity.ValidID(revision) {
		return Download{}, fault.NotFound
	}
	var out Download
	err := s.repo.WithinMedia(ctx, func(tx Tx) error {
		page := id
		if kind == "proposal" {
			p, err := tx.Proposal(ctx, id)
			if err != nil {
				return err
			}
			page = p.PageID
		} else if kind != "published" {
			return fault.BadRequest
		}
		if _, _, err := access.Authorize(ctx, tx, c, page, "read", false); err != nil {
			return err
		}
		var blob *string
		if kind == "proposal" {
			v, err := tx.Candidate(ctx, id, revision)
			if err != nil {
				return err
			}
			blob = v.Snapshot.BlobID
		} else {
			v, err := tx.Revision(ctx, id, revision)
			if err != nil {
				return err
			}
			blob = v.Snapshot.BlobID
		}
		if blob == nil {
			return fault.Invalid
		}
		b, err := tx.StoredBlob(ctx, *blob)
		if err != nil {
			return err
		}
		if b.State != "ready" {
			return fault.StorageUnavailable
		}
		reader, err := s.open(ctx, b)
		if err != nil {
			return err
		}
		out = Download{Reader: reader, Blob: b.Blob, CreatedAt: b.CreatedAt}
		return nil
	})
	if err != nil && out.Reader != nil {
		err = errors.Join(err, out.Reader.Close())
		out = Download{}
	}
	return out, err
}

type Collection struct{ Blobs, Orphans int }

func (s *Service) Collect(ctx context.Context, grace time.Duration) (Collection, error) {
	var out Collection
	if grace <= 0 {
		return out, fault.Invalid
	}
	cutoff := time.Now().UTC().Add(-grace)
	runID, err := s.secrets.ID()
	if err != nil {
		return out, err
	}
	system := identity.Actor{Method: "local"}
	for {
		var rows []Stored
		err := s.repo.WithinMedia(ctx, func(tx Tx) error {
			var err error
			rows, err = tx.ClaimGarbage(ctx, cutoff, 100)
			if err != nil {
				return err
			}
			for _, b := range rows {
				if err := tx.Audit(ctx, system, "blob.gc.claim", b.ID, runID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		for _, b := range rows {
			if err := s.files.Remove(ctx, b.Key); err != nil {
				return out, err
			}
			if err := s.repo.WithinMedia(ctx, func(tx Tx) error {
				if err := tx.FinishDelete(ctx, b.ID); err != nil {
					return err
				}
				return tx.Audit(ctx, system, "blob.gc.delete", b.ID, runID)
			}); err != nil {
				return out, err
			}
			out.Blobs++
		}
		if len(rows) < 100 {
			break
		}
	}
	keys, err := s.files.Orphans(ctx, cutoff)
	if err != nil {
		return out, err
	}
	for _, key := range keys {
		err := s.repo.WithinMedia(ctx, func(tx Tx) error {
			live, err := tx.FileLive(ctx, key)
			if err != nil {
				return err
			}
			if live {
				return nil
			}
			if err := s.files.Remove(ctx, key); err != nil {
				return err
			}
			out.Orphans++
			return nil
		})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

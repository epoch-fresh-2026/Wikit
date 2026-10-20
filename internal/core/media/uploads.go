package media

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

func allowed(mime string) bool {
	return slices.Contains([]string{"image/jpeg", "image/png", "image/gif", "image/webp", "audio/mpeg", "audio/wav", "audio/ogg", "video/mp4", "video/webm", "application/pdf"}, mime)
}
func owned(ctx context.Context, tx Tx, c identity.Credentials, id string) (identity.Actor, Reservation, error) {
	a, err := identity.Require(ctx, tx, c)
	if err != nil {
		return a, Reservation{}, err
	}
	u, err := tx.Upload(ctx, id)
	if err != nil {
		return a, u, err
	}
	if a.Role != "admin" && u.Owner != a.ID {
		return a, u, fault.NotFound
	}
	return a, u, nil
}
func (s *Service) Prepare(ctx context.Context, r access.Request, filename, mime string, size int64, ip string) (Upload, error) {
	if !utf8.ValidString(filename) || strings.TrimSpace(filename) == "" || strings.ContainsAny(filename, "\x00\r\n/\\") || utf8.RuneCountInString(filename) > 255 || size < 1 {
		return Upload{}, fault.Invalid
	}
	if size > s.max {
		return Upload{}, fault.UploadTooLarge
	}
	if !allowed(mime) {
		return Upload{}, fault.UnsupportedMedia
	}
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return Upload{}, fault.BadRequest
	}
	r.Credentials.Write = true
	var actorID string
	if err := s.repo.WithinMedia(ctx, func(tx Tx) error { a, err := identity.Require(ctx, tx, r.Credentials); actorID = a.ID; return err }); err != nil {
		return Upload{}, err
	}
	if err := s.repo.Allow(ctx, "upload.prepare", ip+"\x00"+actorID); err != nil {
		return Upload{}, err
	}
	var out Upload
	err := s.repo.WithinMedia(ctx, func(tx Tx) error {
		a, err := identity.Require(ctx, tx, r.Credentials)
		if err != nil {
			return err
		}
		raw, err := tx.Replay(ctx, a.ID, "upload.prepare", r.Key, r.Hash)
		if err != nil {
			return err
		}
		if raw != nil {
			return json.Unmarshal(raw, &out)
		}
		id, err := s.secrets.ID()
		if err != nil {
			return err
		}
		out = Upload{ID: id, State: "pending", Path: "/api/v1/uploads/" + id + "/content", ExpiresAt: time.Now().UTC().Add(time.Hour)}
		if err := tx.SaveUpload(ctx, Reservation{Upload: out, Owner: a.ID, Filename: filename, Mime: mime, Size: size}, true); err != nil {
			return err
		}
		if err := tx.Audit(ctx, a, "upload.prepare", id, r.ID); err != nil {
			return err
		}
		raw, err = json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Remember(ctx, a.ID, "upload.prepare", r.Key, r.Hash, raw)
	})
	return out, err
}
func (s *Service) Get(ctx context.Context, c identity.Credentials, id string) (Upload, error) {
	id = strings.ToLower(id)
	if !identity.ValidID(id) {
		return Upload{}, fault.NotFound
	}
	var out Upload
	err := s.repo.WithinMedia(ctx, func(tx Tx) error {
		_, u, err := owned(ctx, tx, c, id)
		if err != nil {
			return err
		}
		out = u.Upload
		if u.BlobID != nil {
			b, err := tx.StoredBlob(ctx, *u.BlobID)
			if err != nil {
				return err
			}
			if b.State != "ready" {
				out.State = "failed"
				out.BlobID = nil
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) Put(ctx context.Context, c identity.Credentials, id, mime, requestID, ip string, body io.Reader) (out Blob, result error) {
	id = strings.ToLower(id)
	if !identity.ValidID(id) {
		return out, fault.NotFound
	}
	c.Write = true
	var actorID string
	if err := s.repo.WithinMedia(ctx, func(tx Tx) error { a, _, err := owned(ctx, tx, c, id); actorID = a.ID; return err }); err != nil {
		return out, err
	}
	if err := s.repo.Allow(ctx, "upload.write", ip+"\x00"+actorID); err != nil {
		return out, err
	}
	var claimed Reservation
	err := s.repo.WithinMedia(ctx, func(tx Tx) error {
		_, u, err := owned(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if !time.Now().Before(u.ExpiresAt) {
			return fault.State
		}
		if u.Size > s.max {
			return fault.UploadTooLarge
		}
		if mime != "application/octet-stream" && mime != u.Mime {
			return fault.UnsupportedMedia
		}
		if u.Attempt != "" && time.Now().Before(u.LeaseUntil) {
			return fault.State
		}
		if u.BlobID != nil {
			b, err := tx.StoredBlob(ctx, *u.BlobID)
			if err != nil {
				return err
			}
			if b.State != "ready" {
				return fault.State
			}
		}
		u.Attempt, err = s.secrets.ID()
		if err != nil {
			return err
		}
		u.AttemptBlob, err = s.secrets.ID()
		if err != nil {
			return err
		}
		u.LeaseUntil = time.Now().UTC().Add(30 * time.Minute)
		claimed = u
		return tx.SaveUpload(ctx, u, false)
	})
	if err != nil {
		return out, err
	}
	// Cleanup only this attempt; never overwrite a successor's lease or ready result.
	defer func() {
		if result != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := s.repo.WithinMedia(cleanup, func(tx Tx) error {
				u, err := tx.Upload(cleanup, id)
				if err != nil {
					return err
				}
				if u.Attempt != claimed.Attempt {
					return nil
				}
				u.Attempt = ""
				u.AttemptBlob = ""
				u.LeaseUntil = time.Time{}
				if u.BlobID == nil {
					u.State = "failed"
				}
				return tx.SaveUpload(cleanup, u, false)
			})
			result = errors.Join(result, err)
		}
	}()
	stage, err := s.files.Stage(ctx, claimed.Attempt, body, claimed.Size)
	if err != nil {
		return out, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, s.files.Remove(cleanup, stage.Key))
	}()
	if stage.Size != claimed.Size {
		return out, fault.Invalid
	}
	if stage.Mime != claimed.Mime || !allowed(stage.Mime) {
		return out, fault.UnsupportedMedia
	}
	err = s.repo.WithinMedia(ctx, func(tx Tx) error {
		a, u, err := owned(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if u.Attempt != claimed.Attempt || !time.Now().Before(u.LeaseUntil) || !time.Now().Before(u.ExpiresAt) {
			return fault.State
		}
		if u.BlobID != nil {
			b, err := tx.StoredBlob(ctx, *u.BlobID)
			if err != nil {
				return err
			}
			if b.State != "ready" {
				return fault.State
			}
			if stage.Digest != b.Digest || stage.Size != b.Size {
				return fault.State
			}
			reader, err := s.open(ctx, b)
			if err != nil {
				return err
			}
			if err := reader.Close(); err != nil {
				return fault.StorageUnavailable
			}
			out = b.Blob
		} else {
			key, err := s.files.Commit(ctx, stage.Key, u.AttemptBlob)
			if err != nil {
				return err
			}
			b := Stored{Blob: Blob{ID: u.AttemptBlob, Size: stage.Size, Mime: stage.Mime, Digest: stage.Digest}, Key: key, State: "ready", Uploader: u.Owner, CreatedAt: time.Now().UTC()}
			if err := tx.InsertBlob(ctx, b); err != nil {
				return err
			}
			out = b.Blob
			u.BlobID = &out.ID
			u.State = "ready"
			if err := tx.Audit(ctx, a, "upload.complete", id, requestID); err != nil {
				return err
			}
		}
		u.Attempt = ""
		u.AttemptBlob = ""
		u.LeaseUntil = time.Time{}
		return tx.SaveUpload(ctx, u, false)
	})
	return out, err
}

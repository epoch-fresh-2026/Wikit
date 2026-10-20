package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
)

func (s *Store) WithinMedia(ctx context.Context, fn func(media.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}
func (t *securityTx) Upload(ctx context.Context, id string) (media.Reservation, error) {
	var u media.Reservation
	var attempt, blob *string
	var lease *time.Time
	err := t.QueryRow(ctx, `SELECT id::text,owner_id::text,filename,mime,size,state,expires_at,blob_id::text,attempt_id::text,attempt_blob_id::text,lease_until FROM wikit.uploads WHERE id=$1`, id).Scan(&u.ID, &u.Owner, &u.Filename, &u.Mime, &u.Size, &u.State, &u.ExpiresAt, &u.BlobID, &attempt, &blob, &lease)
	if attempt != nil {
		u.Attempt = *attempt
	}
	if blob != nil {
		u.AttemptBlob = *blob
	}
	if lease != nil {
		u.LeaseUntil = *lease
	}
	u.Path = "/api/v1/uploads/" + u.ID + "/content"
	return u, storageError(err)
}
func (t *securityTx) SaveUpload(ctx context.Context, u media.Reservation, create bool) error {
	if create {
		_, err := t.Exec(ctx, `INSERT INTO wikit.uploads(id,owner_id,filename,mime,size,state,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, u.ID, u.Owner, u.Filename, u.Mime, u.Size, u.State, u.ExpiresAt)
		return storageError(err)
	}
	var lease *time.Time
	if u.Attempt != "" {
		lease = &u.LeaseUntil
	}
	_, err := t.Exec(ctx, `UPDATE wikit.uploads SET state=$2,blob_id=$3,attempt_id=nullif($4,'')::uuid,attempt_blob_id=nullif($5,'')::uuid,lease_until=$6 WHERE id=$1`, u.ID, u.State, u.BlobID, u.Attempt, u.AttemptBlob, lease)
	return storageError(err)
}

const blobColumns = "id::text,size,mime,digest,storage_key,state,uploader::text,created_at"

func (t *securityTx) StoredBlob(ctx context.Context, id string) (media.Stored, error) {
	var b media.Stored
	err := t.QueryRow(ctx, "SELECT "+blobColumns+" FROM wikit.blobs WHERE id=$1", id).Scan(&b.ID, &b.Size, &b.Mime, &b.Digest, &b.Key, &b.State, &b.Uploader, &b.CreatedAt)
	return b, storageError(err)
}
func (t *securityTx) InsertBlob(ctx context.Context, b media.Stored) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.blobs(id,size,mime,digest,storage_key,state,uploader,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, b.ID, b.Size, b.Mime, b.Digest, b.Key, b.State, b.Uploader, b.CreatedAt)
	return storageError(err)
}
func (t *securityTx) ClaimGarbage(ctx context.Context, cutoff time.Time, limit int) ([]media.Stored, error) {
	rows, err := t.Query(ctx, `SELECT `+blobColumns+` FROM wikit.blobs b WHERE
 (state='deleting' OR (state IN ('pending','ready') AND created_at<$1))
 AND NOT EXISTS(SELECT 1 FROM wikit.revisions r WHERE r.blob_id=b.id)
 AND NOT EXISTS(SELECT 1 FROM wikit.proposal_revisions r WHERE r.blob_id=b.id)
 AND NOT EXISTS(SELECT 1 FROM wikit.uploads u WHERE u.attempt_blob_id=b.id AND u.lease_until>now())
 ORDER BY created_at,id LIMIT $2 FOR UPDATE`, cutoff, limit)
	if err != nil {
		return nil, storageError(err)
	}
	out := []media.Stored{}
	for rows.Next() {
		var b media.Stored
		if err := rows.Scan(&b.ID, &b.Size, &b.Mime, &b.Digest, &b.Key, &b.State, &b.Uploader, &b.CreatedAt); err != nil {
			rows.Close()
			return nil, storageError(err)
		}
		out = append(out, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, storageError(err)
	}
	for _, b := range out {
		if _, err := t.Exec(ctx, "UPDATE wikit.blobs SET state='deleting' WHERE id=$1", b.ID); err != nil {
			return nil, storageError(err)
		}
	}
	return out, nil
}
func (t *securityTx) FinishDelete(ctx context.Context, id string) error {
	_, err := t.Exec(ctx, "UPDATE wikit.blobs SET state='deleted' WHERE id=$1 AND state='deleting'", id)
	return storageError(err)
}
func (t *securityTx) FileLive(ctx context.Context, key string) (bool, error) {
	var live bool
	var err error
	if strings.HasPrefix(key, "staging/") {
		err = t.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wikit.uploads WHERE attempt_id=nullif($1,'')::uuid AND lease_until>now())", strings.TrimPrefix(key, "staging/")).Scan(&live)
	} else {
		err = t.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wikit.blobs WHERE storage_key=$1 AND state<>'deleted') OR EXISTS(SELECT 1 FROM wikit.uploads WHERE attempt_blob_id=nullif($2,'')::uuid AND lease_until>now())`, key, strings.TrimPrefix(key, "objects/")).Scan(&live)
	}
	return live, storageError(err)
}

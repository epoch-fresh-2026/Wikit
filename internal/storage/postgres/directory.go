package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/jackc/pgx/v5"
)

func (t *securityTx) PageBatch(ctx context.Context, filter content.TagFilter, kind, after string, limit int) ([]pages.Page, error) {
	rows, err := t.Query(ctx, "SELECT "+pageColumns+` FROM wikit.pages p JOIN wikit.page_policies a ON a.page_id=p.id
 WHERE p.id>$3::bigint AND a.status='published' AND p.head_revision_id IS NOT NULL
 AND (cardinality($1::text[])=0 OR
 ($5='all' AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) wanted(tag) WHERE NOT EXISTS(SELECT 1 FROM wikit.revision_tags rt WHERE rt.revision_id=p.head_revision_id AND rt.tag_key=wanted.tag))) OR
 ($5='any' AND EXISTS(SELECT 1 FROM wikit.revision_tags rt WHERE rt.revision_id=p.head_revision_id AND rt.tag_key=ANY($1::text[]))))
 AND NOT EXISTS(SELECT 1 FROM wikit.revision_tags rt WHERE rt.revision_id=p.head_revision_id AND rt.tag_key=ANY($6::text[]))
 AND ($2='' OR p.type=$2) ORDER BY p.id LIMIT $4`, filter.Tags, kind, after, limit, filter.Match, filter.ExcludeTags)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []pages.Page{}
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, storageError(rows.Err())
}

func (t *securityTx) PageDetails(ctx context.Context, ids []string) ([]pages.Detail, error) {
	if len(ids) == 0 {
		return []pages.Detail{}, nil
	}
	rows, err := t.Query(ctx, "SELECT "+pageColumns+`,r.id::text,r.page_id::text,r.parent_id::text,r.source_proposal_id::text,r.snapshot,r.created_at,r.author_id::text,r.source_kind
 FROM wikit.pages p JOIN wikit.page_policies a ON a.page_id=p.id
 JOIN wikit.revisions r ON r.id=p.head_revision_id AND r.page_id=p.id AND r.sealed
 WHERE p.id=ANY($1::bigint[])`, ids)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []pages.Detail{}
	for rows.Next() {
		var d pages.Detail
		var v pages.Revision
		var raw []byte
		p := &d.Page
		if err := rows.Scan(&p.ID, &p.Type, &p.Visibility, &p.Status, &p.HeadRevisionID, &p.PolicyVersion, &v.ID, &v.PageID, &v.ParentID, &v.SourceProposalID, &raw, &v.CreatedAt, &v.AuthorID, &v.SourceKind); err != nil {
			return nil, storageError(err)
		}
		if err := json.Unmarshal(raw, &v.Snapshot); err != nil {
			return nil, storageError(err)
		}
		if v.Snapshot.Type == "media" {
			url := "/api/v1/pages/" + p.ID + "/revisions/" + v.ID + "/content"
			v.ContentURL = &url
		}
		d.Revision = &v
		out = append(out, d)
	}
	return out, storageError(rows.Err())
}

func (t *securityTx) PageAnchor(ctx context.Context, plain, actor, kind string, limit int) (string, error) {
	var anchor string
	err := t.QueryRow(ctx, `SELECT page_anchor::text FROM wikit.list_cursors WHERE digest=$1 AND actor_id IS NOT DISTINCT FROM nullif($2,'')::uuid AND kind=$3 AND page_size=$4 AND expires_at>now() AND page_anchor IS NOT NULL`, identity.Digest(plain), actor, kind, limit).Scan(&anchor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fault.BadRequest
	}
	return anchor, storageError(err)
}
func (t *securityTx) NewPageCursor(ctx context.Context, plain, actor, kind, anchor string, limit int) (string, error) {
	_, err := t.Exec(ctx, `INSERT INTO wikit.list_cursors(digest,actor_id,kind,page_size,position,expires_at,page_anchor) VALUES($1,nullif($2,'')::uuid,$3,$4,0,$5,$6)`, identity.Digest(plain), actor, kind, limit, time.Now().Add(10*time.Minute), anchor)
	return plain, storageError(err)
}

package postgres

import (
	"context"
	"encoding/json"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/jackc/pgx/v5"
)

func (s *Store) WithinPages(ctx context.Context, fn func(pages.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}
func (t *securityTx) InsertPage(ctx context.Context, p pages.Page) error {
	_, err := t.Exec(ctx, "INSERT INTO wikit.pages(id,type) VALUES($1,$2)", p.ID, p.Type)
	return storageError(err)
}
func (t *securityTx) InsertInitial(ctx context.Context, p proposals.Proposal, refs []content.MediaRef) error {
	if _, err := t.Exec(ctx, "INSERT INTO wikit.proposals VALUES($1,$2,$3,$4,$5,$6)", p.ID, p.PageID, p.AuthorID, p.State, p.Version, p.CurrentRevision.ID); err != nil {
		return storageError(err)
	}
	return t.InsertCandidate(ctx, p, refs)
}
func (t *securityTx) InsertCandidate(ctx context.Context, p proposals.Proposal, refs []content.MediaRef) error {
	raw, err := json.Marshal(p.CurrentRevision.Snapshot)
	if err != nil {
		return err
	}
	if _, err := t.Exec(ctx, `INSERT INTO wikit.proposal_revisions(id,proposal_id,page_id,type,base_revision_id,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.CurrentRevision.ID, p.ID, p.PageID, p.CurrentRevision.Snapshot.Type, p.CurrentRevision.BaseRevisionID, raw, p.CurrentRevision.CreatedAt); err != nil {
		return storageError(err)
	}
	return t.associations(ctx, "proposal_revision", p.CurrentRevision.ID, p.CurrentRevision.Snapshot, refs)
}

const pageColumns = "p.id::text,p.type,a.visibility,a.status,p.head_revision_id::text,a.policy_version"

func scanPage(row pgx.Row) (pages.Page, error) {
	var p pages.Page
	err := row.Scan(&p.ID, &p.Type, &p.Visibility, &p.Status, &p.HeadRevisionID, &p.PolicyVersion)
	return p, storageError(err)
}
func (t *securityTx) Page(ctx context.Context, id string) (pages.Page, error) {
	return scanPage(t.QueryRow(ctx, "SELECT "+pageColumns+" FROM wikit.pages p JOIN wikit.page_policies a ON a.page_id=p.id WHERE p.id=$1", id))
}
func (t *securityTx) PageCandidates(ctx context.Context, tag, kind string) ([]pages.Page, error) {
	rows, err := t.Query(ctx, "SELECT "+pageColumns+` FROM wikit.pages p JOIN wikit.page_policies a ON a.page_id=p.id WHERE ($1='' OR EXISTS(SELECT 1 FROM wikit.revision_tags rt WHERE rt.revision_id=p.head_revision_id AND rt.tag_key=$1)) AND ($2='' OR p.type=$2) ORDER BY p.id`, tag, kind)
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
func (t *securityTx) Revision(ctx context.Context, page, id string) (pages.Revision, error) {
	var v pages.Revision
	var raw []byte
	err := t.QueryRow(ctx, `SELECT id::text,page_id::text,parent_id::text,source_proposal_id::text,snapshot,created_at,author_id::text,source_kind FROM wikit.revisions WHERE page_id=$1 AND id=$2 AND sealed`, page, id).Scan(&v.ID, &v.PageID, &v.ParentID, &v.SourceProposalID, &raw, &v.CreatedAt, &v.AuthorID, &v.SourceKind)
	if err != nil {
		return v, storageError(err)
	}
	if err := json.Unmarshal(raw, &v.Snapshot); err != nil {
		return v, storageError(err)
	}
	if v.Snapshot.Type == "media" {
		url := "/api/v1/pages/" + v.PageID + "/revisions/" + v.ID + "/content"
		v.ContentURL = &url
	}
	return v, nil
}
func (t *securityTx) RevisionIDs(ctx context.Context, page string) ([]string, error) {
	rows, err := t.Query(ctx, "SELECT id::text FROM wikit.revisions WHERE page_id=$1 AND sealed ORDER BY created_at,id", page)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, storageError(err)
		}
		out = append(out, id)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) PublishedTags(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	rows, err := t.Query(ctx, "SELECT DISTINCT tag_key FROM wikit.revision_tags WHERE revision_id=ANY($1::uuid[]) ORDER BY tag_key", ids)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, storageError(err)
		}
		out = append(out, tag)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Proposal(ctx context.Context, id string) (proposals.Proposal, error) {
	var p proposals.Proposal
	var raw []byte
	err := t.QueryRow(ctx, `SELECT p.id::text,p.page_id::text,p.author_id::text,p.state,p.version,r.id::text,r.proposal_id::text,r.base_revision_id::text,r.snapshot,r.created_at FROM wikit.proposals p JOIN wikit.proposal_revisions r ON r.id=p.current_revision_id WHERE p.id=$1 AND r.sealed`, id).Scan(&p.ID, &p.PageID, &p.AuthorID, &p.State, &p.Version, &p.CurrentRevision.ID, &p.CurrentRevision.ProposalID, &p.CurrentRevision.BaseRevisionID, &raw, &p.CurrentRevision.CreatedAt)
	if err != nil {
		return p, storageError(err)
	}
	return p, storageError(json.Unmarshal(raw, &p.CurrentRevision.Snapshot))
}
func (t *securityTx) ProposalCandidates(ctx context.Context, page, state string) ([]proposals.Summary, error) {
	rows, err := t.Query(ctx, "SELECT id::text,page_id::text,state FROM wikit.proposals WHERE ($1='' OR page_id=nullif($1,'')::bigint) AND ($2='' OR state=$2) ORDER BY id", page, state)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []proposals.Summary{}
	for rows.Next() {
		var p proposals.Summary
		if err := rows.Scan(&p.ID, &p.PageID, &p.State); err != nil {
			return nil, storageError(err)
		}
		out = append(out, p)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) CandidateIDs(ctx context.Context, id string) ([]string, error) {
	rows, err := t.Query(ctx, "SELECT id::text FROM wikit.proposal_revisions WHERE proposal_id=$1 AND sealed ORDER BY created_at,id", id)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, storageError(err)
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Candidate(ctx context.Context, proposal, id string) (proposals.Revision, error) {
	var v proposals.Revision
	var raw []byte
	err := t.QueryRow(ctx, "SELECT id::text,proposal_id::text,base_revision_id::text,snapshot,created_at FROM wikit.proposal_revisions WHERE proposal_id=$1 AND id=$2 AND sealed", proposal, id).Scan(&v.ID, &v.ProposalID, &v.BaseRevisionID, &raw, &v.CreatedAt)
	if err != nil {
		return v, storageError(err)
	}
	return v, storageError(json.Unmarshal(raw, &v.Snapshot))
}
func (t *securityTx) Policies(ctx context.Context, ids []string) ([]access.Policy, error) {
	rows, err := t.Query(ctx, `SELECT p.page_id::text,p.visibility,p.status,p.policy_version,
	 coalesce(jsonb_agg(jsonb_build_object('subject_type',g.subject_type,'subject_id',g.subject_id::text,'capabilities',g.caps)) FILTER(WHERE g.subject_id IS NOT NULL),'[]'::jsonb)
	 FROM wikit.page_policies p LEFT JOIN (
	 SELECT page_id,subject_type,subject_id,array_agg(capability ORDER BY capability) caps FROM wikit.page_grants WHERE page_id=ANY($1::bigint[]) GROUP BY page_id,subject_type,subject_id
	 ) g ON g.page_id=p.page_id WHERE p.page_id=ANY($1::bigint[]) GROUP BY p.page_id`, ids)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []access.Policy{}
	for rows.Next() {
		var p access.Policy
		var raw []byte
		if err := rows.Scan(&p.PageID, &p.Visibility, &p.Status, &p.Version, &raw); err != nil {
			return nil, storageError(err)
		}
		if err := json.Unmarshal(raw, &p.Grants); err != nil {
			return nil, storageError(err)
		}
		out = append(out, p)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) Blob(ctx context.Context, id string) (pages.Blob, error) {
	var b pages.Blob
	err := t.QueryRow(ctx, "SELECT id::text,state,uploader::text,storage_key,size FROM wikit.blobs WHERE id=$1", id).Scan(&b.ID, &b.State, &b.Uploader, &b.Key, &b.Size)
	return b, storageError(err)
}

func (t *securityTx) NextPageID(ctx context.Context) (string, error) {
	var id string
	err := t.QueryRow(ctx, "SELECT nextval('wikit.page_ids')::text").Scan(&id)
	return id, storageError(err)
}

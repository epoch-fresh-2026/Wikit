package postgres

import (
	"context"
	"encoding/json"

	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
)

func (s *Store) WithinProposals(ctx context.Context, fn func(proposals.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}
func (t *securityTx) LockPage(ctx context.Context, id string) error {
	var found string
	return storageError(t.QueryRow(ctx, "SELECT id::text FROM wikit.pages WHERE id=$1 FOR UPDATE", id).Scan(&found))
}
func (t *securityTx) LockProposal(ctx context.Context, id string) error {
	var found string
	return storageError(t.QueryRow(ctx, "SELECT id::text FROM wikit.proposals WHERE id=$1 FOR UPDATE", id).Scan(&found))
}
func (t *securityTx) SaveProposal(ctx context.Context, p proposals.Proposal) error {
	_, err := t.Exec(ctx, "UPDATE wikit.proposals SET state=$2,version=$3,review_round=$4,current_revision_id=$5 WHERE id=$1", p.ID, p.State, p.Version, p.ReviewRound, p.CurrentRevision.ID)
	return storageError(err)
}
func (t *securityTx) Reviews(ctx context.Context, id string) ([]proposals.Review, error) {
	rows, err := t.Query(ctx, `SELECT id::text,actor_id::text,proposal_revision_id::text,base_revision_id::text,round,decision,body,created_at FROM wikit.reviews WHERE proposal_id=$1 ORDER BY sequence`, id)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []proposals.Review{}
	for rows.Next() {
		var r proposals.Review
		if err := rows.Scan(&r.ID, &r.ActorID, &r.ProposalRevisionID, &r.BaseRevisionID, &r.Round, &r.Decision, &r.Body, &r.CreatedAt); err != nil {
			return nil, storageError(err)
		}
		out = append(out, r)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) InsertReview(ctx context.Context, id string, r proposals.Review) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.reviews(id,proposal_id,page_id,actor_id,proposal_revision_id,base_revision_id,round,decision,body,created_at)
 SELECT $1,id,page_id,$3,$4,$5,$6,$7,$8,$9 FROM wikit.proposals WHERE id=$2`, r.ID, id, r.ActorID, r.ProposalRevisionID, r.BaseRevisionID, r.Round, r.Decision, r.Body, r.CreatedAt)
	return storageError(err)
}
func (t *securityTx) BlobOnPage(ctx context.Context, page, blob string) (bool, error) {
	var used bool
	err := t.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wikit.revisions WHERE page_id=$1 AND blob_id=$2 AND sealed UNION ALL SELECT 1 FROM wikit.proposal_revisions WHERE page_id=$1 AND blob_id=$2 AND sealed)`, page, blob).Scan(&used)
	return used, storageError(err)
}

// associations is used only with application constants, never request table names.
func (t *securityTx) associations(ctx context.Context, prefix, id string, snapshot content.Snapshot, refs []content.MediaRef) error {
	for _, tag := range snapshot.Tags {
		if _, err := t.Exec(ctx, "INSERT INTO wikit.tags VALUES($1) ON CONFLICT DO NOTHING", tag); err != nil {
			return storageError(err)
		}
		if _, err := t.Exec(ctx, "INSERT INTO wikit."+prefix+"_tags VALUES($1,$2)", id, tag); err != nil {
			return storageError(err)
		}
	}
	for _, ref := range refs {
		if _, err := t.Exec(ctx, "INSERT INTO wikit."+prefix+"_media_refs(revision_id,target_page_id,target_revision_id) VALUES($1,$2,$3)", id, ref.PageID, ref.RevisionID); err != nil {
			return storageError(err)
		}
	}
	_, err := t.Exec(ctx, "UPDATE wikit."+prefix+"s SET sealed=true WHERE id=$1", id)
	return storageError(err)
}
func (t *securityTx) Publish(ctx context.Context, r content.Published, refs []content.MediaRef) error {
	raw, err := json.Marshal(r.Snapshot)
	if err != nil {
		return err
	}
	if _, err := t.Exec(ctx, `INSERT INTO wikit.revisions(id,page_id,type,parent_id,source_proposal_id,snapshot,author_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, r.ID, r.PageID, r.Snapshot.Type, r.ParentID, r.SourceProposalID, raw, r.AuthorID, r.CreatedAt); err != nil {
		return storageError(err)
	}
	if err := t.associations(ctx, "revision", r.ID, r.Snapshot, refs); err != nil {
		return err
	}
	if _, err := t.Exec(ctx, "UPDATE wikit.pages SET head_revision_id=$2 WHERE id=$1", r.PageID, r.ID); err != nil {
		return storageError(err)
	}
	_, err = t.Exec(ctx, "UPDATE wikit.page_policies SET status='published' WHERE page_id=$1", r.PageID)
	return storageError(err)
}
func (t *securityTx) Outbox(ctx context.Context, r content.Published) error {
	_, err := t.Exec(ctx, "INSERT INTO wikit.outbox_events(id,page_id,revision_id,kind,created_at) VALUES($1,$2,$1,'page.published',$3)", r.ID, r.PageID, r.CreatedAt)
	return storageError(err)
}

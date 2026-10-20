package postgres

import (
	"context"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
)

func (s *Store) WithinDiscussions(ctx context.Context, fn func(discussions.Tx) error) error {
	return s.withinSecurity(ctx, func(tx *securityTx) error { return fn(tx) })
}
func (t *securityTx) PageExists(ctx context.Context, id string) error {
	var found string
	return storageError(t.QueryRow(ctx, "SELECT id::text FROM wikit.pages WHERE id=$1", id).Scan(&found))
}
func (t *securityTx) Issue(ctx context.Context, id string) (discussions.Issue, error) {
	var v discussions.Issue
	err := t.QueryRow(ctx, `SELECT id::text,page_id::text,author_id::text,title,body,state,hidden,version,created_at,ARRAY(SELECT proposal_id::text FROM wikit.issue_proposals WHERE issue_id=i.id ORDER BY proposal_id) FROM wikit.page_issues i WHERE id=$1`, id).Scan(&v.ID, &v.PageID, &v.AuthorID, &v.Title, &v.Body, &v.State, &v.Hidden, &v.Version, &v.CreatedAt, &v.ProposalIDs)
	return v, storageError(err)
}
func (t *securityTx) Comment(ctx context.Context, id string) (discussions.Comment, error) {
	var v discussions.Comment
	err := t.QueryRow(ctx, `SELECT id::text,issue_id::text,author_id::text,body,hidden,version,created_at FROM wikit.comments WHERE id=$1`, id).Scan(&v.ID, &v.IssueID, &v.AuthorID, &v.Body, &v.Hidden, &v.Version, &v.CreatedAt)
	return v, storageError(err)
}
func (t *securityTx) IssueSummaries(ctx context.Context, page, state string) ([]discussions.Summary, error) {
	return t.discussionSummaries(ctx, `SELECT id::text,hidden FROM wikit.page_issues WHERE page_id=$1 AND ($2='' OR state=$2) ORDER BY created_at,id`, page, state)
}
func (t *securityTx) CommentSummaries(ctx context.Context, issue string) ([]discussions.Summary, error) {
	return t.discussionSummaries(ctx, `SELECT id::text,hidden FROM wikit.comments WHERE issue_id=$1 ORDER BY created_at,id`, issue)
}
func (t *securityTx) discussionSummaries(ctx context.Context, query string, args ...any) ([]discussions.Summary, error) {
	rows, err := t.Query(ctx, query, args...)
	if err != nil {
		return nil, storageError(err)
	}
	defer rows.Close()
	out := []discussions.Summary{}
	for rows.Next() {
		var v discussions.Summary
		if err := rows.Scan(&v.ID, &v.Hidden); err != nil {
			return nil, storageError(err)
		}
		out = append(out, v)
	}
	return out, storageError(rows.Err())
}
func (t *securityTx) InsertIssue(ctx context.Context, v discussions.Issue) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.page_issues(id,page_id,author_id,title,body,state,hidden,version,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.ID, v.PageID, v.AuthorID, v.Title, v.Body, v.State, v.Hidden, v.Version, v.CreatedAt)
	return storageError(err)
}
func (t *securityTx) SaveIssue(ctx context.Context, v discussions.Issue) error {
	_, err := t.Exec(ctx, `UPDATE wikit.page_issues SET state=$2,hidden=$3,version=$4 WHERE id=$1`, v.ID, v.State, v.Hidden, v.Version)
	return storageError(err)
}
func (t *securityTx) InsertComment(ctx context.Context, v discussions.Comment) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.comments(id,issue_id,author_id,body,hidden,version,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.IssueID, v.AuthorID, v.Body, v.Hidden, v.Version, v.CreatedAt)
	return storageError(err)
}
func (t *securityTx) SaveComment(ctx context.Context, v discussions.Comment) error {
	_, err := t.Exec(ctx, `UPDATE wikit.comments SET hidden=$2,version=$3 WHERE id=$1`, v.ID, v.Hidden, v.Version)
	return storageError(err)
}
func (t *securityTx) ProposalPage(ctx context.Context, id string) (string, error) {
	var page string
	err := t.QueryRow(ctx, `SELECT page_id::text FROM wikit.proposals WHERE id=$1`, id).Scan(&page)
	return page, storageError(err)
}
func (t *securityTx) LinkProposal(ctx context.Context, issue, page, proposal string) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.issue_proposals(issue_id,page_id,proposal_id) VALUES($1,$2,$3)`, issue, page, proposal)
	return storageError(err)
}
func (t *securityTx) Moderation(ctx context.Context, v access.Moderation) error {
	_, err := t.Exec(ctx, `INSERT INTO wikit.moderation_events(page_id,target_id,kind,actor_id,hidden,reason,version,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.PageID, v.TargetID, v.Kind, v.Actor.ID, v.Hidden, v.Reason, v.Version, v.RequestID)
	return storageError(err)
}

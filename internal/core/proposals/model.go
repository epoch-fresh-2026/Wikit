// Package proposals owns candidate histories and publication use cases.
package proposals

import (
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"time"
)

type Revision struct {
	ID             string           `json:"id"`
	ProposalID     string           `json:"proposal_id"`
	BaseRevisionID *string          `json:"base_revision_id"`
	Snapshot       content.Snapshot `json:"snapshot"`
	CreatedAt      time.Time        `json:"created_at"`
}
type Proposal struct {
	ID              string   `json:"id"`
	PageID          string   `json:"page_id"`
	AuthorID        string   `json:"author_id"`
	State           string   `json:"state"`
	Version         int64    `json:"version"`
	CurrentRevision Revision `json:"current_revision"`
}
type Summary struct{ ID, PageID, State string }

func Initial(id, page, author, revision string, snapshot content.Snapshot, now time.Time) Proposal {
	return Proposal{ID: id, PageID: page, AuthorID: author, State: "open", Version: 1, CurrentRevision: Revision{ID: revision, ProposalID: id, Snapshot: snapshot, CreatedAt: now}}
}

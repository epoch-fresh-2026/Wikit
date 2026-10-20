package content

import (
	"context"
	"time"
)

// Page is the stable publication identity and its current governance view.
type Page struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Visibility     string  `json:"visibility"`
	Status         string  `json:"status"`
	HeadRevisionID *string `json:"head_revision_id"`
	PolicyVersion  int64   `json:"policy_version"`
}
type Published struct {
	ID               string    `json:"id"`
	PageID           string    `json:"page_id"`
	ParentID         *string   `json:"parent_id"`
	SourceProposalID string    `json:"source_proposal_id"`
	Snapshot         Snapshot  `json:"snapshot"`
	CreatedAt        time.Time `json:"created_at"`
	AuthorID         string    `json:"author_id"`
	ContentURL       *string   `json:"content_url"`
}
type Blob struct {
	ID, State, Uploader, Key string
	Size                     int64
}
type Originals interface {
	Check(context.Context, Blob) error
}

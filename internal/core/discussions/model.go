// Package discussions owns Page Issues, immutable comments and moderation use cases.
package discussions

import (
	"context"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Issue struct {
	ID          string    `json:"id"`
	PageID      string    `json:"page_id"`
	AuthorID    string    `json:"author_id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	Hidden      bool      `json:"hidden"`
	Version     int64     `json:"version"`
	ProposalIDs []string  `json:"proposal_ids"`
	CreatedAt   time.Time `json:"created_at"`
}
type Comment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	Hidden    bool      `json:"hidden"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
type Summary struct {
	ID     string
	Hidden bool
}
type Tx interface {
	access.Authorization
	PageExists(context.Context, string) error
	Issue(context.Context, string) (Issue, error)
	Comment(context.Context, string) (Comment, error)
	IssueSummaries(context.Context, string, string) ([]Summary, error)
	CommentSummaries(context.Context, string) ([]Summary, error)
	InsertIssue(context.Context, Issue) error
	SaveIssue(context.Context, Issue) error
	InsertComment(context.Context, Comment) error
	SaveComment(context.Context, Comment) error
	ProposalPage(context.Context, string) (string, error)
	LinkProposal(context.Context, string, string, string) error
	Moderation(context.Context, access.Moderation) error
	Audit(context.Context, identity.Actor, string, string, string) error
	Replay(context.Context, string, string, string, string) ([]byte, error)
	Remember(context.Context, string, string, string, string, []byte) error
	Cursor(context.Context, string, string, string, int) (int, error)
	NewCursor(context.Context, string, string, string, int, int) (string, error)
}
type Repository interface {
	WithinDiscussions(context.Context, func(Tx) error) error
	Allow(context.Context, string, string) error
}
type Text interface{ NFC(string) string }
type Service struct {
	repo    Repository
	secrets identity.Secrets
	text    Text
}

func New(repo Repository, secrets identity.Secrets, text Text) *Service {
	return &Service{repo, secrets, text}
}

package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

var operations = []string{
	"accounts_list", "account_update", "groups_list", "group_create", "group_update", "audit_list",
	"pages_list", "page_create", "page_get", "page_policy_get", "page_policy_update", "page_moderate", "revisions_list", "revision_get",
	"proposal_queue", "proposals_list", "proposal_create", "proposal_get", "proposal_revisions_list", "proposal_update", "proposal_diff", "proposal_rebase", "reviews_list", "review_create", "proposal_merge", "proposal_state", "page_revert", "page_restore",
	"issues_list", "issue_create", "issue_get", "issue_state", "issue_link_proposal", "issue_moderate", "comments_list", "comment_create", "comment_moderate", "upload_prepare", "upload_get", "tags_list", "search",
}

func knownOperation(name string) bool { return slices.Contains(operations, name) }

type arguments struct {
	AccountID     string          `json:"account_id"`
	GroupID       string          `json:"group_id"`
	PageID        string          `json:"page_id"`
	ProposalID    string          `json:"proposal_id"`
	RevisionID    string          `json:"revision_id"`
	IssueID       string          `json:"issue_id"`
	CommentID     string          `json:"comment_id"`
	UploadID      string          `json:"upload_id"`
	Limit         *int            `json:"limit"`
	Cursor        string          `json:"cursor"`
	Tag           string          `json:"tag"`
	Type          string          `json:"type"`
	State         string          `json:"state"`
	Q             string          `json:"q"`
	IncludeHidden bool            `json:"include_hidden"`
	Body          json.RawMessage `json:"body"`
	Key           string          `json:"idempotency_key"`
}

func decode[T any](raw []byte) (T, error) {
	var in T
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, fault.BadRequest
	}
	return in, nil
}

type moderation struct {
	Hidden  bool   `json:"hidden"`
	Reason  string `json:"reason"`
	Version int64  `json:"expected_version"`
}

func (s Services) invoke(ctx context.Context, op string, a arguments, r access.Request, ip string) (any, error) {
	c := r.Credentials
	limit := 20
	if a.Limit != nil {
		limit = *a.Limit
	}
	switch op {
	case "accounts_list", "groups_list", "audit_list":
		kind := map[string]string{"accounts_list": "accounts", "groups_list": "groups", "audit_list": "audit"}[op]
		return s.Access.List(ctx, c, kind, a.Cursor, limit)
	case "account_update":
		in, err := decode[struct {
			Role    string `json:"role"`
			Status  string `json:"status"`
			Version int64  `json:"expected_auth_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Access.UpdateAccount(ctx, r, a.AccountID, in.Role, in.Status, in.Version)
	case "group_create", "group_update":
		in, err := decode[struct {
			Name    string   `json:"name"`
			Members []string `json:"member_ids"`
			Version int64    `json:"expected_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Access.SaveGroup(ctx, r, a.GroupID, in.Name, in.Members, in.Version)
	case "page_policy_get":
		if !identity.ValidID(a.PageID) {
			return nil, fault.NotFound
		}
		return s.Access.GetPolicy(ctx, c, a.PageID)
	case "page_policy_update":
		if !identity.ValidID(a.PageID) {
			return nil, fault.NotFound
		}
		in, err := decode[struct {
			Visibility string         `json:"visibility"`
			Grants     []access.Grant `json:"grants"`
			Version    int64          `json:"expected_policy_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Access.UpdatePolicy(ctx, r, a.PageID, access.Policy{Visibility: in.Visibility, Grants: in.Grants}, in.Version)
	case "pages_list", "tags_list", "revisions_list", "proposal_queue", "proposals_list", "proposal_revisions_list":
		kind := map[string]string{"pages_list": "pages", "tags_list": "tags", "revisions_list": "revisions", "proposal_queue": "proposals", "proposals_list": "proposals", "proposal_revisions_list": "candidates"}[op]
		return s.Pages.List(ctx, c, pages.Query{Kind: kind, PageID: a.PageID, ProposalID: a.ProposalID, Tag: a.Tag, Type: a.Type, State: a.State, Cursor: a.Cursor, Limit: limit})
	case "page_get", "revision_get":
		return s.Pages.Get(ctx, c, a.PageID, a.RevisionID)
	case "proposal_get":
		return s.Pages.Proposal(ctx, c, a.ProposalID)
	case "page_create":
		in, err := decode[struct {
			Visibility string           `json:"visibility"`
			Snapshot   content.Snapshot `json:"snapshot"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Pages.Create(ctx, r, in.Visibility, in.Snapshot, ip)
	case "proposal_create":
		in, err := decode[struct {
			Head     *string          `json:"expected_head_revision_id"`
			Snapshot content.Snapshot `json:"snapshot"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Proposals.Create(ctx, r, a.PageID, in.Head, in.Snapshot)
	case "proposal_update":
		in, err := decode[struct {
			proposals.Expected
			Snapshot content.Snapshot `json:"snapshot"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Proposals.Update(ctx, r, a.ProposalID, in.Expected, in.Snapshot)
	case "proposal_diff":
		return s.Proposals.Diff(ctx, c, a.ProposalID)
	case "reviews_list":
		return s.Proposals.Reviews(ctx, c, a.ProposalID, a.Cursor, limit)
	case "proposal_merge", "proposal_rebase":
		in, err := decode[proposals.Expected](a.Body)
		if err != nil {
			return nil, err
		}
		if op == "proposal_merge" {
			return s.Proposals.Merge(ctx, r, a.ProposalID, in)
		}
		return s.Proposals.Rebase(ctx, r, a.ProposalID, in)
	case "review_create":
		in, err := decode[struct {
			proposals.Expected
			Round    int64  `json:"expected_review_round"`
			Decision string `json:"decision"`
			Body     string `json:"body"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Proposals.Review(ctx, r, a.ProposalID, in.Expected, in.Round, in.Decision, in.Body)
	case "proposal_state":
		in, err := decode[struct {
			State   string `json:"state"`
			Version int64  `json:"expected_proposal_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Proposals.State(ctx, r, a.ProposalID, in.State, in.Version)
	case "page_revert", "page_restore":
		in, err := decode[struct {
			Revision string `json:"revision_id"`
			Head     string `json:"expected_head_revision_id"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		kind := "revert"
		if op == "page_restore" {
			kind = "restore"
		}
		return s.Proposals.Rollback(ctx, r, a.PageID, kind, in.Revision, in.Head)
	case "issues_list", "comments_list":
		kind, id := "issues", a.PageID
		if op == "comments_list" {
			kind, id = "comments", a.IssueID
		}
		return s.Discussions.List(ctx, c, discussions.Query{Kind: kind, ID: id, State: a.State, Cursor: a.Cursor, Limit: limit, IncludeHidden: a.IncludeHidden})
	case "issue_get":
		return s.Discussions.Get(ctx, c, a.IssueID, a.IncludeHidden)
	case "issue_create":
		in, err := decode[struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Discussions.Create(ctx, r, a.PageID, in.Title, in.Body, ip)
	case "comment_create":
		in, err := decode[struct {
			Body string `json:"body"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Discussions.Comment(ctx, r, a.IssueID, in.Body, ip)
	case "issue_state":
		in, err := decode[struct {
			State   string `json:"state"`
			Version int64  `json:"expected_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Discussions.State(ctx, r, a.IssueID, in.State, in.Version)
	case "issue_link_proposal":
		in, err := decode[struct {
			Proposal string `json:"proposal_id"`
			Version  int64  `json:"expected_version"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Discussions.Link(ctx, r, a.IssueID, in.Proposal, in.Version)
	case "page_moderate", "issue_moderate", "comment_moderate":
		in, err := decode[moderation](a.Body)
		if err != nil {
			return nil, err
		}
		if op == "page_moderate" {
			return s.Pages.Moderate(ctx, r, a.PageID, in.Hidden, in.Reason, in.Version)
		}
		scope, id := "issue", a.IssueID
		if op == "comment_moderate" {
			scope, id = "comment", a.CommentID
		}
		return s.Discussions.Moderate(ctx, r, scope, id, in.Hidden, in.Reason, in.Version)
	case "upload_prepare":
		in, err := decode[struct {
			Filename string `json:"filename"`
			Mime     string `json:"mime"`
			Size     int64  `json:"size"`
		}](a.Body)
		if err != nil {
			return nil, err
		}
		return s.Media.Prepare(ctx, r, in.Filename, in.Mime, in.Size, ip)
	case "upload_get":
		return s.Media.Get(ctx, c, a.UploadID)
	case "search":
		return s.Search.Search(ctx, c, search.Query{Q: a.Q, Tag: a.Tag, Cursor: a.Cursor, Limit: limit})
	}
	return nil, fault.NotFound
}

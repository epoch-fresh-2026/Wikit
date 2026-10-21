package discussions

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type loaded struct {
	actor   identity.Actor
	policy  access.Policy
	page    string
	issue   Issue
	comment Comment
}

func textValid(s string, max int, nonblank bool) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) <= max && (!nonblank || strings.TrimSpace(s) != "")
}
func load(ctx context.Context, tx Tx, c identity.Credentials, scope, id string, management bool) (loaded, error) {
	var v loaded
	id = strings.ToLower(id)
	if (scope == "page" && !access.ValidPageID(id)) || (scope != "page" && !identity.ValidID(id)) {
		return v, fault.NotFound
	}
	var err error
	switch scope {
	case "page":
		v.page = id
	case "issue":
		v.issue, err = tx.Issue(ctx, id)
		v.page = v.issue.PageID
	case "comment":
		v.comment, err = tx.Comment(ctx, id)
		if err == nil {
			v.issue, err = tx.Issue(ctx, v.comment.IssueID)
			v.page = v.issue.PageID
		}
	default:
		return v, fault.BadRequest
	}
	if err != nil {
		return v, err
	}
	cap := "read"
	if management {
		cap = "manage"
	}
	v.actor, v.policy, err = access.Authorize(ctx, tx, c, v.page, cap, management)
	if err != nil {
		return v, err
	}
	if scope == "page" {
		if err := tx.PageExists(ctx, id); err != nil {
			return v, err
		}
	}
	if !management && (v.issue.Hidden || v.comment.Hidden) {
		return v, fault.NotFound
	}
	return v, nil
}
func permission(ctx context.Context, tx Tx, v loaded, cap string) error {
	groups, err := tx.Groups(ctx)
	if err != nil {
		return err
	}
	return access.CheckPolicy(v.actor, v.policy, groups, cap, false)
}
func editor(ctx context.Context, tx Tx, v loaded) error {
	if v.actor.ID == v.issue.AuthorID {
		return nil
	}
	return permission(ctx, tx, v, "manage")
}
func validRequest(r access.Request) error {
	if len(r.Key) < 8 || len(r.Key) > 128 || r.Hash == "" || r.ID == "" {
		return fault.BadRequest
	}
	return nil
}
func write[T any](ctx context.Context, s *Service, r access.Request, scope, id, op string, management bool, fn func(Tx, loaded) (T, error)) (T, error) {
	var out T
	if err := validRequest(r); err != nil {
		return out, err
	}
	r.Credentials.Write = true
	id = strings.ToLower(id)
	err := s.repo.WithinDiscussions(ctx, func(tx Tx) error {
		if _, err := identity.Require(ctx, tx, r.Credentials); err != nil {
			return err
		}
		v, err := load(ctx, tx, r.Credentials, scope, id, management)
		if err != nil {
			return err
		}
		operation := op + ":" + id
		raw, err := tx.Replay(ctx, v.actor.ID, operation, r.Key, r.Hash)
		if err != nil {
			return err
		}
		if raw != nil {
			if err := json.Unmarshal(raw, &out); err != nil {
				return err
			}
			// Creation replays must check the current result's visibility too.
			if !management {
				switch value := any(out).(type) {
				case Issue:
					_, err = load(ctx, tx, r.Credentials, "issue", value.ID, false)
				case Comment:
					_, err = load(ctx, tx, r.Credentials, "comment", value.ID, false)
				}
			}
			return err
		}
		out, err = fn(tx, v)
		if err != nil {
			return err
		}
		target := id
		switch value := any(out).(type) {
		case Issue:
			target = value.ID
		case Comment:
			target = value.ID
		}
		if err := tx.Audit(ctx, v.actor, op, target, r.ID); err != nil {
			return err
		}
		raw, err = json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Remember(ctx, v.actor.ID, operation, r.Key, r.Hash, raw)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}
func (s *Service) limit(ctx context.Context, r access.Request, scope, id, op, ip string) error {
	if err := validRequest(r); err != nil {
		return err
	}
	r.Credentials.Write = true
	var actor string
	err := s.repo.WithinDiscussions(ctx, func(tx Tx) error {
		if _, err := identity.Require(ctx, tx, r.Credentials); err != nil {
			return err
		}
		v, err := load(ctx, tx, r.Credentials, scope, id, false)
		if err != nil {
			return err
		}
		actor = v.actor.ID
		return permission(ctx, tx, v, "discuss")
	})
	if err != nil {
		return err
	}
	return s.repo.Allow(ctx, op, ip+"\x00"+actor)
}
func (s *Service) Create(ctx context.Context, r access.Request, page, title, body, ip string) (Issue, error) {
	if !textValid(title, 300, true) || !textValid(body, 65536, false) {
		return Issue{}, fault.Invalid
	}
	title = s.text.NFC(strings.TrimSpace(title))
	if !textValid(title, 300, true) {
		return Issue{}, fault.Invalid
	}
	if err := s.limit(ctx, r, "page", page, "issue.create", ip); err != nil {
		return Issue{}, err
	}
	return write(ctx, s, r, "page", page, "issue.create", false, func(tx Tx, v loaded) (Issue, error) {
		if err := permission(ctx, tx, v, "discuss"); err != nil {
			return Issue{}, err
		}
		id, err := s.secrets.ID()
		if err != nil {
			return Issue{}, err
		}
		out := Issue{ID: id, PageID: v.page, AuthorID: v.actor.ID, Title: title, Body: body, State: "open", Version: 1, ProposalIDs: []string{}, CreatedAt: time.Now().UTC()}
		return out, tx.InsertIssue(ctx, out)
	})
}
func (s *Service) Comment(ctx context.Context, r access.Request, issue, body, ip string) (Comment, error) {
	if !textValid(body, 65536, true) {
		return Comment{}, fault.Invalid
	}
	if err := s.limit(ctx, r, "issue", issue, "comment.create", ip); err != nil {
		return Comment{}, err
	}
	return write(ctx, s, r, "issue", issue, "comment.create", false, func(tx Tx, v loaded) (Comment, error) {
		if err := permission(ctx, tx, v, "discuss"); err != nil {
			return Comment{}, err
		}
		id, err := s.secrets.ID()
		if err != nil {
			return Comment{}, err
		}
		out := Comment{ID: id, IssueID: v.issue.ID, AuthorID: v.actor.ID, Body: body, Version: 1, CreatedAt: time.Now().UTC()}
		return out, tx.InsertComment(ctx, out)
	})
}
func (s *Service) State(ctx context.Context, r access.Request, id, state string, version int64) (Issue, error) {
	return write(ctx, s, r, "issue", id, "issue.state", false, func(tx Tx, v loaded) (Issue, error) {
		if !slices.Contains([]string{"open", "closed"}, state) || version < 0 {
			return Issue{}, fault.Invalid
		}
		if err := editor(ctx, tx, v); err != nil {
			return Issue{}, err
		}
		if version != v.issue.Version {
			return Issue{}, fault.Stale
		}
		if state == v.issue.State {
			return Issue{}, fault.State
		}
		v.issue.State = state
		v.issue.Version++
		return v.issue, tx.SaveIssue(ctx, v.issue)
	})
}
func (s *Service) Link(ctx context.Context, r access.Request, id, proposal string, version int64) (Issue, error) {
	proposal = strings.ToLower(proposal)
	if !identity.ValidID(proposal) {
		return Issue{}, fault.NotFound
	}
	if version < 0 {
		return Issue{}, fault.Invalid
	}
	return write(ctx, s, r, "issue", id, "issue.link", false, func(tx Tx, v loaded) (Issue, error) {
		if err := editor(ctx, tx, v); err != nil {
			return Issue{}, err
		}
		if err := permission(ctx, tx, v, "discuss"); err != nil {
			return Issue{}, err
		}
		if version != v.issue.Version {
			return Issue{}, fault.Stale
		}
		page, err := tx.ProposalPage(ctx, proposal)
		if err != nil {
			return Issue{}, err
		}
		if _, _, err := access.Authorize(ctx, tx, r.Credentials, page, "read", false); err != nil {
			return Issue{}, err
		}
		if page != v.page {
			return Issue{}, fault.Invalid
		}
		if slices.Contains(v.issue.ProposalIDs, proposal) {
			return v.issue, nil
		}
		if err := tx.LinkProposal(ctx, v.issue.ID, v.page, proposal); err != nil {
			return Issue{}, err
		}
		v.issue.ProposalIDs = append(v.issue.ProposalIDs, proposal)
		slices.Sort(v.issue.ProposalIDs)
		v.issue.Version++
		return v.issue, tx.SaveIssue(ctx, v.issue)
	})
}
func (s *Service) Moderate(ctx context.Context, r access.Request, scope, id string, hidden bool, reason string, version int64) (any, error) {
	if !textValid(reason, 2000, true) || version < 0 {
		return nil, fault.Invalid
	}
	reason = s.text.NFC(strings.TrimSpace(reason))
	if !textValid(reason, 2000, true) {
		return nil, fault.Invalid
	}
	if scope == "issue" {
		return write(ctx, s, r, scope, id, "issue.moderate", true, func(tx Tx, v loaded) (Issue, error) {
			if version != v.issue.Version {
				return Issue{}, fault.Stale
			}
			if hidden == v.issue.Hidden {
				return Issue{}, fault.State
			}
			v.issue.Hidden = hidden
			v.issue.Version++
			if err := tx.SaveIssue(ctx, v.issue); err != nil {
				return Issue{}, err
			}
			err := tx.Moderation(ctx, access.Moderation{Actor: v.actor, PageID: v.page, TargetID: v.issue.ID, Kind: "issue", Hidden: hidden, Reason: reason, Version: v.issue.Version, RequestID: r.ID})
			return v.issue, err
		})
	}
	if scope == "comment" {
		return write(ctx, s, r, scope, id, "comment.moderate", true, func(tx Tx, v loaded) (Comment, error) {
			if version != v.comment.Version {
				return Comment{}, fault.Stale
			}
			if hidden == v.comment.Hidden {
				return Comment{}, fault.State
			}
			v.comment.Hidden = hidden
			v.comment.Version++
			if err := tx.SaveComment(ctx, v.comment); err != nil {
				return Comment{}, err
			}
			err := tx.Moderation(ctx, access.Moderation{Actor: v.actor, PageID: v.page, TargetID: v.comment.ID, Kind: "comment", Hidden: hidden, Reason: reason, Version: v.comment.Version, RequestID: r.ID})
			return v.comment, err
		})
	}
	return nil, fault.BadRequest
}
